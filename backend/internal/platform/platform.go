// Package platform reports the running operating system and, on Linux, whether the session is
// Wayland or X11, which is what the capture and audio tables gate on.
package platform

import (
	"os"
	"runtime"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

type Info struct {
	// OS is runtime.GOOS: "windows", "linux", "darwin".
	OS string `json:"os"`
	// Display is the Linux display server, "x11" or "wayland".
	// Empty off Linux and on a session where neither is detected.
	Display string `json:"display"`
}

// Detect reports the operating system and, on Linux, the display server.
// Detection prefers XDG_SESSION_TYPE and falls back to WAYLAND_DISPLAY, then DISPLAY.
//
// A session naming none of the three answers empty rather than failing: a headless login has no
// display server, and the capture table already refuses the backends that need one.
func Detect() Info {
	info := Info{OS: runtime.GOOS}
	if runtime.GOOS == "linux" {
		info.Display = detectLinuxDisplay()
	}

	assert.Assert(info.OS != "", "a detected platform names the operating system it runs on")
	assert.Assert(info.Display == "" || info.Display == "x11" || info.Display == "wayland",
		"a detected display server is one this app knows", info.Display)
	return info
}

func detectLinuxDisplay() string {
	switch strings.ToLower(os.Getenv("XDG_SESSION_TYPE")) {
	case "wayland":
		return "wayland"
	case "x11":
		return "x11"
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}
	return ""
}
