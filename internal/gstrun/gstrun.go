// Package gstrun runs one GStreamer publish pipeline inside the process that calls it.
//
// It is what the publish engine spawns in place of gst-launch-1.0, and the reason for the
// swap is everything a launcher cannot be asked. gst-launch is opaque to whoever started
// it: it reports frames over the pipeline's own meter branch and nothing else, so no
// caller can read a negotiated cap, hand the pipeline a new property value while it runs,
// or reach the metadata a capture carries beside its frames.
//
// The pipeline is still a child process, which is the half of gst-launch worth keeping: a
// capture that segfaults inside a driver takes the child with it and leaves the backend,
// the control socket and every viewer running (docs/capture-architecture.md).
//
// What it adds over the launcher is one report today. The negotiated caps of the capture
// go to the caller as a line on standard output, because the transfer characteristic in
// them is the only honest answer to whether a surface is HDR: caps carrying none are SDR,
// and guessing upward publishes a PQ tag over an SDR desktop (docs/plan.md, "HDR").
package gstrun

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
)

// CapsPrefix leads the line this reports the capture's negotiated caps on.
//
// Standard output is where it goes because the supervisor already reads that stream, and
// the meter that reads it skips a line it does not recognise (publish/gststats.go). One
// stream carrying two kinds of line is what keeps the child's contract to a pipe and a
// prefix rather than a second socket to open, inherit and close.
const CapsPrefix = "screenshare-caps "

// Run plays description until it ends, the context is cancelled, or the pipeline reports
// an error, and returns what ended it.
//
// A pipeline that reaches the end of its stream returns nil: a capture ends when it is
// asked to, and the caller decides whether that was expected. An error carries the
// element's own wording, which is what the supervisor puts in front of a reader.
func Run(ctx context.Context, description string, out io.Writer) error {
	return RunWithControl(ctx, description, "", out)
}

// RunWithControl is Run with a control socket at controlPath, where the parent writes the
// live state this pipeline should be holding (control.go). The empty path takes no socket,
// which is what a run nobody talks to uses.
func RunWithControl(ctx context.Context, description, controlPath string, out io.Writer) error {
	assert.Assert(strings.TrimSpace(description) != "", "a run names a pipeline to play")

	gst.Init()

	el, err := gst.ParseLaunch(description)
	if err != nil {
		return fmt.Errorf("building the pipeline: %w", err)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		return fmt.Errorf("the description built a %T rather than a pipeline", el)
	}

	// Stopping is the same two steps whichever way this returns: the pipeline is asked
	// for NULL, which is what releases the capture's own handles - a portal session, a
	// DRM lease, an X connection - rather than leaving them to the process exiting.
	defer pipeline.SetState(gst.StateNull)

	// The socket opens before the pipeline plays, so a parent that writes the moment it
	// sees the child start finds something listening.
	listener, err := listenControl(controlPath)
	if err != nil {
		return fmt.Errorf("opening the control socket: %w", err)
	}
	if listener != nil {
		defer listener.Close()
		go serveControl(listener, pipeline, out)
	}

	if ret := pipeline.SetState(gst.StatePlaying); ret == gst.StateChangeFailure {
		return fmt.Errorf("the pipeline refused to play")
	}

	reported := false
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
			// Caps are negotiated by the time the pipeline settles, and both messages
			// arrive more than once, so the report is taken on the first one that can
			// answer and never again: what the capture negotiated does not change under
			// a running pipeline, and a second line would say the same thing twice.
			if !reported {
				reported = reportCaps(pipeline, out)
			}
		}
	}
	// The context was cancelled, which is the supervisor asking this to stop.
	return nil
}

// reportCaps writes the capture's negotiated caps and reports whether it found any.
//
// The source is found by shape rather than by name: IterateSources is GStreamer's own
// answer for the elements with no sink pad, which is what an element producing frames from
// outside the pipeline is and what nothing downstream of it is. Naming the capture instead
// would put one string in this package and in whichever backend built the description,
// free to disagree the moment either moved.
func reportCaps(pipeline gst.Pipeline, out io.Writer) bool {
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
