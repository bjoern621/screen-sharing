//go:build !windows

package display

import "testing"

// wlr-randr prints a negative coordinate for an output left of or above the layout origin, and an
// output that misses here keeps 0,0 for crop-based capture to grab the wrong rectangle.
func TestAPositionLeftOfTheOriginKeepsItsSign(t *testing.T) {
	cases := []struct {
		line string
		x, y int
	}{
		{"  Position: 2560,0", 2560, 0},
		{"  Position: -1920,0", -1920, 0},
		{"  Position: -1920,-540", -1920, -540},
	}
	for _, c := range cases {
		m := wlrPositionRe.FindStringSubmatch(c.line)
		if m == nil {
			t.Errorf("%q matched nothing", c.line)
			continue
		}
		if x, y := atoi(m[1]), atoi(m[2]); x != c.x || y != c.y {
			t.Errorf("%q parsed as %d,%d, want %d,%d", c.line, x, y, c.x, c.y)
		}
	}
}

// wlr-randr puts a mode's flags in one parenthesised group, so an output resting on its preferred
// mode reports "(preferred, current)" and an output that misses here reports no active mode at all.
func TestTheActiveModeIsTakenBesideThePreferredFlag(t *testing.T) {
	cases := []struct {
		line    string
		current bool
		w, h    int
	}{
		{"    2560x1440 px, 59.951000 Hz (preferred, current)", true, 2560, 1440},
		{"    2944x1840 px, 89.999001 Hz (current)", true, 2944, 1840},
		{"    2944x1840 px, 59.999001 Hz (preferred)", false, 0, 0},
		{"    1920x1080 px, 60.000000 Hz", false, 0, 0},
	}
	for _, c := range cases {
		m := wlrModeRe.FindStringSubmatch(c.line)
		if m == nil {
			t.Errorf("%q matched nothing", c.line)
			continue
		}
		if got := wlrModeHasFlag(m[4], "current"); got != c.current {
			t.Errorf("%q read as current=%t, want %t", c.line, got, c.current)
			continue
		}
		if !c.current {
			continue
		}
		if w, h := atoi(m[1]), atoi(m[2]); w != c.w || h != c.h {
			t.Errorf("%q parsed as %dx%d, want %dx%d", c.line, w, h, c.w, c.h)
		}
	}
}
