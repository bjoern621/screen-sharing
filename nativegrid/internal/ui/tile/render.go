package tile

import (
	"reflect"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
)

// visual is which of the tile's overlay pages a watch state shows. The picture
// stays underneath all of them: a dead receive pipeline has nothing to recover, so
// a tile that was live keeps its last frame under the message.
type visual struct {
	skeleton, chip, reconnect, failure bool
}

// visualByState is the tile's chrome per state. The web tile hides the corner label
// while connecting because the skeleton already names the stream; this tile does the
// same, and the reconnect page names it the same way. Reconnecting takes the waiting
// treatment rather than the failure page because a retry is already scheduled, and
// the failure page is what the state that gave up shows. Idle is absent: a stream
// nobody watches has no tile.
var visualByState = map[session.State]visual{
	session.Loading:      {skeleton: true},
	session.Live:         {chip: true},
	session.Reconnecting: {reconnect: true},
	session.Failed:       {failure: true},
}

// visualOf is the chrome of one watch state. A state no tile draws fails here rather
// than leaving every page hidden over the picture.
func visualOf(s session.State) visual {
	v, ok := visualByState[s]
	assert.Assert(ok, "a tile draws a watched state", s.String())
	return v
}

// SetState draws one watch state. message is the pipeline's, printed by the two
// pages that report what it said: the failure page and the reconnect page.
func (t *Tile) SetState(s session.State, message string) {
	_, ok := visualByState[s]
	assert.Assert(ok, "a tile is handed a state it draws", s.String())

	t.state, t.message = s, message
	t.apply()
}

// SetStalled marks a tile whose stream stopped delivering frames while its pipeline
// keeps running. Nothing else on the tile says so: the watch state is still live and
// a frozen picture looks like a still one. The last frame stays visible under the
// marking, because it is what arrived, and a stream that comes back leaves it right.
func (t *Tile) SetStalled(on bool) {
	t.stalled = on
	t.apply()
}

// SetStream refreshes the configuration the tile draws.
// A watch leg the app moved arrives as a roster push, and what the tile names off the
// configuration follows it: the chip's tooltip on this render pass, the stats card's
// transport and source rows on its next poll.
//
// A tile belongs to one stream for as long as it is open, so a push carries new
// attributes for the stream it was opened for and never another one.
func (t *Tile) SetStream(st roster.Stream) {
	assert.Assert(st.Name == t.stream.Name, "a tile follows the stream it was opened for", t.stream.Name, st.Name)

	if reflect.DeepEqual(t.stream, st) {
		return
	}
	t.stream = st
	t.apply()
}

// apply is the tile's render function: it maps every state field to the widgets, and it
// is the only place that writes one.
// Each property is set in both of its branches, so one call restores the whole tile
// instead of leaving what an earlier state switched on.
//
// It converges on the fields rather than stepping from what is on screen, so a second
// pass over unchanged fields writes the same values and changes nothing.
func (t *Tile) apply() {
	v := visualOf(t.state)
	// A stall is a marking on a live stream, not a state: every other state draws a
	// page that already says more than a frozen picture would.
	stalled := t.stalled && t.state == session.Live

	t.skeleton.SetVisible(v.skeleton)

	t.reconnect.SetVisible(v.reconnect)
	t.reconnectLabel.SetText(t.message)
	// A drop the model has no message for shows the waiting lines alone rather than a
	// blank row under them.
	t.reconnectLabel.SetVisible(t.message != "")

	t.failure.SetVisible(v.failure)
	t.failureLabel.SetText(t.message)

	t.chip.SetVisible(v.chip)
	// gotk4 passes NULL rather than an empty C string for "", so this one call is both
	// branches: a stream with no tip unsets the tooltip instead of hovering an empty box.
	t.chip.SetTooltipText(watchLegTip(t.stream))
	t.chipStall.SetVisible(stalled)
	t.chipMute.SetVisible(t.muted)
	t.stall.SetVisible(stalled)

	t.statsCard.SetVisible(t.statsOpen)
	t.statsToggle.SetOn(t.statsOpen)

	size := sizingOf(t.shape)
	t.root.SetSizeRequest(size.width, size.height)
	t.root.SetHExpand(size.expand)
	t.root.SetVExpand(size.expand)
	setClass(t.root, "spotlit", t.spotlit)
	t.spot.SetOn(t.spotlit)

	setClass(t.controls, "shown", t.hovered)
	t.volume.Widget().SetVisible(t.audio)
	t.volume.SetOn(t.muted)
	// The slider is out only where there is audio to change, so a stream that turns out
	// to carry none folds it away with the button that opened it.
	t.volScale.SetValue(t.level)
	t.volRevealer.SetRevealChild(t.volOpen && t.audio)

	t.showPicture()
	t.applyAudio()
}

// watchLegTip names the leg the stream arrives over, for the corner chip.
// A roster that left the transport out yields no tooltip rather than a sentence with a
// hole in it.
func watchLegTip(st roster.Stream) string {
	if st.Transport == "" {
		return ""
	}
	return "Watching this stream over " + st.Transport +
		", the watch leg (relay to viewer). It is independent of the protocol the stream was published over."
}

// setClass puts a CSS class on a widget or takes it off, the one decision GTK splits
// over two calls.
// A render pass runs both directions, which is what keeps a class from outliving the
// state that added it.
func setClass(w gtk.Widgetter, class string, on bool) {
	assert.IsNotNil(w, "a styled widget exists", class)

	base := gtk.BaseWidget(w)
	if on {
		base.AddCSSClass(class)
		return
	}
	base.RemoveCSSClass(class)
}
