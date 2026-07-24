//go:build !windows

package display

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// x11ConnectedRe matches an xrandr output header for a connected monitor with an
// active mode: the output name, an optional "primary" flag, and the WxH+X+Y
// geometry. A connected output with no active mode (turned off) omits the
// geometry and does not match, so it is left out of the listing.
var x11ConnectedRe = regexp.MustCompile(`^(\S+) connected (primary )?(\d+)x(\d+)\+(\d+)\+(\d+)`)

// x11CurrentModeRe pulls the refresh rate off an indented mode line. xrandr flags
// the active mode with '*', e.g. "1920x1080  143.98*+".
var x11CurrentModeRe = regexp.MustCompile(`([\d.]+)\s*\*`)

// listX11 enumerates monitors from `xrandr --query`. Each connected output with
// an active mode becomes one entry, indexed in xrandr's listing order and
// carrying its virtual-desktop offset for crop-based x11grab capture. Returns nil
// when xrandr is absent or reports no active output, so the caller falls back to
// the next provider or the placeholder.
func listX11() []Monitor {
	out, err := exec.Command("xrandr", "--query").Output()
	if err != nil {
		return nil
	}

	var monitors []Monitor
	for _, line := range strings.Split(string(out), "\n") {
		if m := x11ConnectedRe.FindStringSubmatch(line); m != nil {
			monitors = append(monitors, Monitor{
				Index:   len(monitors),
				Width:   atoi(m[3]),
				Height:  atoi(m[4]),
				OffsetX: atoi(m[5]),
				OffsetY: atoi(m[6]),
				Primary: m[2] != "",
			})
			continue
		}
		// Mode lines are indented under their output header; the flagged current
		// mode sets that output's refresh rate.
		if len(monitors) > 0 && indented(line) {
			if r := x11CurrentModeRe.FindStringSubmatch(line); r != nil {
				monitors[len(monitors)-1].RefreshHz = roundHz(r[1])
			}
		}
	}
	return monitors
}

func indented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// roundHz parses a decimal refresh rate and rounds it to the nearest integer.
// Unparseable input yields 0, the "unknown refresh" sentinel.
func roundHz(s string) int {
	hz, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(hz + 0.5)
}
