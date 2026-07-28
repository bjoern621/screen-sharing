package tile

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
)

// The two rates the picture is watched at, counted in frame-clock ticks.
const (
	// renderSettleFrames is how long a size has to hold before the player hears it.
	// Dragging a window re-allocates on every frame and every size handed over
	// renegotiates the branch behind the scaler, so the tile reports where a resize
	// came to rest rather than every size it passed through.
	renderSettleFrames = 8
	// stillEveryFrames is how often the picture is copied while frames arrive. Each
	// copy is a render pass, and one a second old is the picture a stream that
	// stopped was showing.
	stillEveryFrames = 60
)

// watchPicture starts the tile's heartbeat on the picture's frame clock. Two things
// the tile cannot be told about are read off it.
//
// The size the picture draws at: GTK4 raises no signal when a widget is allocated.
// The sink cannot work the size out on its own either, because what
// gtk4paintablesink learns about its window leaves the element as
// overlay-composition metadata and never reaches the caps, so the size has to arrive
// upstream as a caps bound.
//
// The last frame: the sink drops its texture when its pipeline stops, so the
// paintable is blank by the time the model reports the end. A copy taken while frames
// were still arriving is the only last frame there is left to show.
func (t *Tile) watchPicture() {
	assert.IsNotNil(t.picture, "a heartbeat runs on a built picture", t.stream.Name)
	assert.Assert(t.tick == 0, "a tile watches its picture once", t.stream.Name)

	t.stillOf = gtk.NewWidgetPaintable(t.picture)
	t.tick = t.picture.AddTickCallback(func(gtk.Widgetter, gdk.FrameClocker) bool {
		// The two readers share the tick because both belong to the picture's own clock:
		// the size is read after GTK allocated it, and the copy is of what it drew for
		// that frame.
		// Neither runs before the copy source exists.
		assert.IsNotNil(t.stillOf, "a heartbeat copies through a paintable of its picture", t.stream.Name)

		t.followSize()
		t.keepStill()
		return true
	})
}

// followSize hands the settled size of the picture to the player behind it.
func (t *Tile) followSize() {
	w, h := t.pictureSize()
	if w <= 0 || h <= 0 {
		// A widget GTK has not allocated yet has no size to render at.
		return
	}
	if w != t.seenW || h != t.seenH {
		t.seenW, t.seenH, t.settled = w, h, 0
		return
	}
	if w == t.sentW && h == t.sentH {
		return
	}
	if t.settled++; t.settled < renderSettleFrames {
		return
	}
	t.pushSize(w, h)
}

// pushSize tells the player how large its frames are drawn. A backend without the
// seam is left alone and keeps rendering at the size the source sends.
func (t *Tile) pushSize(w, h int) {
	assert.Assert(w > 0 && h > 0, "a render size is an allocation the picture holds", w, h)

	t.sentW, t.sentH = w, h
	sizer, ok := t.current.(player.RenderSizer)
	if !ok {
		return
	}
	sizer.SetRenderSize(w, h)
	logger.Debugf("tile %q draws %dx%d device pixels", t.stream.Name, w, h)
}

// pictureSize is the picture's size in device pixels, which is what the pipeline
// scales to: a tile on a 2x display draws twice the pixels its logical size names.
func (t *Tile) pictureSize() (w, h int) {
	scale := t.picture.ScaleFactor()
	return t.picture.Width() * scale, t.picture.Height() * scale
}

// keepStill copies the picture while frames arrive. Any other state already shows
// the last frame rather than producing one.
func (t *Tile) keepStill() {
	if t.state != session.Live || t.current == nil {
		return
	}
	if t.sinceStill++; t.sinceStill < stillEveryFrames {
		return
	}
	t.sinceStill = 0
	// The copy is of the picture alone, so it carries the video and none of the pages
	// that are drawn over it.
	t.still = t.stillOf.CurrentImage()
}

// showPicture puts the player's paintable under the tile while frames arrive and the
// still otherwise, which is what keeps the last frame under the reconnect and failure
// pages: both say the picture is what arrived, and the sink's paintable is blank as
// soon as its pipeline is gone.
//
// The swap waits for the live state rather than following the player, so a retry's
// fresh pipeline does not replace the last frame with black for as long as it takes
// to deliver one.
func (t *Tile) showPicture() {
	var want *gdk.Paintable
	if t.state == session.Live && t.current != nil {
		want = t.current.Paintable()
	} else if t.still != nil {
		want = t.still
	}
	if want == nil || want == t.shown {
		return
	}
	t.shown = want
	t.picture.SetPaintable(want)
}
