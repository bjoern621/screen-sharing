// Package gstrun runs one GStreamer publish pipeline inside the process that calls it.
//
// The publish engine spawns it in place of gst-launch-1.0, for everything a launcher cannot be
// asked.
// gst-launch is opaque to whoever started it, reporting frames over the pipeline's own meter branch
// and nothing else, so no caller can read a negotiated cap, write a property on a running pipeline,
// or reach the metadata a capture carries beside its frames.
//
// The pipeline is still a child process, which is the half of gst-launch worth keeping:
// a capture that segfaults inside a driver takes the child with it and leaves the backend,
// the control socket and every viewer running (docs/capture-architecture.md).
//
// What it adds is what the capture turned out to be.
// The negotiated caps reach the caller as a line on standard output, their transfer characteristic
// being the only honest answer to whether a surface is HDR.
// Caps carrying none are SDR, and guessing upward publishes a PQ tag over an SDR desktop.
// The same answer narrows the encoder input to the colour the surface carries before anything
// converts it (surface.go), which a fixed pipeline string has no way to state.
package gstrun

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/pipedelay"
)

// CapsPrefix leads the line the capture's negotiated caps are reported on.
//
// Standard output, because the supervisor already reads that stream and its meter skips a line it
// does not recognise (publish/gststats.go).
// One stream carrying several kinds of line keeps the child's contract to a pipe and a prefix,
// rather than a second socket to open, inherit and close.
const CapsPrefix = "screenshare-caps "

// Run plays description until it ends, the context is cancelled or the pipeline reports an error,
// and answers what ended it.
//
// A pipeline reaching end of stream returns nil: a capture ends when it is asked to,
// and the caller decides whether that was expected.
// An error carries the element's own wording, which is what the supervisor puts in front of a
// reader.
func Run(ctx context.Context, description string, out io.Writer) error {
	return RunWithControl(ctx, description, "", out)
}

// RunWithControl is Run with a control socket at controlPath.
// The parent writes the live state this pipeline should be holding there (control.go).
// The empty path takes no socket, which is what a run nobody talks to uses.
func RunWithControl(ctx context.Context, description, controlPath string, out io.Writer) error {
	return RunWithOptions(ctx, description, Options{Control: controlPath}, out)
}

// Options is what a run does beyond playing its pipeline.
//
// A struct rather than more arguments: each entry is work the child does for a caller that asked
// and none for one that did not, a measuring run taking neither and a publish whichever its
// settings earned.
type Options struct {
	// Control is where the parent writes the live state this pipeline should be holding,
	// and the empty path takes no socket (control.go).
	Control string
	// Pointer reports where the pointer is, for a publish whose cursor mode sends the position instead
	// of drawing it (pointer.go).
	Pointer bool
	// Delay names the element a frame's delay through this pipeline is measured at, and the empty
	// name measures none (delay.go).
	Delay string
}

// RunWithOptions is Run with whatever this run does beside playing.
//
// A description that will not parse and a pipeline that refuses to play are Umgebungsfehler:
// the elements come from the machine's own GStreamer install, so a missing plugin is a fact about
// the machine and leaves as an error the supervisor shows.
func RunWithOptions(ctx context.Context, description string, options Options, out io.Writer) error {
	assert.IsNotNil(ctx, "a run runs under a context")
	assert.IsNotNil(out, "a run reports what it negotiated to a writer")
	assert.Assert(strings.TrimSpace(description) != "", "a run names a pipeline to play")

	// Wrapped once, before any goroutine below is handed it (syncwriter.go).
	out = &syncWriter{w: out}

	gst.Init()

	el, err := gst.ParseLaunch(description)
	if err != nil {
		return fmt.Errorf("building the pipeline: %w", err)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		return fmt.Errorf("the description built a %T rather than a pipeline", el)
	}

	// NULL on every return path: it is what releases the capture's own handles, a portal session,
	// a DRM lease, an X connection, rather than leaving them to the process exiting.
	defer pipeline.SetState(gst.StateNull)

	// Before PLAYING, and it has to be: the narrowing acts on the caps event the capture sends
	// downstream, which is on its way the moment the state change starts (surface.go).
	narrowToSurface(pipeline)

	// Also before PLAYING, so a parent writing the moment it sees the child start finds something
	// listening.
	listener, err := listenControl(options.Control)
	if err != nil {
		return fmt.Errorf("opening the control socket: %w", err)
	}
	if listener != nil {
		defer listener.Close()
		go serveControl(listener, pipeline, out)
	}

	// The reader runs on a clock of its own and ends with the context, which is what makes a position
	// something the child reports rather than something the pipeline carries.
	if options.Pointer {
		go reportPointer(ctx, out)
	}

	// The probe goes on before PLAYING and the reporting after it, on a clock of its own like the
	// pointer's: a delay is a reading taken while the pipeline runs rather than something a frame
	// carries out of it.
	var delay *pipedelay.Probe
	if options.Delay != "" {
		delay = watchDelay(pipeline, options.Delay)
	}

	if ret := pipeline.SetState(gst.StatePlaying); ret == gst.StateChangeFailure {
		return fmt.Errorf("the pipeline refused to play")
	}
	if delay != nil {
		go reportDelay(ctx, pipeline, delay, out)
	}

	reported := false
	// The bus is drained on the calling goroutine, so Run is the pipeline's lifetime.
	// Cancelling the context ends the channel, which is what stops the run and drops it back to NULL.
	for msg := range pipeline.GetBus().Messages(ctx) {
		switch msg.Type() {
		case gst.MessageError:
			debug, err := msg.ParseError()
			if debug != "" {
				fmt.Fprintf(out, "%s\n", debug)
			}
			return err
		case gst.MessageEOS:
			return nil
		case gst.MessageAsyncDone, gst.MessageStateChanged:
			// SetState answers before the change lands, so the caps arrive here rather than off that call.
			// Both messages repeat, and the report is taken on the first that can answer and never again:
			// what the capture negotiated does not change under a running pipeline.
			if !reported {
				reported = reportCaps(pipeline, out)
			}
		}
	}
	// The channel ended, which is the supervisor cancelling: stopping is not a failure.
	return nil
}

// reportCaps writes the capture's negotiated caps and reports whether it found any.
//
// The source is found by shape and never by name: IterateSources is GStreamer's own answer for the
// elements with no sink pad, which is what produces frames from outside the pipeline and what
// nothing downstream of it is.
// A name would live both here and in whichever backend built the description, free to disagree the
// moment either moved.
func reportCaps(pipeline gst.Pipeline, out io.Writer) bool {
	assert.IsNotNil(out, "negotiated caps are reported to a writer")

	for v := range pipeline.IterateSources().Values() {
		el, ok := v.(gst.Element)
		if !ok {
			continue
		}
		pad := el.GetStaticPad("src")
		if pad == nil {
			continue
		}
		caps := pad.GetCurrentCaps()
		if caps == nil {
			continue
		}
		fmt.Fprintf(out, "%s%s\n", CapsPrefix, caps.String())
		return true
	}
	return false
}
