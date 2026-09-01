// Package pointer reads where the mouse is, for the capture backends whose display server answers.
//
// The position travels beside the stream rather than being drawn into it, the metadata cursor mode:
// a viewer draws it itself, sharp at any scale and costing the encoder nothing
// (docs/capture-architecture.md, "The pointer").
//
// Who can answer is the display server's property.
// X11 hands any client the pointer's position on request, so the X11 capture backends read it here.
// A Wayland client cannot ask: the protocol exposes no position outside a client's own surfaces.
// There the answer rides in the cursor metadata PipeWire carries beside each frame,
// readable only by the process holding the capture, so it is taken off the frames (gstrun/pointer.go).
package pointer

import "time"

// Position is where the pointer was, and when.
//
// Coordinates are the display server's own, in its pixels and from its origin.
// Nothing here converts them into the captured picture's space:
// which screen a capture reads and how it was scaled are the publish settings' facts,
// and a reader holding both is what places a pointer on a picture (viewer-architecture.md).
type Position struct {
	X, Y int
	// At is when it was read,
	// which lets a viewer hold the position back to the frame it belongs to,
	// rather than letting it lead the picture.
	At time.Time
	// Visible reports whether the pointer is over the captured surface at all.
	// A pointer that has left the screen is not at its last position, and drawing it there sticks one
	// against an edge for as long as it is away.
	Visible bool
}

// Spot is where the pointer is on the captured picture, as a fraction of it.
//
// A fraction and not a pixel, so nothing downstream needs the size anything was read or drawn at:
// the publish scales the picture on the way out and a viewer draws it at a size of its own,
// and a fraction survives both.
// It is also the one space both capture backends can answer in.
// X11 reports against the display server's root and the portal against the region it captures,
// so a pixel crossing a process boundary would mean two things (gstrun/pointer.go).
//
// X and Y are 0..1 from the picture's top left, and are read only where Visible holds.
type Spot struct {
	X, Y float64
	// At is when the position was read.
	// It lets a viewer hold one back to the frame it belongs to rather than letting it lead the picture.
	At time.Time
	// Visible reports whether the pointer is over the captured picture at all.
	// A pointer that has left is not at its last position,
	// and drawing it there sticks one against an edge for as long as it is away.
	Visible bool
}

// Reader answers where the pointer is.
// A reader that cannot answer is not an error: it is a session exposing no position,
// and the cursor table already refuses the mode there.
type Reader interface {
	// Read is where the pointer is now, and false where this session will not say.
	Read() (Position, bool)
	// Close releases whatever the reader holds open, and is safe to call twice.
	Close()
}

// Interval is how often a reader is asked, not the frame rate.
// A position costs no frame,
// so a 240 Hz pointer over a 30 fps stream is the reason to draw it client-side,
// and the timestamp on each lets a viewer hold one back where leading the picture looks wrong.
//
// 4 ms is faster than any panel this app has met and slower than the rate at which reading costs
// anything measurable.
const Interval = 4 * time.Millisecond
