// Package sidebar lists the streams the relay offers, one row per stream, with a
// check that watches it.
//
// The rows follow the grid's display order, so dragging a tile re-sorts them, and a
// row is a drag handle for that same order, which is how a stream nobody watches yet
// can be moved into place.
//
// Under the rows sits the app bar (appbar.go), which acts on the app rather than on
// a stream: its settings to the front, and this machine's own publish on or off.
package sidebar

import (
	"fmt"
	"slices"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/dnd"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// View is the stream sidebar.
type View struct {
	root  gtk.Widgetter
	title *adw.WindowTitle
	list  *gtk.ListBox
	app   *appBar
	sess  *session.Session
	drag  *dnd.Controller
	// rows are the rows by stream index, which is not their position in the list:
	// the list follows the display order.
	rows []*row
	// rank is each row's place in the display order, keyed by the row's object
	// address. The list sorts on it, so a reorder is one InvalidateSort and no row
	// ever leaves the list: removing a row cancels the drag it is the source of,
	// which is the drag doing the reordering.
	rank map[uintptr]int
	// resort coalesces the reordering bursts a drag produces.
	resort *idle.Coalescer
	// icons holds the row icons, which stay for the process lifetime: a row is
	// hidden when its stream goes away, never removed.
	icons theme.Icons
}

func New(sess *session.Session, drag *dnd.Controller, dispatch idle.Dispatch) *View {
	assert.IsNotNil(sess, "the sidebar lists a session")
	assert.IsNotNil(drag, "the sidebar shares the drag controller")

	v := &View{
		title: adw.NewWindowTitle("Streams", ""),
		list:  gtk.NewListBox(),
		sess:  sess,
		drag:  drag,
		rank:  map[uintptr]int{},
	}
	v.resort = idle.New(dispatch, v.resortRows)

	v.list.AddCSSClass("navigation-sidebar")
	v.list.SetSelectionMode(gtk.SelectionNone)
	v.list.SetSortFunc(func(a, b *gtk.ListBoxRow) int {
		return v.rank[a.Object.Native()] - v.rank[b.Object.Native()]
	})
	v.list.ConnectRowActivated(func(activated *gtk.ListBoxRow) {
		// The row is matched by identity, not by position: the list is sorted by the
		// display order, which the stream indexes behind the rows do not follow.
		for _, r := range v.rows {
			if r.widget.Eq(activated) {
				r.toggle()
				return
			}
		}
	})

	placeholder := gtk.NewLabel("No streams on the relay")
	placeholder.AddCSSClass("sidebar-empty")
	v.list.SetPlaceholder(placeholder)

	for i := range sess.Len() {
		v.addRow(i)
	}
	v.drag.AttachTarget(v.list, v.rowWidget, v.resort.Pending)
	v.app = newAppBar(&v.icons, sess.RunAppCommand)
	v.root = v.build()
	v.drawApp()
	return v
}

// Widget is the sidebar, the split view's sidebar child.
func (v *View) Widget() gtk.Widgetter { return v.root }

// Changed redraws for one model change.
func (v *View) Changed(c session.Change) {
	switch c.Kind {
	case session.StreamAdded:
		v.addRow(c.Index)
	case session.StateChanged:
		v.drawRow(c.Index)
		v.drawTitle()
	case session.StallChanged:
		// The watch state is untouched, so the heading's count holds and only the
		// row's status face moves.
		v.drawRow(c.Index)
	case session.RosterChanged:
		// Presence moves rows in and out of sight without touching a watch state, and
		// it can change any of them at once.
		assert.Assert(len(v.rows) == v.sess.Len(), "a row per stream", len(v.rows), v.sess.Len())
		for i := range v.rows {
			v.drawRow(i)
		}
		v.drawTitle()
	case session.OrderChanged:
		v.resort.Schedule()
	case session.AppChanged:
		v.drawApp()
	}
}

// build wraps the list in the scroller, the header the window shows it in, and
// the app bar under both.
func (v *View) build() gtk.Widgetter {
	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(v.list)
	scroll.SetVExpand(true)

	header := adw.NewHeaderBar()
	header.SetTitleWidget(v.title)

	view := adw.NewToolbarView()
	view.AddTopBar(header)
	view.SetContent(scroll)
	view.AddBottomBar(v.app.Widget())
	return view
}

// drawApp puts the app's state on the bar under the list.
func (v *View) drawApp() {
	app, present := v.sess.App()
	v.app.draw(app, present)
}

// addRow appends the row of a stream that joined, at its slot in the display order:
// the slot of a row added at launch comes from the remembered order, not from the
// order the streams were configured in.
func (v *View) addRow(i int) {
	r := newRow(v.sess.Stream(i).Name, &v.icons,
		func(on bool) { v.sess.SetWatched(i, on) },
		func(transport string, options map[string]string) { v.sess.RequestWatchLeg(i, transport, options) })
	v.rows = append(v.rows, r)
	assert.Assert(len(v.rows) == i+1, "a row per stream, in stream order", len(v.rows), i)

	v.drag.AttachSource(r.widget, i)
	// Ranked before it enters the list, so it lands at its slot right away.
	v.rank[r.widget.Object.Native()] = slices.Index(v.sess.Order(), i)
	v.list.Append(r.widget)
	v.drawRow(i)
}

// drawRow puts stream i's state on its row.
func (v *View) drawRow(i int) {
	v.rows[i].draw(v.sess.Stream(i), v.sess.State(i), v.sess.Stalled(i), v.sess.Visible(i))
}

// drawTitle states the open-tile count beside the heading, the way the web roster's
// badge does. An empty subtitle collapses, so a grid with nothing open shows the
// heading alone, like the web shows no badge.
func (v *View) drawTitle() {
	n := v.sess.Watching()
	if n == 0 {
		v.title.SetSubtitle("")
		return
	}
	v.title.SetSubtitle(fmt.Sprintf("%d watching", n))
}

// resortRows re-ranks every row and lets the list sort itself.
func (v *View) resortRows() {
	for pos, i := range v.sess.Order() {
		v.rank[v.rows[i].widget.Object.Native()] = pos
	}
	v.list.InvalidateSort()
}

// rowWidget is the hit-test target for a drag over the list: a hidden row holds no
// place in the order the pointer could aim at.
func (v *View) rowWidget(i int) gtk.Widgetter {
	if !v.sess.Visible(i) {
		return nil
	}
	return v.rows[i].widget
}
