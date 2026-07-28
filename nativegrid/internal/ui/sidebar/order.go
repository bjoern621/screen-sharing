package sidebar

import (
	"slices"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
)

// The rank map is read and written here and nowhere else. The list holds the rows in
// the order they were appended and asks compareRows where each one belongs, so a
// reorder is a re-rank and one InvalidateSort.

// compareRows is the list's sort function, which orders two rows by their place in
// the display order.
func (v *View) compareRows(a, b *gtk.ListBoxRow) int {
	assert.IsNotNil(a, "the list sorts two of its rows")
	assert.IsNotNil(b, "the list sorts two of its rows")

	ra, ok := v.rank[a.Object.Native()]
	assert.Assert(ok, "a row in the list holds a place in the display order", a.Object.Native())
	rb, ok := v.rank[b.Object.Native()]
	assert.Assert(ok, "a row in the list holds a place in the display order", b.Object.Native())
	return ra - rb
}

// rankRow puts one row at its place in the display order. A new row is ranked
// before it enters the list, so it lands at its slot rather than at the end.
func (v *View) rankRow(i int) {
	assert.Assert(i >= 0 && i < len(v.rows), "a ranked row is one of the sidebar's", i, len(v.rows))

	pos := slices.Index(v.sess.Order(), i)
	assert.Assert(pos >= 0, "the display order covers every stream", i)
	v.rank[v.rows[i].widget.Object.Native()] = pos
}

// resortRows re-ranks every row and lets the list sort itself.
func (v *View) resortRows() {
	order := v.sess.Order()
	assert.Assert(len(order) == len(v.rows), "a place in the display order per row", len(order), len(v.rows))

	for pos, i := range order {
		assert.Assert(i >= 0 && i < len(v.rows), "the display order names streams the sidebar has rows for", i, len(v.rows))
		v.rank[v.rows[i].widget.Object.Native()] = pos
	}
	v.list.InvalidateSort()
}

// rowWidget is the hit-test target for a drag over the list: a hidden row holds no
// place in the order the pointer could aim at.
func (v *View) rowWidget(i int) gtk.Widgetter {
	assert.Assert(i >= 0 && i < len(v.rows), "a drag hit-tests against streams the sidebar has rows for", i, len(v.rows))

	if !v.sess.Visible(i) {
		return nil
	}
	return v.rows[i].widget
}
