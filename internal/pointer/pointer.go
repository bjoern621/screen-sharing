// Package pointer reads where the mouse is, for the capture backends whose display server
// will answer.
//
// The position travels beside the stream rather than being drawn into it, which is what the
// metadata cursor mode is: a viewer draws it itself, sharp at any scale, and it costs the
// encoder nothing (docs/capture-architecture.md, "The pointer").
//
// Who can answer is a property of the display server and not of this app. X11 hands any
// client the pointer's position on request, so the X11 capture backends read it here. A
// Wayland client cannot ask at all - the position outside a client's own surfaces is not
// something the protocol exposes - so on that session the answer comes from the cursor
// metadata PipeWire carries beside each frame, which only the process holding the capture can
// read, and that is why this is the publish child's job rather than the backend's.
package pointer

import "time"

// Position is where the pointer was, and when.
//
// The coordinates are the display server's own, in its pixels and its origin. Nothing here
// converts them into the captured picture's: which screen a capture is reading and how it was
// scaled are the publish settings' facts, and a reader that has both is the one that can
// place a pointer on a picture (viewer-architecture.md).
type Position struct {
	// X and Y are where it is, in the display server's pixels.
	X, Y int
	// At is when it was read, which is what lets a viewer hold the position back to the
	// frame it belongs to rather than letting it lead the picture.
	At time.Time
	// Visible reports whether the pointer is over the captured surface at all. A pointer
	// that has left the screen is not at its last position, and drawing it there would leave
	// one stuck against an edge for as long as it is away.
	Visible bool
}

// Reader answers where the pointer is.
//
// An interface because the answer comes from the display server, which differs per session
// and per platform. A reader that cannot answer is not an error: it is a session that does
// not expose the position, which the cursor table already refuses the mode on.
type Reader interface {
	// Read is where the pointer is now, and false where this session will not say.
	Read() (Position, bool)
	// Close releases whatever the reader holds open, and is safe to call twice.
	Close()
}

// Interval is how often a reader is asked.
//
// It is deliberately not the frame rate. Binding the two would throw away the reason to draw
// the pointer client-side at all: a position costs no frame, so a 240 Hz pointer over a 30 fps
// stream is the whole win, and the timestamp on each is what lets a viewer hold one back if
// leading the picture looks wrong.
//
// 4 ms is faster than any panel this app has met and slower than the rate at which reading it
// costs anything measurable.
const Interval = 4 * time.Millisecond
