package window

import "bjoernblessin.de/go-utils/util/assert"

// leave is Escape, and it gives back one state per press, fullscreen first.
// A window in fullscreen with a tile spotlit therefore takes two presses.
// Dropping both on one press would leave nothing to say which of them was meant.
func (c *chrome) leave() bool {
	assert.IsNotNil(c.win, "a shortcut acts on a window")

	// Fullscreen is the sidebar being gone, so leaving it is the sidebar coming
	// back, which unfullscreens the window in showSidebar (geometry.go).
	if !c.split.ShowSidebar() {
		c.showSidebar(true)
		return true
	}
	// Session.Spot is negative while the grid shows every tile.
	spot := c.sess.Spot()
	if spot < 0 {
		return false
	}
	c.sess.ToggleSpot(spot)
	return true
}

// toggleSidebar is Ctrl+B, and F11 shares it.
// It goes through showSidebar (geometry.go) like the header button does, so the
// two halves of the control cannot disagree about what the window shows.
func (c *chrome) toggleSidebar() bool {
	c.showSidebar(!c.split.ShowSidebar())
	return true
}

// spotlightNth spotlights the nth tile the grid draws, counting from one, and
// leaves the spotlight when that tile already holds it.
//
// The count runs over the watched streams in display order, which is the order
// the grid attaches its tiles in. Counting the display order itself would name
// tiles by streams that draw none: an unwatched stream keeps its slot in the
// order.
func (c *chrome) spotlightNth(n int) bool {
	assert.Assert(n > 0 && n <= spotDigits, "a number key names a tile its row reaches", n, spotDigits)

	for _, i := range c.sess.Order() {
		if !c.sess.State(i).Watched() {
			continue
		}
		n--
		if n == 0 {
			c.sess.ToggleSpot(i)
			return true
		}
	}
	return false
}
