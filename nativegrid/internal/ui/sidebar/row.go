package sidebar

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// addRow appends the row of a stream that joined, at its slot in the display order:
// the slot of a row added at launch comes from the remembered order, not from the
// order the streams were configured in.
func (v *View) addRow(i int) {
	assert.Assert(i == len(v.rows), "a row is added for the stream that just joined", i, len(v.rows))
	assert.Assert(i < v.sess.Len(), "an added row has a stream behind it", i, v.sess.Len())

	r := newRow(v.sess.Stream(i).Name, &v.icons,
		func(on bool) { v.sess.SetWatched(i, on) },
		func(transport string, options map[string]string) { v.sess.RequestWatchLeg(i, transport, options) })
	v.rows = append(v.rows, r)

	v.drag.AttachSource(r.widget, i)
	v.rankRow(i)
	v.list.Append(r.widget)
	v.drawRow(i)
}

// drawRow puts stream i's state on its row.
func (v *View) drawRow(i int) {
	assert.Assert(i >= 0 && i < len(v.rows), "a row per stream", i, len(v.rows))

	v.rows[i].draw(v.sess.Stream(i), v.sess.State(i), v.sess.Stalled(i), v.sess.Visible(i))
}

// activateRow watches or unwatches the row the list activated. The row is matched by
// identity, not by position: the list is sorted by the display order, which the
// stream indexes behind the rows do not follow.
func (v *View) activateRow(activated *gtk.ListBoxRow) {
	assert.IsNotNil(activated, "the list activates one of its rows")

	for _, r := range v.rows {
		if r.widget.Eq(activated) {
			r.toggle()
			return
		}
	}
	assert.Never("an activated row is one the sidebar built", activated.Object.Native())
}

// row is one stream's sidebar row: the check that watches the stream, its name, the
// watch-leg control, and the status that mirrors the tile.
type row struct {
	widget *gtk.ListBoxRow
	check  *gtk.CheckButton
	leg    *legView
	status *gtk.Stack
	// syncing suppresses the check's own signal while the view writes the model's
	// state into it, so a redraw cannot loop back into the model it is drawing.
	syncing bool
}

// newRow builds a row. watch is called when the check changes by hand, never by the
// view writing the model's state back into it; ask carries a watch-leg change to
// the app.
func newRow(name string, icons *theme.Icons, watch func(on bool), ask func(transport string, options map[string]string)) *row {
	assert.IsNotNil(icons, "a row's icons belong to a set", name)
	assert.IsNotNil(watch, "a row watches through a callback", name)
	assert.IsNotNil(ask, "a row's watch-leg change goes somewhere", name)

	r := &row{
		widget: gtk.NewListBoxRow(),
		check:  gtk.NewCheckButton(),
		leg:    newLegView(icons, ask),
		status: newStatus(icons),
	}
	r.check.ConnectToggled(func() {
		if r.syncing {
			return
		}
		watch(r.check.Active())
	})

	label := gtk.NewLabel(name)
	label.SetXAlign(0)
	label.SetHExpand(true)

	box := gtk.NewBox(gtk.OrientationHorizontal, 8)
	box.Append(r.check)
	box.Append(label)
	box.Append(r.leg.Widget())
	box.Append(r.status)
	r.widget.SetChild(box)
	return r
}

// draw puts one watch state on the row: its status face, the pressed-pill tint of a
// watched row, the check, and the watch leg the stream arrives over. stalled is the
// model's frame report, which the face folds into the live state.
//
// Every property is set on every pass, the tint of an unwatched row as much as the
// tint of a watched one, so a second draw on unchanged state is a redraw of the same
// row rather than a class nobody clears.
func (r *row) draw(st roster.Stream, state session.State, stalled, visible bool) {
	r.leg.draw(st)

	face := faceFor(state, stalled)
	r.status.SetVisibleChildName(face.page)
	r.status.SetTooltipText(face.tip)
	r.widget.SetVisible(visible)
	if state.Watched() {
		r.widget.AddCSSClass("watched")
	} else {
		r.widget.RemoveCSSClass("watched")
	}

	// The tile's disconnect button unwatches a stream without the check knowing, so
	// the check follows the model rather than the click that set it.
	r.syncing = true
	r.check.SetActive(state.Watched())
	r.syncing = false
}

// toggle flips the check by hand, which is what activating the row does.
func (r *row) toggle() {
	r.check.SetActive(!r.check.Active())
}
