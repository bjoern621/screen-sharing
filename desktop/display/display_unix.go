//go:build !windows

package display

import "bjoernblessin.de/screenshare/platform"

// List enumerates the machine's monitors on non-Windows platforms. It dispatches
// to the enumerators matching the active session and returns the first that finds
// a monitor. When none succeed (no graphical session, a missing enumeration tool,
// or an unsupported compositor) it returns a single placeholder whose zero width
// and height tell the UI the resolution is unknown.
func List() []Monitor {
	for _, enumerate := range providersFor(platform.Detect().Display) {
		if monitors := enumerate(); len(monitors) > 0 {
			return monitors
		}
	}
	return []Monitor{{Index: 0, Primary: true}}
}

// providersFor orders the monitor enumerators to try for a display server. A
// Wayland session falls back to the X11 enumerator so XWayland-backed setups
// still report their outputs. An unknown or empty server (a non-Linux platform,
// or a headless session) has no enumerator and falls through to the placeholder.
func providersFor(server string) []func() []Monitor {
	switch server {
	case "wayland":
		return []func() []Monitor{listWayland, listX11}
	case "x11":
		return []func() []Monitor{listX11}
	default:
		return nil
	}
}
