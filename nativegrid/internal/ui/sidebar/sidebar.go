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
	assert.IsNotNil(dispatch, "the sidebar defers its resorting to a UI loop")

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
	v.list.SetSortFunc(v.compareRows)
	v.list.ConnectRowActivated(v.activateRow)

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
//
// Every kind the model declares is named here, the ones the sidebar draws nothing
// for as loudly as the rest: a kind that falls through leaves rows standing on state
// their streams have left, and the next kind added to the model would do exactly
// that.
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
	case session.AudioReady:
		// A row shows no audio face. Audio reaches the viewer through the tile's own
		// controls, which the grid draws from this same change, so there is nothing
		// here to mirror it with.
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
	default:
		assert.Never("unexpected change kind", int(c.Kind))
	}
}

// drawApp puts the app's state on the bar under the list.
func (v *View) drawApp() {
	app, present := v.sess.App()
	v.app.draw(app, present)
}
