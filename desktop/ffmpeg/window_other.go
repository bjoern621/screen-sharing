//go:build !windows

package ffmpeg

import (
	"bytes"
	"os/exec"
)

// WindowExists reports whether a window whose title matches title exists.
//
// Detection is best-effort and relies on xdotool (X11, or XWayland-backed
// players under Wayland). When xdotool is absent the readiness signal never
// fires and the caller falls back to its timeout; the stream itself is
// unaffected.
func WindowExists(title string) bool {
	out, err := exec.Command("xdotool", "search", "--name", title).Output()
	if err != nil {
		return false
	}
	return len(bytes.TrimSpace(out)) > 0
}
