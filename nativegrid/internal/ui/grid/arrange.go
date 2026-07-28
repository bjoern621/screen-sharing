package grid

import (
	"math"

	"bjoernblessin.de/go-utils/util/assert"
)

// columnSpan is the width of a tile in grid columns. Tiles span two homogeneous
// columns, so an incomplete last row can be centered on a whole one-column offset,
// which is half a tile.
const columnSpan = 2

// tileAspect is the shape a tile is ranked as. Sources publish 16:9 and the picture
// letterboxes inside its cell, so the space a cell wastes on the other axis is not
// tile.
const tileAspect = 16.0 / 9.0

// cell is where one tile sits in the grid, in half-tile columns.
type cell struct {
	column, row int
}

// arrange places n tiles row-major in the column count that gives a 16:9 tile the
// most area inside a width by height allocation, and centers an incomplete last row.
// The result holds one cell per tile, in the order the tiles are shown.
//
// width and height are the space the cells and the gaps between them share, so the
// caller subtracts whatever margin it keeps around the grid.
//
// The tile count alone cannot answer this: three tiles fit one row on a wide window
// and one column on a tall one, and a near-square shape wastes most of both.
//
// A near-square grid is the starting point and a column count has to beat it
// strictly. That settles the case an allocation cannot: an area with no room to
// measure, before the window has allocated the tile area, ranks every column count
// the same.
func arrange(n, width, height, gap int) []cell {
	assert.Assert(n > 0, "a grid is laid out for at least one tile", n)

	columns := int(math.Ceil(math.Sqrt(float64(n))))
	best := tileWidth(n, columns, width, height, gap)
	for c := 1; c <= n; c++ {
		if w := tileWidth(n, c, width, height, gap); w > best {
			columns, best = c, w
		}
	}
	rows := (n + columns - 1) / columns

	cells := make([]cell, 0, n)
	for pos := range n {
		row := pos / columns
		inRow := columns
		if row == rows-1 {
			inRow = n - row*columns
		}
		// The offset is what the row is short of a full one, in half-tile columns, so
		// a row of two under a row of three starts half a tile in.
		cells = append(cells, cell{
			column: (columns - inRow) + columnSpan*(pos%columns),
			row:    row,
		})
	}
	return cells
}

// tileWidth is the width of the largest 16:9 rectangle that fits one cell of a grid
// columns wide holding n tiles. Tile area grows with that width, so it ranks the
// column counts without the squaring. A cell the allocation leaves no room for
// scores zero.
func tileWidth(n, columns, width, height, gap int) float64 {
	rows := (n + columns - 1) / columns
	cellWidth := float64(width-(columns-1)*gap) / float64(columns)
	cellHeight := float64(height-(rows-1)*gap) / float64(rows)
	if cellWidth <= 0 || cellHeight <= 0 {
		return 0
	}
	return math.Min(cellWidth, cellHeight*tileAspect)
}
