package sidebar

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// statusSize is the size of a row's trailing status widget, the web roster chip's
// status in miniature.
const statusSize = 16

// The pages of a row's status stack.
const (
	pageIdle      = "idle"
	pageLoading   = "loading"
	pageReconnect = "reconnecting"
	pageLive      = "live"
	pageStalled   = "stalled"
	pageFailed    = "failed"
)

// statusFace is one drawn condition: the stack page it shows, and what the face
// means in words, which a 16px glyph cannot say on its own.
type statusFace struct {
	page string
	tip  string
}

// statusFaceByState is which face a row shows per watch state: a static muted dot
// when idle, the spinning loader while connecting, the retry glyph in destructive
// while a dropped stream is brought back, a pulsing accent dot once live, and a
// destructive triangle once nothing more is tried.
var statusFaceByState = map[session.State]statusFace{
	session.Idle:         {page: pageIdle, tip: "Not watching this stream. The row's check opens a tile for it."},
	session.Loading:      {page: pageLoading, tip: "Opening the watch leg and waiting for the first frame."},
	session.Reconnecting: {page: pageReconnect, tip: "The connection dropped and a retry is pending. The stream's tile keeps its last frame until the stream is back, or until the retries run out and it fails."},
	session.Live:         {page: pageLive, tip: "Watching this stream. Frames are arriving from the relay."},
	session.Failed:       {page: pageFailed, tip: "The connection failed. The stream's tile carries the pipeline's message and a retry."},
}

// stalledFace is what a live row shows while no frames arrive.
var stalledFace = statusFace{
	page: pageStalled,
	tip:  "Watching this stream, but no frames are arriving. Its tile keeps the last frame that did, and the connection to the relay is still open.",
}

// faceFor is the face one row draws. A stall is not a State: the model reports it
// beside one, and only a live stream can be stalled, so it replaces the live face
// and no other.
func faceFor(state session.State, stalled bool) statusFace {
	if stalled && state == session.Live {
		return stalledFace
	}
	f, ok := statusFaceByState[state]
	assert.Assert(ok, "a row draws every watch state", state.String())
	return f
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
	assert.IsNotNil(watch, "a row watches through a callback", name)

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

// newStatus builds the status stack, one page per face.
//
// The two faces the design language has no color for take the ones it does: a stall
// is the failure glyph in the plain foreground, since the stream is still connected
// and destructive marks failure alone, and a pending retry is the retry glyph in
// destructive, since the pipeline behind it did fail.
func newStatus(icons *theme.Icons) *gtk.Stack {
	s := gtk.NewStack()
	s.AddNamed(statusDot(false), pageIdle)
	s.AddNamed(theme.NewSpinner(statusSize).Widget(), pageLoading)
	s.AddNamed(icons.Image("refresh", statusSize, theme.Destructive), pageReconnect)
	s.AddNamed(statusDot(true), pageLive)
	s.AddNamed(icons.Image("alert-triangle", statusSize, theme.Foreground), pageStalled)
	s.AddNamed(icons.Image("alert-triangle", statusSize, theme.Destructive), pageFailed)
	return s
}

// statusDot is the small round state dot; live turns it accent-colored and pulsing
// via CSS.
func statusDot(live bool) *gtk.Box {
	d := gtk.NewBox(gtk.OrientationHorizontal, 0)
	d.AddCSSClass("status-dot")
	if live {
		d.AddCSSClass("live")
	}
	d.SetHAlign(gtk.AlignCenter)
	d.SetVAlign(gtk.AlignCenter)
	return d
}
