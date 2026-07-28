// Package grid is the tile area: an empty-state page until a stream is watched,
// then a grid of tiles shaped to the space it has, or the spotlight layout when one
// stream is spotlit.
//
// The view owns the tiles and nothing else. What is watched, in what order, and
// which stream is spotlit is the model's; the view redraws from what it reads back
// when the model reports a change (observe.go), through the tile lifecycle in
// tiles.go and the arrangement in layout.go.
package grid

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/dnd"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/tile"
)

// The stack's pages, one per layout the tile area takes.
const (
	pageEmpty = "empty"
	pageGrid  = "grid"
	pageSpot  = "spot"
)

// gap is the space between tiles and around the tile area, the web grid's gap-3.
const gap = 12

// View is the tile area.
type View struct {
	root    *gtk.Overlay // the stack under the size probe
	stack   *gtk.Stack
	grid    *gtk.Grid
	spotBox *gtk.Box // the spotlight page: the spotlit tile over the strip
	strip   *gtk.Box
	sess    *session.Session
	drag    *dnd.Controller
	// probe reports the tile area's allocation, which the grid's shape is chosen
	// from. GTK4 emits a widget's own resize on GtkDrawingArea alone, so an
	// input-transparent one is laid over the stack, where it is allocated whichever
	// page shows rather than only while the grid is up.
	probe *gtk.DrawingArea
	// tiles are the open tiles by stream index, so an index that was never watched
	// costs nothing.
	tiles map[int]*tile.Tile
	// cells is the arrangement the attached tiles sit in, which a resize is measured
	// against: an allocation that lands on the same shape costs no relayout.
	// Only the grid page has one, and showPage drops it with the page.
	cells []cell
	// relayout coalesces the reordering bursts a drag produces.
	relayout *idle.Coalescer
}

func New(sess *session.Session, drag *dnd.Controller, dispatch idle.Dispatch) *View {
	assert.IsNotNil(sess, "the tile area draws a session")
	assert.IsNotNil(drag, "the tile area shares the drag controller")
	assert.IsNotNil(dispatch, "the tile area defers its relayout to a UI loop")

	v := &View{
		root:    gtk.NewOverlay(),
		stack:   gtk.NewStack(),
		grid:    gtk.NewGrid(),
		spotBox: gtk.NewBox(gtk.OrientationVertical, gap),
		strip:   gtk.NewBox(gtk.OrientationHorizontal, gap),
		probe:   gtk.NewDrawingArea(),
		sess:    sess,
		drag:    drag,
		tiles:   map[int]*tile.Tile{},
	}
	v.relayout = idle.New(dispatch, v.rebuild)

	v.grid.SetColumnHomogeneous(true)
	v.grid.SetRowHomogeneous(true)
	v.grid.SetRowSpacing(gap)
	v.grid.SetColumnSpacing(gap)
	setMargins(v.grid, gap)

	v.strip.SetHAlign(gtk.AlignCenter)
	setMargins(v.spotBox, gap)

	v.stack.AddNamed(emptyState(), pageEmpty)
	v.stack.AddNamed(v.grid, pageGrid)
	v.stack.AddNamed(v.spotBox, pageSpot)
	v.showPage(pageEmpty)

	// The probe draws nothing and takes no input; it is in the tree for its
	// allocation, not for anything on screen.
	v.probe.SetCanTarget(false)
	v.probe.ConnectResize(func(_, _ int) { v.resized() })
	v.root.SetChild(v.stack)
	v.root.AddOverlay(v.probe)

	drag.AttachTarget(v.grid, v.tileWidget, v.relayout.Pending)
	return v
}

// Widget is the tile area, the split view's content.
func (v *View) Widget() gtk.Widgetter { return v.root }

func setMargins(w gtk.Widgetter, m int) {
	assert.IsNotNil(w, "a margin is set on a widget")
	assert.Assert(m >= 0, "a margin is a distance", m)

	base := gtk.BaseWidget(w)
	base.SetMarginTop(m)
	base.SetMarginBottom(m)
	base.SetMarginStart(m)
	base.SetMarginEnd(m)
}
