package grid

import "testing"

// TestArrange pins the near-square shape and the centering of an incomplete last
// row, in half-tile columns: a lone tile fills its row, four tiles make a square,
// and the last row of five starts half a tile in.
func TestArrange(t *testing.T) {
	cases := []struct {
		n    int
		want []cell
	}{
		{n: 1, want: []cell{{column: 0, row: 0}}},
		{
			n:    2,
			want: []cell{{column: 0, row: 0}, {column: 2, row: 0}},
		},
		{
			n: 3,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0},
				{column: 1, row: 1},
			},
		},
		{
			n: 4,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0},
				{column: 0, row: 1}, {column: 2, row: 1},
			},
		},
		{
			n: 5,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0}, {column: 4, row: 0},
				{column: 1, row: 1}, {column: 3, row: 1},
			},
		},
		{
			n: 6,
			want: []cell{
				{column: 0, row: 0}, {column: 2, row: 0}, {column: 4, row: 0},
				{column: 0, row: 1}, {column: 2, row: 1}, {column: 4, row: 1},
			},
		},
	}
	for _, c := range cases {
		got := arrange(c.n)
		if len(got) != len(c.want) {
			t.Errorf("arrange(%d) placed %d tiles, want %d", c.n, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("arrange(%d)[%d] = %+v, want %+v", c.n, i, got[i], c.want[i])
			}
		}
	}
}

// TestArrangeRowsStayWithinTheGrid holds the invariant the attach depends on: no
// tile reaches past the row width the near-square shape allocates.
func TestArrangeRowsStayWithinTheGrid(t *testing.T) {
	for n := 1; n <= 32; n++ {
		cells := arrange(n)
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
				t.Errorf("arrange(%d) row %d holds %d tiles in a grid %d columns wide", n, row, count, width)
			}
		}
	}
}
