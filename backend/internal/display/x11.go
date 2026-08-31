//go:build !windows

package display

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// x11ConnectedRe matches an xrandr output header for a connected output with an active mode:
// "HDMI-A-1 connected primary 1920x1080+2560+0 (normal left inverted right x axis y axis)".
// A connected output that is turned off carries no geometry,
// so it does not match and stays out of the listing.
var x11ConnectedRe = regexp.MustCompile(`^(\S+) connected (primary )?(\d+)x(\d+)\+(\d+)\+(\d+)`)

// x11CurrentModeRe pulls the refresh rate off an indented mode line.
// xrandr flags the active mode with '*': "1920x1080 143.98*+".
var x11CurrentModeRe = regexp.MustCompile(`([\d.]+)\s*\*`)

// listX11 enumerates monitors from "xrandr --query", the RandR view of the X screen.
// A connected output with an active mode becomes one entry,
// indexed in xrandr's listing order and carrying its offset within the X screen,
// the origin crop-based capture starts its rectangle at.
//
// xrandr missing and xrandr reporting no active output are Umgebungsfehler, so neither asserts:
// both answer nil and the caller falls through to the next provider or to the placeholder.
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
		// A mode line is indented under its output header,
		// and the flagged one carries that output's refresh rate.
		if len(monitors) > 0 && indented(line) {
			if r := x11CurrentModeRe.FindStringSubmatch(line); r != nil {
				monitors[len(monitors)-1].RefreshHz = roundHz(r[1])
			}
		}
	}

	for i, m := range monitors {
		assert.Assert(m.Index == i, "an enumerated output is indexed in listing order", m.Index, i)
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

// roundHz rounds a decimal refresh rate to the nearest whole Hz.
// Text that does not parse yields zero, the unknown-refresh sentinel:
// it came out of another program and is not this app's to guarantee.
func roundHz(s string) int {
	hz, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(hz + 0.5)
}
