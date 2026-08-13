package publish

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/text"
)

// What each capture backend needs of the platform and the session before it can run, and what a
// machine missing it is greyed with (docs/field-availability.md).
//
// It sits beside the registry pairing a backend with its engine, because a backend that cannot run
// here and a backend with no engine are the same question asked twice,
// and splitting them leaves one to be restated by whoever asks second.
//
// A gate is stated once per requirement rather than once per row carrying it: x11grab and ximagesrc
// read the X screen through the same extension and differ only in which publish engine runs them,
// so they share one session gate rather than two spellings of it.

// needs is one backend's requirement.
type needs struct {
	// os is the operating system the backend runs on, as platform.Info spells it.
	os string
	// display is the Linux display server the backend needs, empty for a backend with no session gate.
	// The portal has none on purpose: xdg-desktop-portal serves ScreenCast on X11 sessions too, so the
	// backend is offered on either and a desktop with no backend for it fails with the portal's own
	// error.
	display string
	// grant reports whether the backend needs a privilege nothing here can establish.
	// Not a reason the backend is unavailable: the process either holds the privilege or the capture
	// dies at launch, and no probe tells which in advance.
	//
	// What the privilege is, and what a machine on the wrong operating system or in the wrong session
	// is missing, are statements assembled from the three columns above rather than sentences stored
	// in them, so a surface writes each at its own length (internal/text).
	grant bool
}

// captureNeed pairs a backend with its requirement, in the order a caller picking one on the user's
// behalf tries them: the backend a desktop session normally has first, the ones asking something of
// it after.
//
// A slice and not a map, because the order is part of the answer.
type captureNeed struct {
	capture string
	needs   needs
}

var captureNeeds = []captureNeed{
	{"ddagrab", needs{os: "windows"}},
	{"gdigrab", needs{os: "windows"}},
	{"d3d11screencapturesrc", needs{os: "windows"}},
	{"x11grab", needs{os: "linux", display: "x11"}},
	{"ximagesrc", needs{os: "linux", display: "x11"}},
	{"kmsgrab", needs{os: "linux", grant: true}},
	{"portal", needs{os: "linux"}},
	{"avfoundation", needs{os: "darwin"}},
	{"avfvideosrc", needs{os: "darwin"}},
}

// The two tables describe one set of backends, so a row in either without its counterpart is a
// backend registered and ungated, or gated and unregistered.
// Both are bugs in this package rather than conditions to survive, so they fail at load.
func init() {
	assert.Assert(len(captureNeeds) == len(captureBackends),
		"a capture backend is registered and gated exactly once", len(captureNeeds), len(captureBackends))
	for _, n := range captureNeeds {
		_, ok := captureBackends[n.capture]
		assert.Assert(ok, "a gated capture backend has a publisher", n.capture)
		assert.Assert(n.needs.os != "", "a capture backend names the operating system it runs on", n.capture)
	}
}

// gatedOperatingSystems are the operating systems the table names.
// One outside the set runs no backend this app knows, so it restricts none of them rather than
// blocking every one: this package has nothing true to say about such a machine, and saying nothing
// leaves the choice with the user rather than with a rule never written for them.
func gatedOperatingSystems() map[string]bool {
	out := make(map[string]bool, len(captureNeeds))
	for _, n := range captureNeeds {
		out[n.needs.os] = true
	}
	return out
}

// Available reports whether the capture backend runs on this platform and session, and what is
// missing where it does not.
//
// An unknown backend is asserted rather than reported: every name reaching here comes off the
// registry or off settings the registry validated.
// A privilege the process may or may not hold is deliberately not an unavailability, see Grant.
func Available(capture string, p platform.Info) (bool, *screensharev1.Text) {
	need, ok := needsOf(capture)
	assert.Assert(ok, "an asked-about capture backend is a registered one", capture)

	if !gatedOperatingSystems()[p.OS] {
		return true, nil
	}
	if p.OS != need.os {
		return false, text.Of(screensharev1.TextCode_TEXT_CODE_CAPTURE_WRONG_OS,
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE, capture),
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OS, need.os))
	}
	if need.display != "" && p.Display != need.display {
		return false, text.Of(screensharev1.TextCode_TEXT_CODE_CAPTURE_WRONG_SESSION,
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE, capture),
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_DISPLAY, need.display))
	}
	return true, nil
}

// AudioAvailable reports whether a pipeline built on this capture backend can record the named
// second-track source, and what is missing where it cannot.
//
// The platform is read off the backend rather than off the machine, which is why this sits here.
// A pipeline naming ddagrab is a Windows pipeline wherever it is rendered, since both engines build
// their arguments from the settings alone, so a render is testable on any machine and the displayed
// command is the one the publish button starts.
// Asking the running machine instead would let a Windows pipeline rendered on Linux record a track
// no Windows session can produce.
//
// The verdict is platform.AudioSourceAvailable's and the statement is the source table's.
// What is added is the one derivation neither package can make alone, capture backend to the
// operating system it runs on, which is the column captureNeeds holds.
func AudioAvailable(capture, audio string) (bool, *screensharev1.Text) {
	need, ok := needsOf(capture)
	assert.Assert(ok, "an asked-about capture backend is a registered one", capture)

	if available, reason := platform.AudioSourceAvailable(audio, platform.Info{OS: need.os}); !available {
		return false, reason
	}

	// One kind is an engine's question as well as a platform's.
	// A program playing sound is a PipeWire node, and only the GStreamer engine has an element that
	// opens one: ffmpeg's pulse input takes a device, and PulseAudio cannot record one program's
	// stream at all.
	// A capture backend fixes the engine, which is what makes this the one place able to say it.
	if audio == platform.AudioSourceApplication && engineOf(capture) != EngineGst {
		return false, text.Of(screensharev1.TextCode_TEXT_CODE_AUDIO_SOURCE_UNSERVED_BY_ENGINE,
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_AUDIO, audio),
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_ENGINE, engineOf(capture)),
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OTHER_ENGINE, EngineGst))
	}
	return true, nil
}

// Grant is the privilege the capture backend needs and no probe can establish, nil for a backend
// behind none.
//
// A backend behind one stays selectable by hand, where the choice is the user's and so is the
// failure, and it stays out of the set something picks on their behalf (AutoCaptures).
func Grant(capture string) *screensharev1.Text {
	need, ok := needsOf(capture)
	assert.Assert(ok, "an asked-about capture backend is a registered one", capture)

	if !need.grant {
		return nil
	}
	return text.Of(screensharev1.TextCode_TEXT_CODE_CAPTURE_NEEDS_GRANT,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE, capture))
}

// AutoCaptures lists the backends this platform runs that need nothing granted first, in table
// order, which is the order a caller picking one on the user's behalf tries them.
func AutoCaptures(p platform.Info) []string {
	out := make([]string, 0, len(captureNeeds))
	for _, n := range captureNeeds {
		if available, _ := Available(n.capture, p); !available {
			continue
		}
		if n.needs.grant {
			continue
		}
		out = append(out, n.capture)
	}
	return out
}

// needsOf reads one row of the gate table, false where no row names the backend.
func needsOf(capture string) (needs, bool) {
	for _, n := range captureNeeds {
		if n.capture == capture {
			return n.needs, true
		}
	}
	return needs{}, false
}

// engineOf is the publish engine a capture backend runs, empty for one no publisher carries.
// EngineFor's lookup without the error, since every caller here holds a backend the registry named.
func engineOf(capture string) string {
	engine, err := EngineFor(capture)
	if err != nil {
		return ""
	}
	return engine
}
