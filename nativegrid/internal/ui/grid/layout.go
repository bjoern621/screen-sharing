package grid

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/tile"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/widgets"
)

// rebuild re-attaches the open tiles in display order, as the grid or the spotlight
// layout. A state change within a tile does not come through here; the tile redraws
// itself in place.
//
// Every container is cleared before anything is attached, which is what makes a
// second rebuild on unchanged model state produce the same tree: a tile lands in one
// parent, in one place, however often the model reports the same order.
func (v *View) rebuild() {
	widgets.ClearBox(v.strip)
	widgets.ClearBox(v.spotBox)
	widgets.ClearGrid(v.grid)
	assert.Assert(v.grid.FirstChild() == nil && v.spotBox.FirstChild() == nil && v.strip.FirstChild() == nil,
		"a rebuild attaches into empty containers")

	shown := v.shown()
	if len(shown) == 0 {
		v.showPage(pageEmpty)
		return
	}

	spot := v.sess.Spot()
	for _, i := range shown {
		v.tileAt(i).SetSpotlit(i == spot)
	}
	if spot >= 0 {
		v.layoutSpotlight(shown, spot)
		v.showPage(pageSpot)
		return
	}
	// The page goes up before the arrangement is stored, because an arrangement off the
	// grid page is the state showPage exists to prevent: filling v.cells first would leave
	// that false for as long as the stack still names the page being left.
	v.showPage(pageGrid)
	v.layoutGrid(shown)
}

// showPage puts one of the stack's pages up, and drops the arrangement with the page
// that was laid out in it.
// Only the grid measures itself against the allocation, so off it there is no
// arrangement rather than one nobody may read.
func (v *View) showPage(name string) {
	assert.Assert(name == pageEmpty || name == pageGrid || name == pageSpot, "a page belongs to the stack", name)

	if name != pageGrid {
		v.cells = nil
	}
	v.stack.SetVisibleChildName(name)
}

// resized re-runs the arrangement for the tile area's new allocation, and only when
// the new one lands on a different shape: a window drag reports every intermediate
// size, and re-attaching every tile for a size the grid arranges the same way would
// cost a reparent per frame.
//
// The relayout goes through the coalescer rather than running here. An allocation is
// no place to reparent widgets, and the drop target reads Pending to know the bounds
// it hit-tests against are the ones a due relayout is about to move.
func (v *View) resized() {
	// The empty page and the spotlight hold their shape at any size; only the grid
	// reads the allocation.
	if v.stack.VisibleChildName() != pageGrid {
		assert.Assert(len(v.cells) == 0, "an arrangement belongs to the grid page", len(v.cells))
		return
	}
	if slices.Equal(v.cells, v.arrangement(len(v.shown()))) {
		return
	}
	v.relayout.Schedule()
}

// arrangement is where n tiles sit in the tile area's allocation, less the margin the
// grid keeps around itself.
func (v *View) arrangement(n int) []cell {
	return arrange(n, v.probe.Width()-2*gap, v.probe.Height()-2*gap, gap)
}

// shown is the streams with an open tile, in display order.
func (v *View) shown() []int {
	order := v.sess.Order()
	shown := make([]int, 0, len(order))
	for _, i := range order {
		if !v.sess.State(i).Watched() {
			continue
		}
		_, open := v.tiles[i]
		assert.Assert(open, "a watched stream holds a tile", i)
		shown = append(shown, i)
	}
	assert.Assert(len(shown) == v.sess.Watching(), "a tile per watched stream", len(shown), v.sess.Watching())

	return shown
}

// layoutGrid attaches the tiles in the shape the tile area's allocation gives the
// largest tile. The arrangement is kept, so a resize can tell whether the new
// allocation is worth a relayout.
func (v *View) layoutGrid(shown []int) {
	assert.Assert(len(shown) > 0, "a grid is laid out for at least one tile")

	v.cells = v.arrangement(len(shown))
	assert.Assert(len(v.cells) == len(shown), "a cell per shown tile", len(v.cells), len(shown))

	for pos, i := range shown {
		t := v.tileAt(i)
		t.SetShape(tile.ShapeFill)
		v.grid.Attach(t.Widget(), v.cells[pos].column, v.cells[pos].row, columnSpan, 1)
	}
}

// layoutSpotlight gives the page to one tile and shrinks the rest into a centered
// film strip below it, the web grid's spotlight. A spotlight with nothing beside it
// leaves the strip out rather than showing an empty band.
func (v *View) layoutSpotlight(shown []int, spot int) {
	assert.Assert(slices.Contains(shown, spot), "a spotlit stream is one of the shown ones", spot)

	spotlit := v.tileAt(spot)
	spotlit.SetShape(tile.ShapeFill)
	v.spotBox.Append(spotlit.Widget())

	stripped := false
	for _, i := range shown {
		if i == spot {
			continue
		}
		t := v.tileAt(i)
		t.SetShape(tile.ShapeStrip)
		v.strip.Append(t.Widget())
		stripped = true
	}
	if stripped {
		v.spotBox.Append(v.strip)
	}
}
