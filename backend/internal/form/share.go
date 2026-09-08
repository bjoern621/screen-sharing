package form

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/share"
	"bjoernblessin.de/screenshare/internal/window"
)

// What the share group's controls decide, and what each of them is held against.
//
// Two facts rule a kind out, and they send a reader to different places.
// The capture backend states what its own interface reads, which another backend answers
// (publish.ShareServed).
// The session states whether this machine can enumerate what a kind names,
// which no setting on this screen changes (window.Enumerates).

// shareKindState greys the whole control on a backend that reads nothing this side names,
// the desktop portal being the case that exists:
// the compositor draws the picker and answers with whatever was chosen there.
func shareKindState(av availability) state {
	if len(publish.ShareKinds(av.s.Publish.Capture)) == 0 {
		return availabilityDisabled(say(captureAsksWhatToShare, argCapture(av.s.Publish.Capture)))
	}
	return availabilityLive()
}

// shareKindReason is why one kind is out of reach, and nil where it is reachable.
func shareKindReason(av availability, kind string) *screensharev1.Text {
	if !share.Known(kind) {
		return nil
	}
	if !publish.ShareServed(av.s.Publish.Capture, kind) {
		return say(captureTakesNoShareKind, argCapture(av.s.Publish.Capture), argShareKind(kind))
	}
	// The session's own statement rather than one built here,
	// the same one the catalog carries so a surface writes one sentence for it (window.Statement).
	if kind == share.Window {
		if gap := window.Statement(av.deps.Platform); gap != nil {
			return gap
		}
	}
	return nil
}

// shareTargetState draws the control the picked kind names and hides the other two.
//
// Hidden rather than greyed, the treatment a knob of one mode takes:
// a monitor list under a window capture decides nothing,
// and a reader learns nothing from reading why (docs/field-availability.md).
func shareTargetState(av availability, kind string) state {
	if av.s.Publish.ShareKind != kind {
		return availabilityHidden()
	}
	return availabilityLive()
}
