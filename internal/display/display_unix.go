//go:build !windows

package display

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/platform"
)

// List enumerates the machine's monitors, taking the first enumerator for the active session that
// reports one.
// Where none do, which is a session with no graphical login, a missing enumeration tool or a
// compositor no probe covers, the single entry returned carries zero width and height: the
// resolution is unknown, not zero.
func List() []Monitor {
	for _, enumerate := range providersFor(platform.Detect().Display) {
		if monitors := enumerate(); len(monitors) > 0 {
			return monitors
		}
	}
	return []Monitor{{Index: 0, Primary: true}}
}

// providersFor is the monitor enumerators to try for a display server, in order.
// A Wayland session ends on the X11 enumerator, so an XWayland-backed setup still reports its
// outputs.
// The empty server is what platform.Detect answers off Linux and in a headless session, and it has
// no enumerator: List falls through to its placeholder.
//
// Those three are the values platform.Detect resolves a session to.
// Any other name came from a caller that read the display server elsewhere, an Entwicklungsfehler:
// answering it with no enumerator would put a real session behind the headless placeholder.
func providersFor(server string) []func() []Monitor {
	switch server {
	case "wayland":
		return []func() []Monitor{listWayland, listX11}
	case "x11":
		return []func() []Monitor{listX11}
	case "":
		return nil
	default:
		assert.Never("unexpected display server", server)
		return nil
	}
}
