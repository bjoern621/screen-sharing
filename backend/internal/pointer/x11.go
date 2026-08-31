//go:build linux

package pointer

/*
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <stdlib.h>
*/
import "C"

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// XQueryPointer answers any client that asks, with nothing to subscribe to, so the reader polls.
//
// One connection opened and kept: one per read would put a round trip and an authentication
// handshake on a path running hundreds of times a second.
// Close releases the display server's own handle rather than leaving it to the process exiting.

// NewX11 opens a reader against the display named in the environment,
// false where there is no X server to open.
//
// No X server is a Wayland session rather than a failure: the position comes from the capture's own
// metadata there, and the cursor table already refuses this mode on the backends reading the screen
// through X.
func NewX11() (Reader, bool) {
	display := C.XOpenDisplay(nil)
	if display == nil {
		return nil, false
	}
	return &x11Reader{display: display}, true
}

type x11Reader struct {
	display *C.Display
}

// Read asks the server where the pointer is on the screen the connection's default root belongs to.
//
// Root-relative coordinates, that being the space a screen capture reads in.
// The window-relative pair XQueryPointer answers with is relative to the root window here,
// so it is the same thing,
// and the child-window handle is of no use to a viewer.
//
// A closed reader answers false rather than asserting,
// a poll outliving a Close being a race rather than a broken contract.
func (r *x11Reader) Read() (Position, bool) {
	if r.display == nil {
		return Position{}, false
	}

	var root, child C.Window
	var rootX, rootY, winX, winY C.int
	var mask C.uint
	// A multi-head X display composes its outputs onto one default root,
	// so a position on that root is where the pointer is on every output at once.
	same := C.XQueryPointer(r.display, C.XDefaultRootWindow(r.display),
		&root, &child, &rootX, &rootY, &winX, &winY, &mask)

	at := Position{
		X: int(rootX),
		Y: int(rootY),
		// XQueryPointer reports false where the pointer sits on another screen of the same display,
		// which is not over what this capture reads.
		Visible: same != 0,
		At:      time.Now(),
	}
	assert.Assert(!at.At.IsZero(), "a read position carries the instant it was read", at.X, at.Y)
	return at, true
}

// Close drops the connection and is safe to call twice.
func (r *x11Reader) Close() {
	if r.display == nil {
		return
	}
	C.XCloseDisplay(r.display)
	r.display = nil
}
