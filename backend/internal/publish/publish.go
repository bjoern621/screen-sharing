// Package publish selects and supervises the pipeline that captures the screen and pushes
// the encoded stream to the relay.
//
// The boundary is the Publisher: the app starts and stops a Publisher and never names ffmpeg or
// GStreamer.
// A capture backend owns its whole pipeline behind that contract.
// One publish engine drives the screen grabbers feeding a single ffmpeg process, the other drives
// the capture backends where GStreamer captures, encodes and ships in one graph.
// Both satisfy the same contract, so the lifecycle code above the boundary is identical for either.
package publish

import (
	"fmt"
	"reflect"
	"slices"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// Stats is one encoder progress sample surfaced to the UI.
// The ffmpeg progress shape is the wire format, and an engine with no equivalent progress stream
// measures the same figures its own way (gstMeter).
// A figure the pipeline exposes no measurement for is marked in Missing rather than sent as a zero,
// zero being the reading that marks a stalled encoder.
type Stats = ffmpeg.Stats

// Missing is the set of Stats figures a sample carries no measurement for.
type Missing = ffmpeg.Missing

// Handle supervises one running publish session.
type Handle interface {
	Running() bool
	// Stop terminates the pipeline, and a requested stop is not a failure.
	Stop()
}

// Callbacks receive progress and the terminal exit of a publish session.
// OnStats is best-effort.
// OnExit fires exactly once, with a non-nil error only on unexpected failure,
// the tail of stderr and the path of the run log.
type Callbacks struct {
	OnStats func(Stats)
	OnExit  func(err error, stderrTail string, logPath string)
	// OnPointer receives where the pointer is, for a publish whose cursor mode sends the position
	// instead of drawing it into the frames.
	// It fires at the reader's own rate, faster than the frame rate, and never on any other mode or
	// any engine whose child cannot report one (internal/pointer).
	OnPointer func(pointer.Spot)
}

// Publisher builds and runs the publish pipeline for a family of capture backends.
// Command renders the pipeline for the UI without running it.
type Publisher interface {
	Command(s settings.Settings) (string, error)
	// Start runs the pipeline.
	// preview is the loopback leg the child copies its already-encoded video to for the local preview,
	// and its zero value is a run with none, which a rendered command always is: the port belongs
	// to one launch (preview.go).
	Start(s settings.Settings, tag string, preview PreviewLeg, cb Callbacks) (Handle, error)
	// Engine names the publish engine that runs the pipeline, "ffmpeg" or "gstreamer".
	// The lifecycle code above the boundary never reads it, and the settings form does: which rate-control
	// knobs reach the encoder differs per engine, and a control the engine ignores is greyed
	// with that reason instead of silently doing nothing.
	Engine() string
	// Carries reports whether the transport implements the publish capability this engine serializes
	// through.
	// The ffmpeg engine needs an FFmpegPublisher and the GStreamer engine a GstPublisher, so
	// a GStreamer capture backend cannot carry a transport with no GStreamer sink.
	Carries(transportName string) bool
}

// The publish engines that run the capture backends.
// An engine names itself to capabilities.Validate, to the encoder probe and to the settings form,
// so the spelling is the capability table's own rather than a second one able to drift
// from the engine a Gap names.
const (
	EngineFfmpeg = capabilities.EngineFfmpeg
	EngineGst    = capabilities.EngineGst
)

// captureBackends is the single source pairing a capture backend with the engine that runs it.
// Capture backend and publish engine are two axes rather than one: a screen source is a row here,
// and the engine a row names follows from which framework has an element or input device for it,
// not from a property of the engine.
//
// A screen both frameworks read is two rows, one per engine, each named as its own framework names
// the source: the macOS screen is avfoundation under ffmpeg and avfvideosrc under GStreamer,
// the Windows desktop ddagrab or gdigrab under ffmpeg and d3d11screencapturesrc under GStreamer.
//
// A row with one engine is a source the other framework has nothing for.
// ffmpeg has no PipeWire input device, so the portal is GStreamer's alone.
// GStreamer has no capture element for DRM/KMS scanout buffers, so kmsgrab is ffmpeg's.
var captureBackends = map[string]Publisher{
	"ddagrab":      ffmpegEngine{},
	"gdigrab":      ffmpegEngine{},
	"x11grab":      ffmpegEngine{},
	"kmsgrab":      ffmpegEngine{},
	"avfoundation": ffmpegEngine{},
	// The one row whose backend acquires something, and the hold is where what it acquired lives
	// between the launches of one stream (ReleaseSources).
	"portal":                gstEngine{capture: portalCapture{hold: &portal.Hold{}}},
	"ximagesrc":             gstEngine{capture: ximageCapture{}},
	"avfvideosrc":           gstEngine{capture: avfCapture{}},
	"d3d11screencapturesrc": gstEngine{capture: d3d11Capture{}},
}

// sourceHolder is a backend keeping an acquired screen source between the launches of one stream.
//
// A capability discovered by assertion rather than a method on Publisher, the way a child taking
// values while it plays is (live.go).
// Acquiring anything at all is the portal's alone: every other backend opens its source
// from the element or the input device and holds nothing between runs, and a Release on each
// of them would be four methods that do nothing.
type sourceHolder interface {
	// Release drops the held source, and succeeds where nothing is held.
	Release()
}

// ReleaseSources drops the screen source a backend holds between the launches of one stream.
//
// Called where no publish is in force, which is the whole of a hold's life: a relaunch and a retry
// both pass through a point with the stream still in force and must find the source still held,
// while a source kept past the last child leaves the compositor sharing a screen nobody receives.
// Idempotent, and a backend that acquires nothing has nothing to drop.
func ReleaseSources() {
	ReleaseSourcesExcept("")
}

// ReleaseSourcesExcept drops what every backend but the named one holds.
//
// Called at a launch, which is where the stream can move to another capture backend: the one it
// moved off would otherwise keep a source no child reads, and for the portal that is a screen
// the compositor goes on sharing for the rest of the stream.
// A backend named here that holds nothing, and a name no row carries, both release nothing.
func ReleaseSourcesExcept(capture string) {
	for name, p := range captureBackends {
		if name == capture {
			continue
		}
		if holder, ok := p.(sourceHolder); ok {
			holder.Release()
		}
	}
}

func For(capture string) (Publisher, error) {
	p, ok := captureBackends[capture]
	if !ok {
		return nil, fmt.Errorf("unknown capture backend %q", capture)
	}
	return p, nil
}

// Command renders the pipeline the settings publish through, without running it.
// The engine owning the selected capture backend renders it, so the result is an ffmpeg command
// line or a gst-launch pipeline.
//
// Every check a publish is refused for runs here, the command being what a run executes.
// A caller holding settings against the engines needs no second validation path, and the line
// the UI displays is one the publish button can start.
func Command(s settings.Settings) (string, error) {
	p, err := For(s.Publish.Capture)
	if err != nil {
		return "", err
	}
	return p.Command(s)
}

// SamePipeline reports whether two settings publish through the same pipeline, which says whether
// moving from one to the other needs the child process relaunched.
//
// The rendered command is the whole of what an engine hands its child, and both engines render it
// from the settings alone, so a field no builder reads cannot change the string and one a builder
// reads always does.
// Comparing the render is therefore the question itself.
// A table of which fields matter would be a second statement of it, and would fall behind
// the builders the first time one of them read a field the table does not name.
//
// Settings neither engine can render carry the reason instead of an answer: they name a pipeline
// that cannot run, so whether some other pipeline equals it is not a question with one.
// Identical settings are answered without a render, since they build one pipeline whether or not
// that pipeline is buildable.
func SamePipeline(before, after settings.Settings) (bool, error) {
	// The one field a builder reads that is not part of what the pipeline is.
	// Minted per start and good for minutes, so comparing it would make every repeat a different
	// pipeline: a start naming the running stream would relaunch it, and a shell could not repeat
	// a request whose answer went missing.
	before.Relay.Token, after.Relay.Token = "", ""

	if reflect.DeepEqual(before, after) {
		return true, nil
	}
	beforeCmd, err := Command(before)
	if err != nil {
		return false, err
	}
	afterCmd, err := Command(after)
	if err != nil {
		return false, err
	}
	return beforeCmd == afterCmd, nil
}

// Captures lists the capture backends the app runs, sorted for a stable order across the wire.
func Captures() []string {
	out := make([]string, 0, len(captureBackends))
	for capture := range captureBackends {
		out = append(out, capture)
	}
	slices.Sort(out)
	return out
}

// Engines lists the publish engines that run the capture backends, sorted for a stable order.
// Each answers capabilities.Validate under its own name and is probed separately for the codecs it
// runs here, the two wrapping different encoder implementations.
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

func EngineFor(capture string) (string, error) {
	p, err := For(capture)
	if err != nil {
		return "", err
	}
	return p.Engine(), nil
}

// RateCeilingMbps is the rate an encode on these settings is held to, and false where it is bounded
// by nothing.
//
// What counts as a ceiling is the mode's answer: a bitrate held every second is its own ceiling,
// a constrained burst is the figure above the target, and a quality target has one only where
// the element holds it inside a rate buffer (capabilities.QualityCeiling).
// An average target bursts freely and a lossless encode costs what exactness costs, so neither has
// one.
//
// One answer for the whole app: the plot's rule, the diagnostics and the encoder read this rather
// than each deriving a bound from the settings, and a surface deriving its own would draw one
// nothing enforces.
func RateCeilingMbps(s settings.Settings) (float64, bool) {
	switch s.Publish.Mode {
	case capabilities.ModeCbr:
		return float64(s.Publish.BitrateM), s.Publish.BitrateM > 0
	case capabilities.ModeVbr:
		return float64(s.Publish.MaxrateM), s.Publish.MaxrateM > 0
	case capabilities.ModeCrf:
		engine, err := EngineFor(s.Publish.Capture)
		if err != nil || !capabilities.QualityCeiling(s.Publish.Codec(), engine) {
			return 0, false
		}
		return float64(s.Publish.MaxrateM), s.Publish.MaxrateM > 0
	}
	return 0, false
}

// TransportsFor returns the transports the capture backend's engine carries, in transport registry
// order.
// The result is the subset of transport.Names() that engine serializes through, so a capture whose
// engine has no sink for a transport leaves it out.
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
