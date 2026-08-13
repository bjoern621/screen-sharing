//go:build !windows

package display

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// listWayland enumerates monitors under a Wayland session.
// The protocols the query tools speak are per-family extensions rather than one interface spanning
// the compositors, so each family needs a probe of its own and the first that reports a monitor
// wins.
//
// A compositor no probe covers, GNOME and KDE among them, answers nil and leaves the caller its X11
// (XWayland) fallback or the placeholder.
func listWayland() []Monitor {
	for _, enumerate := range []func() []Monitor{listHyprland, listWlrRandr} {
		if monitors := enumerate(); len(monitors) > 0 {
			return monitors
		}
	}
	return nil
}

// listHyprland reads "hyprctl monitors -j", Hyprland's own JSON monitor list.
// hyprctl missing and output that does not parse are Umgebungsfehler and both answer nil.
func listHyprland() []Monitor {
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return nil
	}

	var raw []struct {
		Width       int     `json:"width"`
		Height      int     `json:"height"`
		X           int     `json:"x"`
		Y           int     `json:"y"`
		RefreshRate float64 `json:"refreshRate"`
		Focused     bool    `json:"focused"`
	}
	if json.Unmarshal(out, &raw) != nil {
		return nil
	}

	monitors := make([]Monitor, len(raw))
	for i, r := range raw {
		monitors[i] = Monitor{
			Index: i,
			// Hyprland's width and height are the mode's pixel count, not the fractionally scaled logical
			// size, which is the number crop-based capture needs.
			Width:   r.Width,
			Height:  r.Height,
			OffsetX: r.X,
			OffsetY: r.Y,
			// Hyprland names no primary output, so the focused one stands in and one entry carries the flag.
			Primary:   r.Focused,
			RefreshHz: int(r.RefreshRate + 0.5),
		}
	}

	assert.Assert(len(monitors) == len(raw), "every reported output becomes an entry", len(monitors), len(raw))
	return monitors
}

// wlrModeRe matches a wlr-randr mode line: "1920x1080 px, 143.981003 Hz (current)".
var wlrModeRe = regexp.MustCompile(`(\d+)x(\d+) px,\s*([\d.]+) Hz`)

// wlrPositionRe matches a wlr-randr position line: "Position: -1920,0".
// An output left of or above the layout origin sits at a negative coordinate, and a pattern
// refusing the sign leaves that output at 0,0 for crop-based capture to grab the wrong rectangle.
var wlrPositionRe = regexp.MustCompile(`Position:\s*(-?\d+),(-?\d+)`)

// listWlrRandr parses "wlr-randr", which covers the wlroots compositors, Sway among them.
// An output header starts at column 0 and that output's mode, refresh rate and position follow on
// indented lines.
// Only an output with an active mode is kept, and the survivors are renumbered so Index runs
// contiguously from zero.
//
// The listing is nil where wlr-randr is not installed and where the compositor implements none of
// the output-management protocol it queries, both of them the same absence to a caller.
func listWlrRandr() []Monitor {
	out, err := exec.Command("wlr-randr").Output()
	if err != nil {
		return nil
	}

	var monitors []Monitor
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		if !indented(line) {
			monitors = append(monitors, Monitor{})
			continue
		}
		if len(monitors) == 0 {
			continue
		}
		cur := &monitors[len(monitors)-1]
		if strings.Contains(line, "(current)") {
			if m := wlrModeRe.FindStringSubmatch(line); m != nil {
				cur.Width = atoi(m[1])
				cur.Height = atoi(m[2])
				cur.RefreshHz = roundHz(m[3])
			}
			continue
		}
		if p := wlrPositionRe.FindStringSubmatch(line); p != nil {
			cur.OffsetX = atoi(p[1])
			cur.OffsetY = atoi(p[2])
		}
	}

	// A zero width is an output that reported no active mode: disabled, or a block that did not parse.
	kept := monitors[:0]
	for _, m := range monitors {
		if m.Width > 0 {
			m.Index = len(kept)
			kept = append(kept, m)
		}
	}

	for i, m := range kept {
		assert.Assert(m.Index == i, "a kept output is renumbered to its list position", m.Index, i)
		assert.Assert(m.Width > 0, "a kept output has an active mode", m.Width, m.Height)
	}
	return kept
}
