// Package tile is one stream's tile: the player's picture under a loading
// skeleton, the corner label chip, the pages a dropped or frozen stream shows, a
// failure page with a retry, and the media controls of the web tile that translate
// to a native window.
//
// The tile watches its own picture (picture.go): the size it draws at bounds what
// the pipeline scales to, and a copy of the video taken while it plays is what stays
// on screen once the pipeline is gone.
//
// Pop-out has no meaning here (the grid is its own window) and hide-video would
// need the roster's audio-only strip, so both stay web-only.
package tile

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/stats"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/widgets"
)

// stripWidth and stripHeight size the film-strip tiles under a spotlit stream: the
// web grid's h-28 strip (112px) at 16:9.
const (
	stripWidth  = 199
	stripHeight = 112
)

// doubleClick is the press count that spotlights a tile.
const doubleClick = 2

// Hooks are a tile's callbacks into the model, one per control that changes what
// the window watches.
type Hooks struct {
	Retry      func()
	ToggleSpot func()
	Disconnect func()
}

// Shape is how a container sizes a tile.
type Shape int

const (
	ShapeFill  Shape = iota // fills its cell: a grid tile, and the spotlit one
	ShapeStrip              // a fixed thumbnail in the spotlight's film strip
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

// Tile is one stream's tile.
type Tile struct {
	root    *gtk.Overlay
	picture *gtk.Picture
	icons   *theme.Icons
	hooks   Hooks
	// stream is the configuration the tile was opened for; the overlay names the
	// transport and source fragment it plays.
	stream roster.Stream
	// current is the player the tile draws, nil until the first Attach.
	current player.Player
	// state is the watch state the tile last drew and stalled the marking layered
	// over it. The two arrive from the model separately, so both are kept: either
	// one changing redraws from both.
	state   session.State
	stalled bool

	// The picture's heartbeat (picture.go). still is the copy of the video the tile
	// falls back to once the player's paintable blanks, taken through stillOf every
	// stillEveryFrames ticks, and shown is the paintable the picture last got.
	tick       uint
	stillOf    *gtk.WidgetPaintable
	still      *gdk.Paintable
	shown      *gdk.Paintable
	sinceStill int
	// seen is the picture size the heartbeat last read and sent the one the player
	// was last told, both in device pixels; settled counts the frames seen has held.
	seenW, seenH int
	sentW, sentH int
	settled      int

	skeleton       *gtk.Box
	chip           *gtk.Box
	chipStall      *gtk.Image
	chipMute       *gtk.Image
	stall          *gtk.Box
	reconnect      *gtk.Box
	reconnectLabel *gtk.Label
	failure        *gtk.Box
	failureLabel   *gtk.Label

	controls    *gtk.Box
	volume      *widgets.IconToggle
	volRevealer *gtk.Revealer
	volScale    *gtk.Scale
	muted       bool

	statsToggle *widgets.IconToggle
	statsCard   *stats.Card
	statsOpen   bool
	// statsGen counts the toggles of the overlay, so the refresh a closed card left
	// pending recognizes itself as the one an open has superseded.
	statsGen uint
	poller   stats.Poller

	spot *widgets.IconToggle
}

// New builds a tile in the loading state. The stream's player arrives through
// Attach, which the grid calls as soon as one runs.
func New(st roster.Stream, hooks Hooks) *Tile {
	assert.IsNotNil(hooks.Retry, "a tile retries through its hooks", st.Name)
	assert.IsNotNil(hooks.ToggleSpot, "a tile spotlights through its hooks", st.Name)
	assert.IsNotNil(hooks.Disconnect, "a tile disconnects through its hooks", st.Name)

	t := &Tile{
		picture: gtk.NewPicture(),
		icons:   &theme.Icons{},
		hooks:   hooks,
		stream:  st,
		// The pages are built showing the loading one, so the tile draws a state
		// before the grid hands it the first one.
		state: session.Loading,
	}
	t.picture.SetHExpand(true)
	t.picture.SetVExpand(true)

	t.buildSkeleton()
	t.buildChip()
	t.buildStall()
	t.buildReconnect()
	t.buildFailure()
	t.statsCard = stats.NewCard()
	t.buildControls()

	t.root = gtk.NewOverlay()
	t.root.AddCSSClass("tile")
	t.root.SetChild(t.picture)
	t.root.AddOverlay(t.skeleton)
	t.root.AddOverlay(t.reconnect)
	t.root.AddOverlay(t.chip)
	t.root.AddOverlay(t.stall)
	t.root.AddOverlay(t.failure)
	t.root.AddOverlay(t.statsCard.Widget())
	t.root.AddOverlay(t.controls)
	t.root.SetHExpand(true)
	t.root.SetVExpand(true)

	// The controls fade in while the pointer is on the tile, the web tile's
	// group-hover. Leaving also folds the volume slider away.
	hover := gtk.NewEventControllerMotion()
	hover.ConnectEnter(func(x, y float64) { t.controls.AddCSSClass("shown") })
	hover.ConnectLeave(func() {
		t.controls.RemoveCSSClass("shown")
		t.volRevealer.SetRevealChild(false)
	})
	t.root.AddController(hover)

	// A double click is the second way to the spotlight, beside the control bar's
	// button. The gesture claims no event sequence, so a press that turns into a
	// drag stays the drag source's, which is attached to this same widget.
	spot := gtk.NewGestureClick()
	spot.SetButton(gdk.BUTTON_PRIMARY)
	spot.ConnectPressed(func(nPress int, x, y float64) {
		if nPress == doubleClick && !t.onOverlayControl(x, y) {
			t.hooks.ToggleSpot()
		}
	})
	t.root.AddController(spot)

	t.watchPicture()
	logger.Debugf("tile opened for %q", st.Name)
	return t
}

// onOverlayControl reports whether a press landed on one of the pages the tile puts
// over the picture to be operated: the control bar, the open stats card, the failure
// page with its retry. Each answers a click of its own, and a second one on it is not
// a request to spotlight.
func (t *Tile) onOverlayControl(x, y float64) bool {
	picked := t.root.Pick(x, y, gtk.PickDefault)
	if picked == nil {
		return false
	}
	hit := gtk.BaseWidget(picked)
	for _, over := range []gtk.Widgetter{t.controls, t.statsCard.Widget(), t.failure} {
		base := gtk.BaseWidget(over)
		if hit.Eq(base) || hit.IsAncestor(base) {
			return true
		}
	}
	return false
}

// Widget is the tile's root, for a container to hold and a drag source to attach
// to.
func (t *Tile) Widget() gtk.Widgetter { return t.root }

// SetState draws one watch state. message is the pipeline's, printed by the two
// pages that report what it said: the failure page and the reconnect page.
func (t *Tile) SetState(s session.State, message string) {
	v := visualOf(s)
	if v.failure {
		t.failureLabel.SetText(message)
	}
	if v.reconnect {
		// A drop the model has no message for shows the waiting lines alone rather
		// than a blank row under them.
		t.reconnectLabel.SetText(message)
		t.reconnectLabel.SetVisible(message != "")
	}
	t.state = s
	t.apply(v)
}

// SetStalled marks a tile whose stream stopped delivering frames while its pipeline
// keeps running. Nothing else on the tile says so: the watch state is still live and
// a frozen picture looks like a still one. The last frame stays visible under the
// marking, because it is what arrived, and a stream that comes back leaves it right.
func (t *Tile) SetStalled(on bool) {
	t.stalled = on
	t.apply(visualOf(t.state))
}

// apply draws the state's chrome with the stall marking over it.
func (t *Tile) apply(v visual) {
	// A stall is a marking on a live stream, not a state: every other state draws a
	// page that already says more than a frozen picture would.
	stalled := t.stalled && t.state == session.Live

	t.skeleton.SetVisible(v.skeleton)
	t.reconnect.SetVisible(v.reconnect)
	t.chip.SetVisible(v.chip)
	t.chipStall.SetVisible(stalled)
	t.stall.SetVisible(stalled)
	t.failure.SetVisible(v.failure)
	t.showPicture()
}

// Attach hands the tile the player it draws. A retry arrives here with a fresh
// player, so the mute and volume the tile shows are applied to it rather than the
// tile falling back to what a new pipeline defaults to.
func (t *Tile) Attach(p player.Player) {
	assert.IsNotNil(p, "a tile draws a player", t.stream.Name)

	t.current = p
	// A fresh pipeline renders at the size the source sends until it is told
	// otherwise, so what the last player was told is forgotten and the size the tile
	// already has goes over at once rather than after the heartbeat settles again.
	t.sentW, t.sentH = 0, 0
	if w, h := t.pictureSize(); w > 0 && h > 0 {
		t.pushSize(w, h)
	}
	t.showPicture()
	p.SetMuted(t.muted)
	p.SetVolume(t.volScale.Value())
}

// SetAudioAvailable offers or withdraws the volume control, which is hidden until
// the stream turns out to carry audio, like the web tile hides VolumeControl on a
// video-only sink.
//
// The audio branch appears while the pipeline plays, after Attach, so the mute and
// volume the tile shows reach the branch from here rather than from the attach.
func (t *Tile) SetAudioAvailable(on bool) {
	t.volume.Widget().SetVisible(on)
	if !on || t.current == nil {
		return
	}
	t.current.SetMuted(t.muted)
	t.current.SetVolume(t.volScale.Value())
}

// SetShape sizes the tile for the layout it is in. The size the shape works out to
// is GTK's to allocate, so it reaches the player from the heartbeat that reads it
// back off the picture rather than from here.
func (t *Tile) SetShape(s Shape) {
	switch s {
	case ShapeFill:
		t.root.SetSizeRequest(-1, -1)
		t.root.SetHExpand(true)
		t.root.SetVExpand(true)
	case ShapeStrip:
		t.root.SetSizeRequest(stripWidth, stripHeight)
		t.root.SetHExpand(false)
		t.root.SetVExpand(false)
	default:
		assert.Never("unexpected tile shape", int(s))
	}
}

// SetSpotlit styles the tile for its place in the spotlight layout: the upgraded
// ring on the spotlit tile, and the spot button flipping between maximize and
// minimize like the web tile's.
func (t *Tile) SetSpotlit(on bool) {
	if on {
		t.root.AddCSSClass("spotlit")
	} else {
		t.root.RemoveCSSClass("spotlit")
	}
	t.spot.SetOn(on)
}

// Dispose releases the tile's themed icons and stops its heartbeat; the grid calls
// it when the tile leaves the watch set, while the widgets are still in the tree.
func (t *Tile) Dispose() {
	t.picture.RemoveTickCallback(t.tick)
	t.icons.Release()
	logger.Debugf("tile closed for %q", t.stream.Name)
}
