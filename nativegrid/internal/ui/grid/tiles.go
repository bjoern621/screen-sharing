package grid

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/tile"
)

// syncTile brings stream i's tile in line with its watch state: opened on the first
// watch, redrawn on every state after that, and gone once the stream is idle. It
// reports whether the set of open tiles changed, which is what the layout follows.
//
// Everything the tile shows is set on every pass, so a second call with the model
// unchanged leaves the same tile drawing the same thing.
func (v *View) syncTile(i int) (changed bool) {
	assert.Assert(i >= 0 && i < v.sess.Len(), "a tile follows a stream the model knows", i, v.sess.Len())

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
			Retry:      func() { v.sess.Restart(i) },
			ToggleSpot: func() { v.sess.ToggleSpot(i) },
			Disconnect: func() { v.sess.SetWatched(i, false) },
		})
		v.drag.AttachSource(t.Widget(), i)
		v.tiles[i] = t
	}
	// The stream is pushed rather than left at what the tile opened with: a restart
	// on a moved watch leg comes through here, and the tile names the leg it plays.
	t.SetStream(v.sess.Stream(i))
	t.SetState(state, v.sess.Message(i))
	// The stall is read rather than waited for, so a tile that opens on a stream
	// already stalled draws it from its first frame.
	t.SetStalled(v.sess.Stalled(i))
	// The model's player goes over on every pass, the nil a factory failure leaves among
	// them: a tile that kept the last one would go on driving a pipeline the session
	// already stopped.
	t.Attach(v.sess.Player(i))
	t.SetAudioAvailable(v.sess.HasAudio(i))
	return !open
}

// refreshStreams reads the model's streams back into the open tiles.
// A tile keeps the stream it draws from, so a roster push that moves a watched
// stream to another watch leg reaches its chip and its stats card through here and
// nowhere else.
func (v *View) refreshStreams() {
	for i, t := range v.tiles {
		t.SetStream(v.sess.Stream(i))
	}
}

// tileWidget is the tile area's widget for stream i, nil while the stream has no
// tile, which is what the drag's hit test skips.
func (v *View) tileWidget(i int) gtk.Widgetter {
	assert.Assert(i >= 0 && i < v.sess.Len(), "a drag hit-tests a stream the model knows", i, v.sess.Len())

	t, open := v.tiles[i]
	// nil reads as "this stream draws nothing", which is the right answer for an
	// unwatched one and would silently swallow a watched stream that lost its tile.
	assert.Assert(open || !v.sess.State(i).Watched(), "a watched stream holds a tile", i)
	if !open {
		return nil
	}
	return t.Widget()
}

// tileAt is the open tile of a stream the caller already knows has one.
func (v *View) tileAt(i int) *tile.Tile {
	t, open := v.tiles[i]
	assert.Assert(open, "a laid out stream holds a tile", i)

	return t
}
