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
)

// The X11 reader.
//
// XQueryPointer answers any client that asks, which is what makes polling the right shape
// here and the wrong one on Wayland: there is nothing to subscribe to and nothing to wait for,
// so the reader holds one connection open and asks it whenever the child wants a position.
//
// The connection is opened once and kept. Opening one per read would put a round trip and an
// authentication handshake on a path that runs hundreds of times a second, and closing it is
// what releases the display server's own handle rather than leaving it to the process exiting.

// NewX11 opens a reader against the display named in the environment, and false where there is
// no X server to open.
//
// A session with no X server is not a failure: it is a Wayland one, where the position comes
// from the capture's own metadata instead, and where the cursor table already refuses this
// mode on the backends that read the screen through X.
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

// Read asks the server where the pointer is on the screen the connection's default root
// belongs to.
//
// The root-relative coordinates are the ones taken, because that is the space a screen capture
// reads in: the window-relative pair XQueryPointer also answers with is relative to the root
// window here, which is the same thing, and the child-window handle it reports is not
// something a viewer has any use for.
func (r *x11Reader) Read() (Position, bool) {
	if r.display == nil {
		return Position{}, false
	}

	var root, child C.Window
	var rootX, rootY, winX, winY C.int
	var mask C.uint
	// The default screen's root: a capture reads one screen, and a multi-head X display
	// composes its outputs onto one root, so the pointer's position on that root is where it
	// is on every output at once.
	same := C.XQueryPointer(r.display, C.XDefaultRootWindow(r.display),
		&root, &child, &rootX, &rootY, &winX, &winY, &mask)

	return Position{
		X: int(rootX),
		Y: int(rootY),
		// XQueryPointer reports false where the pointer is on another screen of the same
		// display, which is a pointer that is not over what this capture is reading.
		Visible: same != 0,
		At:      time.Now(),
	}, true
}

// Close drops the connection, and is safe to call twice.
func (r *x11Reader) Close() {
	if r.display == nil {
		return
	}
	C.XCloseDisplay(r.display)
	r.display = nil
}
