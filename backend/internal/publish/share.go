package publish

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/share"
)

// What each capture backend can be pointed at.
//
// A table for the reason cursorServes beside it is one: the same fact is otherwise answered
// by whoever asks.
// It states what the backend's own interface reads.
// Whether this machine can offer the choice is a second question, asked separately,
// a window list being the display server's answer rather than the element's
// (internal/window, Enumerates).

// shareServes is what each capture backend reads, in share.Kinds order.
//
// A backend absent from the map is a bug rather than a backend reading nothing, which init asserts
// against the registry.
// An empty row is a backend that reads what somebody else pointed it at,
// which is a different fact and has its own statement (shareRefusal).
var shareServes = map[string][]string{
	// output_idx selects the output and offset_x, offset_y and video_size crop inside it.
	// Desktop Duplication hands out one output at a time and no window at all.
	"ddagrab": {share.Monitor, share.Region},
	// GDI reads the whole virtual desktop or one window by handle,
	// and crops either with offset_x, offset_y and video_size.
	// The monitor kind is that whole desktop here: the input takes no output to single one out,
	// which is what the monitor control greys with (internal/form, availabilityMonitorless).
	"gdigrab": {share.Monitor, share.Window, share.Region},
	// monitor-index selects the output, crop-x and the three beside it crop inside it,
	// and window-handle reads one window through Windows Graphics Capture.
	"d3d11screencapturesrc": {share.Monitor, share.Window, share.Region},

	// The X screen is one picture and every selection is a rectangle of it,
	// so the monitor kind and the region kind are one mechanism pointed at two things.
	// ximagesrc reads a window by its XID as well, where x11grab reads a rectangle and nothing else.
	"x11grab":   {share.Monitor, share.Region},
	"ximagesrc": {share.Monitor, share.Window, share.Region},

	// kmsgrab reads a scanout, which is one whole output and holds no windows.
	"kmsgrab": {share.Monitor},

	// AVFoundation's screen input picks its own display and takes neither a window nor a rectangle,
	// so the monitor kind is the whole of what it reads (docs/capture-architecture.md).
	"avfoundation": {share.Monitor},
	"avfvideosrc":  {share.Monitor},

	// The compositor draws the picker and answers with whatever was chosen there,
	// so nothing on this side names what is captured.
	"portal": {},
}

// The table describes the same set of backends the registry does, so a row in one and not the other
// is a backend whose reach nobody stated, or an entry for a backend nobody registered.
// Both are bugs in this package, so they fail at load.
func init() {
	assert.Assert(len(shareServes) == len(captureBackends),
		"a capture backend states what it reads exactly once",
		len(shareServes), len(captureBackends))
	assert.Assert(len(shareServes[capturePortal]) == 0,
		"the portal backend names nothing it reads, the compositor answering that", capturePortal)
	for name, serves := range shareServes {
		_, ok := captureBackends[name]
		assert.Assert(ok, "a backend with a reach row is one the registry carries", name)
		for _, kind := range serves {
			assert.Assert(share.Known(kind), "a served share kind is one the settings name", name, kind)
		}
	}
}

// ShareServed reports whether the capture backend reads what this share kind names.
func ShareServed(capture, kind string) bool {
	assert.Assert(share.Known(kind), "a reach question names a kind the settings carry", kind)

	for _, k := range shareServes[capture] {
		if k == kind {
			return true
		}
	}
	return false
}

// ShareKinds is what the capture backend reads, in share.Kinds order,
// and empty for one that reads what somebody else pointed it at.
func ShareKinds(capture string) []string {
	return shareServes[capture]
}

// ShareRefusal is why a backend does not read what a kind names.
//
// Two facts rather than one sentence with the backend's name in it, the two sending a reader
// to different places.
// A backend that reads screens and not windows is answered by picking another kind,
// and one that is pointed at its target by somebody else is answered by going and pointing it.
func ShareRefusal(capture string) screensharev1.TextCode {
	if len(shareServes[capture]) == 0 {
		return screensharev1.TextCode_TEXT_CODE_CAPTURE_ASKS_WHAT_TO_SHARE
	}
	return screensharev1.TextCode_TEXT_CODE_CAPTURE_TAKES_NO_SHARE_KIND
}
