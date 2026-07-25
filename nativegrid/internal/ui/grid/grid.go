// Package grid is the tile area: an empty-state page until a stream is watched,
// then a near-square grid of tiles, or the spotlight layout when one stream is
// spotlit.
//
// The view owns the tiles and nothing else. What is watched, in what order, and
// which stream is spotlit is the model's; the view redraws from what it reads back
// when the model reports a change.
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
	stack   *gtk.Stack
	grid    *gtk.Grid
	spotBox *gtk.Box // the spotlight page: the spotlit tile over the strip
	strip   *gtk.Box
	sess    *session.Session
	drag    *dnd.Controller
	// tiles are the open tiles by stream index, so an index that was never watched
	// costs nothing.
	tiles map[int]*tile.Tile
	// relayout coalesces the reordering bursts a drag produces.
	relayout *idle.Coalescer
}

func New(sess *session.Session, drag *dnd.Controller, dispatch idle.Dispatch) *View {
	assert.IsNotNil(sess, "the tile area draws a session")
	assert.IsNotNil(drag, "the tile area shares the drag controller")

	v := &View{
		stack:   gtk.NewStack(),
		grid:    gtk.NewGrid(),
		spotBox: gtk.NewBox(gtk.OrientationVertical, gap),
		strip:   gtk.NewBox(gtk.OrientationHorizontal, gap),
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
	v.stack.SetVisibleChildName(pageEmpty)

	drag.AttachTarget(v.grid, v.tileWidget, v.relayout.Pending)
	return v
}

// Widget is the tile area, the split view's content.
func (v *View) Widget() gtk.Widgetter { return v.stack }

// Changed redraws for one model change.
func (v *View) Changed(c session.Change) {
	switch c.Kind {
	case session.StateChanged:
		// Only a tile that came or went changes the layout. A stream going live or
		// failing redraws inside its tile, and a rebuild would reparent every tile
		// for it, which a drag in flight would feel.
		if v.syncTile(c.Index) {
			v.rebuild()
		}
	case session.AudioReady:
		if t, ok := v.tiles[c.Index]; ok {
			t.SetAudioAvailable(v.sess.HasAudio(c.Index))
		}
	case session.OrderChanged:
		// Reordering must not reparent widgets from inside a drag-and-drop callback,
		// so every order change lands on the next loop pass.
		v.relayout.Schedule()
	}
}

// syncTile brings stream i's tile in line with its watch state: opened on the first
// watch, redrawn on every state after that, and gone once the stream is idle. It
// reports whether the set of open tiles changed, which is what the layout follows.
func (v *View) syncTile(i int) (changed bool) {
	state := v.sess.State(i)
	t, open := v.tiles[i]

	if !state.Watched() {
		if open {
			t.Dispose()
			delete(v.tiles, i)
		}
		return open
	}
	if !open {
		t = tile.New(v.sess.Stream(i), tile.Hooks{
			Retry:      func() { v.sess.SetWatched(i, true) },
			ToggleSpot: func() { v.sess.ToggleSpot(i) },
			Disconnect: func() { v.sess.SetWatched(i, false) },
		})
		v.drag.AttachSource(t.Widget(), i)
		v.tiles[i] = t
	}
	t.SetState(state, v.sess.Message(i))
	// A player exists in every state but a failure the factory reported, and a retry
	// arrives here with a fresh one.
	if p := v.sess.Player(i); p != nil {
		t.Attach(p)
	}
	t.SetAudioAvailable(v.sess.HasAudio(i))
	return !open
}

// tileWidget is the tile area's widget for stream i, nil while the stream has no
// tile, which is what the drag's hit test skips.
func (v *View) tileWidget(i int) gtk.Widgetter {
	t, open := v.tiles[i]
	if !open {
		return nil
	}
	return t.Widget()
}

func setMargins(w gtk.Widgetter, m int) {
	base := gtk.BaseWidget(w)
	base.SetMarginTop(m)
	base.SetMarginBottom(m)
	base.SetMarginStart(m)
	base.SetMarginEnd(m)
}
