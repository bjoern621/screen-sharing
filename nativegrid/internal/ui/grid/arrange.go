package grid

import (
	"math"

	"bjoernblessin.de/go-utils/util/assert"
)

// columnSpan is the width of a tile in grid columns. Tiles span two homogeneous
// columns, so an incomplete last row can be centered on a whole one-column offset,
// which is half a tile.
const columnSpan = 2

// cell is where one tile sits in the grid, in half-tile columns.
type cell struct {
	column, row int
}

// arrange places n tiles row-major in a near-square grid and centers an incomplete
// last row. The result holds one cell per tile, in the order the tiles are shown.
func arrange(n int) []cell {
	assert.Assert(n > 0, "a grid is laid out for at least one tile", n)

	columns := int(math.Ceil(math.Sqrt(float64(n))))
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
