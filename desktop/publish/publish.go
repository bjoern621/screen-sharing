// Package publish selects and supervises the pipeline that captures the screen
// and pushes the encoded stream to the relay.
//
// The seam is the Publisher: the app starts and stops a Publisher and never
// names ffmpeg or GStreamer. A capture backend owns its whole pipeline behind
// that contract. One engine drives the screen grabbers that feed a single
// ffmpeg process; another drives the xdg-desktop-portal path, where GStreamer
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
// progress shape as the wire format. An engine with no per-frame progress
// leaves OnStats uncalled rather than inventing figures.
type Stats = ffmpeg.Stats

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
	// Carries reports whether this engine can drive the named transport, i.e.
	// the transport implements the publish capability the engine serializes
	// through. The ffmpeg engine needs an FFmpegPublisher, the GStreamer engine
	// a GstPublisher, so the portal path cannot carry a transport with no
	// GStreamer sink.
	Carries(transportName string) bool
}

// engineByCapture is the single source mapping a capture backend to the engine
// that runs it. The ffmpeg screen grabbers share one engine; the portal path
// runs through GStreamer.
var engineByCapture = map[string]Publisher{
	"ddagrab": ffmpegEngine{},
	"gdigrab": ffmpegEngine{},
	"x11grab": ffmpegEngine{},
	"kmsgrab": ffmpegEngine{},
	"portal":  gstEngine{},
}

// For returns the Publisher that runs the given capture backend.
func For(capture string) (Publisher, error) {
	p, ok := engineByCapture[capture]
	if !ok {
		return nil, fmt.Errorf("unknown capture backend %q", capture)
	}
	return p, nil
}

// Captures lists the capture backends the app can run, sorted for a stable
// order across the wire.
func Captures() []string {
	out := make([]string, 0, len(engineByCapture))
	for capture := range engineByCapture {
		out = append(out, capture)
	}
	slices.Sort(out)
	return out
}

// TransportsFor returns the transports the capture backend's engine can carry,
// in transport registry order. The result is the subset of transport.Names()
// the engine can serialize through, so a capture whose engine lacks a sink for a
// transport (the portal/GStreamer path and WebRTC) excludes it.
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
