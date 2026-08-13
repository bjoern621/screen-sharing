//go:build !windows

package display

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/platform"
)

// List enumerates the machine's monitors on non-Windows platforms.
// It dispatches to the enumerators matching the active session and returns the first that finds a
// monitor.
// Where none succeed, which is a session with no graphical login, a missing enumeration tool or an
// unsupported compositor, it returns a single placeholder whose zero width and height tell the UI
// the resolution is unknown.
func List() []Monitor {
	for _, enumerate := range providersFor(platform.Detect().Display) {
		if monitors := enumerate(); len(monitors) > 0 {
			return monitors
		}
	}
	return []Monitor{{Index: 0, Primary: true}}
}

// providersFor orders the monitor enumerators to try for a display server.
// A Wayland session falls back to the X11 enumerator so XWayland-backed setups still report their
// outputs.
// The empty server is what platform.Detect reports off Linux and in a headless session,
// and it has no enumerator: List answers with its placeholder.
//
// Those three are what platform.Detect resolves the session environment to.
// Any other name comes from a caller that read the display server somewhere else,
// which is an Entwicklungsfehler: answering it with no enumerator would put a real session behind
// the placeholder a headless one gets.
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
