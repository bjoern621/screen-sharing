package publish

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/text"
)

// What each capture backend needs of the platform and the session before it can run.
//
// It lives here, beside the registry that pairs a backend with its engine, because a
// backend that cannot run on this machine and a backend that has no engine are the
// same question asked twice: both are answers about whether the row is reachable, and
// splitting them leaves one of them to be restated by whoever asks second. It was the
// frontend that restated it - the table was CAPTURE_NEEDS in util/deps.ts - which is
// the drift docs/ipc-api.md exists to end.
//
// A gate is stated once per requirement rather than once per row that carries it:
// x11grab and ximagesrc read the X screen through the same extension and differ only
// in which publish engine runs them, so they carry the same session gate rather than
// two spellings of it.

// needs is one backend's requirement.
type needs struct {
	// os is the operating system the backend runs on, as platform.Info spells it.
	os string
	// display is the Linux display server the backend needs, empty for a backend with
	// no session gate. The portal has none on purpose: xdg-desktop-portal serves the
	// ScreenCast interface on X11 sessions too, so the backend is offered on either and
	// the publish carries the portal's own error where the desktop has no backend for it.
	display string
	// grant reports whether the backend needs a privilege nothing here can establish.
	// It is not a reason the backend is unavailable: the process either has the
	// privilege or the capture dies at launch, and no probe tells which in advance.
	//
	// What the privilege is, and what a machine on the wrong operating system or in
	// the wrong session is missing, are statements assembled from this row rather than
	// sentences stored in it: the capture backend, the operating system it needs and
	// the session it needs are the three columns above, and a surface holding those
	// three writes the sentence at its own length (internal/text).
	grant bool
}

// captureNeed pairs a backend with its requirement, in the order a caller picking one
// on the user's behalf tries them: from the backend a desktop session normally has to
// the ones that ask something of it.
//
// It is a slice and not a map because that order is part of the answer, and a map has
// none.
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

// The two tables describe one set of backends, so a row in either without a row in the
// other is a backend that is registered and ungated or gated and unregistered. Both
// are bugs in this package rather than conditions to survive, so they fail at load.
func init() {
	assert.Assert(len(captureNeeds) == len(captureBackends),
		"a capture backend is registered and gated exactly once", len(captureNeeds), len(captureBackends))
	for _, n := range captureNeeds {
		_, ok := captureBackends[n.capture]
		assert.Assert(ok, "a gated capture backend has a publisher", n.capture)
		assert.Assert(n.needs.os != "", "a capture backend names the operating system it runs on", n.capture)
	}
}

// gatedOperatingSystems are the operating systems the table names. One outside the set
// runs no backend this app knows, so it restricts none of them rather than blocking
// every one: a machine the table has never heard of is a machine this package has
// nothing true to say about, and saying nothing is what leaves the choice with the
// user rather than with a rule that was never written for them.
func gatedOperatingSystems() map[string]bool {
	out := make(map[string]bool, len(captureNeeds))
	for _, n := range captureNeeds {
		out[n.needs.os] = true
	}
	return out
}

// Available reports whether the capture backend can run on this platform and session,
// and the sentence saying why not when it cannot.
//
// An unknown backend is a caller that made one up: every name reaching here comes off
// the registry or off settings the registry validated, so it is asserted rather than
// reported. A privilege the process may or may not hold is deliberately not an
// unavailability - see Grant.
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

// AudioAvailable reports whether a pipeline built on this capture backend can record
// the named second-track source, and the sentence saying what is missing where it
// cannot.
//
// The platform is read off the backend rather than off the machine, which is the whole
// reason this sits here and not beside the other two audio reads. A pipeline naming
// ddagrab is a Windows pipeline wherever it is rendered: both engines build their
// arguments from the settings alone so that the render is testable on any machine and
// the displayed command is the one the publish button starts. Asking the running
// machine instead would make a Windows pipeline rendered on Linux record an audio track
// no Windows session can produce.
//
// The verdict itself is platform.AudioSourceAvailable's, and the sentence is the one
// the source table already states for that operating system. What is added here is the
// single derivation neither package can make alone - capture backend to the operating
// system it runs on - which is the column captureNeeds already holds. Before this, both
// engines carried the sentence per backend, so one source's absence was written four
// times and the greying a form showed came from a fifth.
func AudioAvailable(capture, audio string) (bool, *screensharev1.Text) {
	need, ok := needsOf(capture)
	assert.Assert(ok, "an asked-about capture backend is a registered one", capture)

	return platform.AudioSourceAvailable(audio, platform.Info{OS: need.os})
}

// Grant is the privilege the capture backend needs and no probe can establish,
// empty for a backend behind none.
//
// A backend behind one stays selectable by hand, where the choice is the user's and
// the failure is theirs to read. It is left out of the set something picks on their
// behalf, which is what AutoCaptures is for.
func Grant(capture string) *screensharev1.Text {
	need, ok := needsOf(capture)
	assert.Assert(ok, "an asked-about capture backend is a registered one", capture)

	if !need.grant {
		return nil
	}
	return text.Of(screensharev1.TextCode_TEXT_CODE_CAPTURE_NEEDS_GRANT,
		text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE, capture))
}

// AutoCaptures lists the backends this platform runs that need nothing granted first,
// in table order, which is the order a caller picking one on the user's behalf tries
// them.
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

// needsOf reads one row of the gate table.
func needsOf(capture string) (needs, bool) {
	for _, n := range captureNeeds {
		if n.capture == capture {
			return n.needs, true
		}
	}
	return needs{}, false
}
