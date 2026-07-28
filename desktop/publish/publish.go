// Package publish selects and supervises the pipeline that captures the screen
// and pushes the encoded stream to the relay.
//
// The seam is the Publisher: the app starts and stops a Publisher and never
// names ffmpeg or GStreamer. A capture backend owns its whole pipeline behind
// that contract. One publish engine drives the screen grabbers that feed a single
// ffmpeg process; the other drives the portal capture backend, where GStreamer
// captures, encodes and ships in one graph. Both satisfy the same contract, so
// the lifecycle code above the seam is identical for either.
package publish

import (
	"fmt"
	"slices"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// Stats is one encoder progress sample surfaced to the UI. It reuses the ffmpeg
// progress shape as the wire format; an engine without an equivalent progress
// stream measures the same figures its own way (see gstMeter), and a figure its
// pipeline exposes no measurement for is marked in Missing rather than sent as a
// zero, which is the reading that marks a stalled encoder.
type Stats = ffmpeg.Stats

// Missing is the set of Stats figures a sample carries no measurement for.
type Missing = ffmpeg.Missing

// Handle supervises one running publish session.
type Handle interface {
	// Running reports whether the pipeline is still alive.
	Running() bool
	// Stop terminates the pipeline; a requested stop is not a failure.
	Stop()
}

// Callbacks receive progress and the terminal exit of a publish session.
// OnStats is best-effort. OnExit fires exactly once, with a non-nil error only
// on unexpected failure, the tail of stderr, and the path of the run log.
type Callbacks struct {
	OnStats func(Stats)
	OnExit  func(err error, stderrTail string, logPath string)
}

// Publisher builds and runs the publish pipeline for a family of capture
// backends. Command renders the pipeline for the UI without running it.
type Publisher interface {
	Command(s settings.Stream) (string, error)
	Start(s settings.Stream, tag string, cb Callbacks) (Handle, error)
	// Engine names the publish engine that runs the pipeline, "ffmpeg" or
	// "gstreamer". The lifecycle code above the seam never reads it; the settings
	// form does, because which rate-control knobs reach the encoder differs per
	// engine, and a control the engine ignores is greyed with that reason instead
	// of silently doing nothing.
	Engine() string
	// Carries reports whether this engine can drive the named transport, i.e.
	// the transport implements the publish capability the engine serializes
	// through. The ffmpeg engine needs an FFmpegPublisher, the GStreamer engine
	// a GstPublisher, so the portal capture backend cannot carry a transport with no
	// GStreamer sink.
	Carries(transportName string) bool
}

// The publish engines that run the capture backends. An engine names itself to
// capabilities.Validate, to the encoder probe and to the settings form, so the two
// spellings are constants rather than literals repeated per caller.
const (
	EngineFfmpeg = "ffmpeg"
	EngineGst    = "gstreamer"
)

// captureBackends is the single source pairing a capture backend with the engine
// that runs it. Capture backend and publish engine are two axes, not one: a
// screen source is a row here, and which engine the row names follows from which
// framework has an element or an input device for that source, not from a
// property of the engine.
//
// The pairing is one-to-one because no source has both. ffmpeg has the four
// screen grabbers and no PipeWire input device; GStreamer has pipewiresrc and
// ximagesrc and no DRM/KMS source element at all, which is why kmsgrab appears
// once and under ffmpeg. A source both frameworks could read would be two rows,
// one per engine, and nothing else would change.
var captureBackends = map[string]Publisher{
	"ddagrab":   ffmpegEngine{},
	"gdigrab":   ffmpegEngine{},
	"x11grab":   ffmpegEngine{},
	"kmsgrab":   ffmpegEngine{},
	"portal":    gstEngine{capture: portalCapture{}},
	"ximagesrc": gstEngine{capture: ximageCapture{}},
}

// For returns the Publisher that runs the given capture backend.
func For(capture string) (Publisher, error) {
	p, ok := captureBackends[capture]
	if !ok {
		return nil, fmt.Errorf("unknown capture backend %q", capture)
	}
	return p, nil
}

// Captures lists the capture backends the app can run, sorted for a stable
// order across the wire.
func Captures() []string {
	out := make([]string, 0, len(captureBackends))
	for capture := range captureBackends {
		out = append(out, capture)
	}
	slices.Sort(out)
	return out
}

// Engines lists the publish engines that run the capture backends, sorted for a stable
// order. Each of them answers capabilities.Validate under its own name and is probed
// separately for which codecs it can run here, since the two wrap different encoder
// implementations.
func Engines() []string {
	seen := make(map[string]bool, len(captureBackends))
	var out []string
	for _, p := range captureBackends {
		if name := p.Engine(); !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// EngineFor returns the name of the publish engine that runs the capture backend.
func EngineFor(capture string) (string, error) {
	p, err := For(capture)
	if err != nil {
		return "", err
	}
	return p.Engine(), nil
}

// TransportsFor returns the transports the capture backend's engine can carry,
// in transport registry order. The result is the subset of transport.Names()
// the engine can serialize through, so a capture whose engine lacks a sink for a
// transport excludes it.
func TransportsFor(capture string) ([]string, error) {
	p, err := For(capture)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, name := range transport.Names() {
		if p.Carries(name) {
			out = append(out, name)
		}
	}
	return out, nil
}
