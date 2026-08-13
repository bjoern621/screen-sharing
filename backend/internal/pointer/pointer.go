// Package pointer reads where the mouse is, for the capture backends whose display server answers.
//
// The position travels beside the stream rather than being drawn into it,
// which is what the metadata cursor mode is: a viewer draws it itself, sharp at any scale, and it
// costs the encoder nothing (docs/capture-architecture.md, "The pointer").
//
// Who can answer is the display server's property, not this app's.
// X11 hands any client the pointer's position on request, so the X11 capture backends read it here.
// A Wayland client cannot ask: the protocol exposes no position outside a client's own surfaces.
// There the answer rides in the cursor metadata PipeWire carries beside each frame,
// readable only by the process holding the capture,
// which is why this is the publish child's job and not a backend's.
package pointer

import "time"

// Position is where the pointer was, and when.
//
// The coordinates are the display server's own, in its pixels and from its origin.
// Nothing here converts them into the captured picture's space:
// which screen a capture reads and how it was scaled are the publish settings' facts,
// and a reader holding both is the one that can place a pointer on a picture
// (viewer-architecture.md).
type Position struct {
	X, Y int
	// At is when it was read, which lets a viewer hold the position back to the frame it belongs to
	// rather than letting it lead the picture.
	At time.Time
	// Visible reports whether the pointer is over the captured surface at all.
	// A pointer that has left the screen is not at its last position, and drawing it there sticks one
	// against an edge for as long as it is away.
	Visible bool
}

// Reader answers where the pointer is.
//
// An interface because the answer comes from the display server, which differs per session and per
// platform.
// A reader that cannot answer is not an error: it is a session exposing no position, and the cursor
// table already refuses the mode there.
type Reader interface {
	// Read is where the pointer is now, and false where this session will not say.
	Read() (Position, bool)
	// Close releases whatever the reader holds open, and is safe to call twice.
	Close()
}

// Interval is how often a reader is asked.
//
// Deliberately not the frame rate.
// A position costs no frame,
// so a 240 Hz pointer over a 30 fps stream is the whole reason to draw it client-side,
// and the timestamp on each lets a viewer hold one back where leading the picture looks wrong.
//
// 4 ms is faster than any panel this app has met and slower than the rate at which reading costs
// anything measurable.
const Interval = 4 * time.Millisecond
