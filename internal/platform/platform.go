// Package platform reports the running OS and, on Linux, whether the session is
// Wayland or X11, so the UI can disable capture backends that cannot work here.
package platform

import (
	"os"
	"runtime"
	"strings"
)

// Info describes the running platform.
type Info struct {
	// OS is runtime.GOOS: "windows", "linux", "darwin", ...
	OS string `json:"os"`
	// Display is the Linux display server ("x11" or "wayland"), or "" when not
	// applicable (non-Linux) or undetectable.
	Display string `json:"display"`
}

// Detect reports the current OS and, on Linux, the display server. Linux
// detection prefers XDG_SESSION_TYPE, then falls back to the WAYLAND_DISPLAY /
// DISPLAY environment variables.
func Detect() Info {
	info := Info{OS: runtime.GOOS}
	if runtime.GOOS == "linux" {
		info.Display = detectLinuxDisplay()
	}
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
