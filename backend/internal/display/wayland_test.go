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
