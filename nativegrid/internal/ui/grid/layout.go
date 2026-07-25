package grid

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/tile"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/widgets"
)

// rebuild re-attaches the open tiles in display order, as the near-square grid or
// the spotlight layout. A state change within a tile does not come through here; the
// tile redraws itself in place. Every container is cleared first, so a tile never
// lands in a second parent.
func (v *View) rebuild() {
	widgets.ClearBox(v.strip)
	widgets.ClearBox(v.spotBox)
	widgets.ClearGrid(v.grid)

	shown := v.shown()
	if len(shown) == 0 {
		v.stack.SetVisibleChildName(pageEmpty)
		return
	}

	spot := v.sess.Spot()
	for _, i := range shown {
		v.tiles[i].SetSpotlit(i == spot)
	}
	if spot >= 0 {
		v.layoutSpotlight(shown, spot)
		v.stack.SetVisibleChildName(pageSpot)
		return
	}
	v.layoutGrid(shown)
	v.stack.SetVisibleChildName(pageGrid)
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
	return shown
}

// layoutGrid attaches the tiles as the near-square grid.
func (v *View) layoutGrid(shown []int) {
	cells := arrange(len(shown))
	for pos, i := range shown {
		t := v.tiles[i]
		t.SetShape(tile.ShapeFill)
		v.grid.Attach(t.Widget(), cells[pos].column, cells[pos].row, columnSpan, 1)
	}
}

// layoutSpotlight gives the page to one tile and shrinks the rest into a centered
// film strip below it, the web grid's spotlight. A spotlight with nothing beside it
// leaves the strip out rather than showing an empty band.
func (v *View) layoutSpotlight(shown []int, spot int) {
	spotlit := v.tiles[spot]
	spotlit.SetShape(tile.ShapeFill)
	v.spotBox.Append(spotlit.Widget())

	stripped := false
	for _, i := range shown {
		if i == spot {
			continue
		}
		t := v.tiles[i]
		t.SetShape(tile.ShapeStrip)
		v.strip.Append(t.Widget())
		stripped = true
	}
	if stripped {
		v.spotBox.Append(v.strip)
	}
}
