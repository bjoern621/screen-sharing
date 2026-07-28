package sidebar

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

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

// newStatus builds the status stack, one page per face.
//
// The two faces the design language has no color for take the ones it does: a stall
// is the failure glyph in the plain foreground, since the stream is still connected
// and destructive marks failure alone, and a pending retry is the retry glyph in
// destructive, since the pipeline behind it did fail.
func newStatus(icons *theme.Icons) *gtk.Stack {
	assert.IsNotNil(icons, "a status face is drawn from an icon set")

	s := gtk.NewStack()
	s.AddNamed(statusDot(false), pageIdle)
	s.AddNamed(theme.NewSpinner(statusSize).Widget(), pageLoading)
	s.AddNamed(icons.Image("refresh", statusSize, theme.Destructive), pageReconnect)
	s.AddNamed(statusDot(true), pageLive)
	s.AddNamed(icons.Image("alert-triangle", statusSize, theme.Foreground), pageStalled)
	s.AddNamed(icons.Image("alert-triangle", statusSize, theme.Destructive), pageFailed)

	// The table is what a row draws from, so a face without a page here is a row that
	// would ask the stack for a child it does not carry.
	for state, f := range statusFaceByState {
		assert.IsNotNil(s.ChildByName(f.page), "the stack carries a page per face", state.String(), f.page)
	}
	assert.IsNotNil(s.ChildByName(stalledFace.page), "the stack carries a page per face", stalledFace.page)
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
