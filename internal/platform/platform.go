// Package platform reports the running OS and, on Linux, whether the session is Wayland or X11,
// so the UI can disable capture backends that cannot work here.
package platform

import (
	"os"
	"runtime"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// Info describes the running platform.
type Info struct {
	// OS is runtime.GOOS: "windows", "linux", "darwin", and so on.
	OS string `json:"os"`
	// Display is the Linux display server ("x11" or "wayland"), and empty where it does not apply (a
	// non-Linux machine) or cannot be detected.
	Display string `json:"display"`
}

// Detect reports the current OS and, on Linux, the display server.
// Linux detection prefers XDG_SESSION_TYPE, then falls back to the WAYLAND_DISPLAY and DISPLAY
// environment variables.
//
// A session that names none of the three is an Umgebungsfehler and answers empty,
// since a headless login has no display server and the capture table already refuses the backends
// that need one.
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
