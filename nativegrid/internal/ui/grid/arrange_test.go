package grid

import "testing"

// The allocation sides the cases below are measured against.
const (
	wide   = 1600
	narrow = 500
	square = 800
)

// TestArrange pins the shape an allocation drives the tiles into and the centering
// of an incomplete last row, in half-tile columns.
func TestArrange(t *testing.T) {
	cases := []struct {
		name          string
		n             int
		width, height int
		want          []cell
	}{
		{
			name: "a lone tile fills its row",
			n:    1, width: square, height: square,
			want: []cell{{column: 0, row: 0}},
		},
		{
			name: "three tiles on a wide window are one row",
			n:    3, width: wide, height: narrow,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0}, {column: 4, row: 0},
			},
		},
		{
			name: "three tiles on a tall window are one column",
			n:    3, width: narrow, height: wide,
			want: []cell{
				{column: 0, row: 0}, {column: 0, row: 1}, {column: 0, row: 2},
			},
		},
		{
			name: "four tiles on a square window are two by two",
			n:    4, width: square, height: square,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0},
				{column: 0, row: 1}, {column: 2, row: 1},
			},
		},
		{
			name: "the last row of five on a wide window starts half a tile in",
			n:    5, width: wide, height: narrow,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0}, {column: 4, row: 0},
				{column: 1, row: 1}, {column: 3, row: 1},
			},
		},
		{
			name: "the last row of five on a square window starts half a tile in",
			n:    5, width: square, height: square,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0},
				{column: 0, row: 1}, {column: 2, row: 1},
				{column: 1, row: 2},
			},
		},
		{
			name: "six tiles on a wide window are two rows of three",
			n:    6, width: wide, height: narrow,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0}, {column: 4, row: 0},
				{column: 0, row: 1}, {column: 2, row: 1}, {column: 4, row: 1},
			},
		},
		{
			name: "an allocation with no room falls back to the near-square shape",
			n:    3, width: 0, height: 0,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0},
				{column: 1, row: 1},
			},
		},
	}
	for _, c := range cases {
		got := arrange(c.n, c.width, c.height, gap)
		if len(got) != len(c.want) {
			t.Errorf("%s: placed %d tiles, want %d", c.name, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: tile %d at %+v, want %+v", c.name, i, got[i], c.want[i])
			}
		}
	}
}

// TestArrangeRowsStayWithinTheGrid holds the invariant the attach depends on: no
// tile reaches past the row width its column count allocates, whatever shape the
// allocation drives it into.
func TestArrangeRowsStayWithinTheGrid(t *testing.T) {
	areas := []struct{ width, height int }{
		{wide, narrow}, {narrow, wide}, {square, square}, {320, 240}, {0, 0},
	}
	for _, a := range areas {
		for n := 1; n <= 32; n++ {
			cells := arrange(n, a.width, a.height, gap)
			if len(cells) != n {
				t.Errorf("arrange(%d) in %dx%d placed %d tiles", n, a.width, a.height, len(cells))
				continue
			}
			width := 0
			for _, c := range cells {
				if end := c.column + columnSpan; end > width {
					width = end
				}
			}
			perRow := map[int]int{}
			for _, c := range cells {
				perRow[c.row]++
			}
			for row, count := range perRow {
				if count*columnSpan > width {
					t.Errorf("arrange(%d) in %dx%d: row %d holds %d tiles in a grid %d columns wide",
						n, a.width, a.height, row, count, width)
				}
			}
		}
	}
}
