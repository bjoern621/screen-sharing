package grid

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/session"
)

// Changed redraws for one model change.
//
// Every kind the model reports is answered, including the ones the tile area draws
// nothing for.
// A kind that falls through leaves the tiles standing for a model that has moved on,
// and nothing on screen says so.
func (v *View) Changed(c session.Change) {
	switch c.Kind {
	case session.StreamAdded:
		// A stream joins the model unwatched and an unwatched stream has no tile, so
		// the sidebar offers it and the tile area hears of it again once it is watched.
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
	case session.RosterChanged:
		// A push carries the watch leg a stream arrives over, which the tile names in
		// its chip and its stats card.
		// The watch set is untouched, so no tile opens or closes and nothing but the
		// streams behind them changes.
		v.refreshStreams()
	case session.OrderChanged:
		// Reordering must not reparent widgets from inside a drag-and-drop callback,
		// so every order change lands on the next loop pass.
		v.relayout.Schedule()
	case session.StallChanged:
		// A stall leaves the watch set alone, so it redraws inside the one tile.
		if t, ok := v.tiles[c.Index]; ok {
			t.SetStalled(v.sess.Stalled(c.Index))
		}
	case session.AppChanged:
		// The publish state is the app's own and the sidebar carries its controls, so
		// nothing in the tile area stands for it.
	default:
		assert.Never("unexpected change kind", int(c.Kind))
	}
}
