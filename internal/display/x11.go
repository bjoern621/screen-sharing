//go:build !windows

package display

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// x11ConnectedRe matches an xrandr output header for a connected monitor with an active mode:
// the output name, an optional "primary" flag, and the WxH+X+Y geometry.
// A connected output with no active mode, one that is turned off, omits the geometry and does not
// match, so it is left out of the listing.
var x11ConnectedRe = regexp.MustCompile(`^(\S+) connected (primary )?(\d+)x(\d+)\+(\d+)\+(\d+)`)

// x11CurrentModeRe pulls the refresh rate off an indented mode line.
// xrandr flags the active mode with '*', as in "1920x1080 143.98*+".
var x11CurrentModeRe = regexp.MustCompile(`([\d.]+)\s*\*`)

// listX11 enumerates monitors from "xrandr --query".
// Each connected output with an active mode becomes one entry, indexed in xrandr's listing order
// and carrying its virtual-desktop offset for crop-based x11grab capture.
//
// It answers nil where xrandr is absent or reports no active output, so the caller falls back to
// the next provider or the placeholder.
// Both are Umgebungsfehler, which is why neither is asserted on.
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
		// Mode lines are indented under their output header; the flagged current mode sets that output's
		// refresh rate.
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

// roundHz parses a decimal refresh rate and rounds it to the nearest integer.
// Unparseable input yields zero, the "unknown refresh" sentinel, since the text came out of another
// program and is not this app's to guarantee.
func roundHz(s string) int {
	hz, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(hz + 0.5)
}
