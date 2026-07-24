//go:build !windows

package display

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
)

// listWayland enumerates monitors under a Wayland session. Wayland has no
// standard enumeration CLI, so each compositor family needs its own probe; the
// probes run in order and the first that reports a monitor wins. Returns nil on
// a compositor none of them cover (GNOME, KDE), leaving the caller to fall back
// to the X11 (XWayland) enumerator or the placeholder.
func listWayland() []Monitor {
	for _, enumerate := range []func() []Monitor{listHyprland, listWlrRandr} {
		if monitors := enumerate(); len(monitors) > 0 {
			return monitors
		}
	}
	return nil
}

// listHyprland reads `hyprctl monitors -j`, the JSON monitor list Hyprland
// exposes. Returns nil when hyprctl is absent or the output does not parse.
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
			Index:  i,
			Width:  r.Width,
			Height: r.Height,
			// Hyprland reports the mode's pixel size independent of the fractional
			// scale, which is what crop-based capture needs.
			OffsetX: r.X,
			OffsetY: r.Y,
			// Hyprland has no primary-output concept; the focused monitor stands
			// in so one entry carries the flag.
			Primary:   r.Focused,
			RefreshHz: int(r.RefreshRate + 0.5),
		}
	}
	return monitors
}

// wlrModeRe matches a wlr-randr mode line for the active mode, e.g.
// "1920x1080 px, 143.981003 Hz (current)".
var wlrModeRe = regexp.MustCompile(`(\d+)x(\d+) px,\s*([\d.]+) Hz`)

// wlrPositionRe matches a wlr-randr "Position: X,Y" line.
var wlrPositionRe = regexp.MustCompile(`Position:\s*(\d+),(\d+)`)

// listWlrRandr parses `wlr-randr`, covering wlroots compositors (Sway and
// others). Output headers start at column 0; each output's mode, refresh rate
// and position follow on indented lines. Only outputs with an active mode are
// kept, then re-indexed so the indices are contiguous. Returns nil when wlr-randr
// is absent.
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

	// Drop outputs with no active mode (disabled, or an unparsed block) and
	// renumber the survivors so Index matches list position.
	kept := monitors[:0]
	for _, m := range monitors {
		if m.Width > 0 {
			m.Index = len(kept)
			kept = append(kept, m)
		}
	}
	return kept
}
