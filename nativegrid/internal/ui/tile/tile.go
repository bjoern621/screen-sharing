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
//
// What the tile looks like is decided in one place.
// A setter writes a state field and calls apply (render.go), which maps the fields to
// the widgets, and the controllers (input.go) do the same rather than reaching into a
// widget themselves.
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

// Hooks are a tile's callbacks into the model, one per control that changes what
// the window watches.
type Hooks struct {
	Retry      func()
	ToggleSpot func()
	Disconnect func()
}

// Tile is one stream's tile.
//
// The state fields are the render pass's input, and a setter is the only writer of one.
// A widget property written anywhere else is a second definition of what the tile looks
// like, and the render pass alone can no longer restore it.
type Tile struct {
	root    *gtk.Overlay
	picture *gtk.Picture
	icons   *theme.Icons
	hooks   Hooks

	// stream is the configuration the tile draws, refreshed through SetStream.
	// The overlay names the transport and source fragment it plays, and a watch leg the
	// app moved changes both.
	stream roster.Stream
	// current is the player the tile draws, nil until the first Attach.
	current player.Player
	// state is the watch state the tile draws, message the pipeline's last words that
	// the failure and reconnect pages print, and stalled the marking layered over a live
	// stream that stopped delivering frames.
	// State and stall arrive from the model separately, so both are kept: either one
	// changing redraws from both.
	state   session.State
	message string
	stalled bool
	// shape is how the layout sizes the tile, and spotlit whether it is the stream the
	// spotlight is on.
	shape   Shape
	spotlit bool
	// audio is whether the stream turned out to carry audio, which is what the volume
	// control waits for.
	// muted and level are the tile's own audio state, held here rather than in the
	// pipeline because a retry brings a player that defaults to neither (audio.go).
	audio bool
	muted bool
	level float64
	// hovered is the pointer on the tile, which fades the controls in, and volOpen the
	// pointer on the volume button, which slides the slider out over them.
	hovered bool
	volOpen bool

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
		// The tile draws a state before the grid hands it the first one, and fills its
		// cell until a layout says otherwise.
		state: session.Loading,
		shape: ShapeFill,
		level: fullVolume,
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

	t.wireInput()
	t.watchPicture()
	// The builders leave the widgets in whatever state their constructors give them.
	// The first render pass is what puts the loading tile on screen.
	t.apply()

	logger.Debugf("tile opened for %q", st.Name)
	return t
}

// Widget is the tile's root, for a container to hold and a drag source to attach
// to.
func (t *Tile) Widget() gtk.Widgetter { return t.root }

// Dispose releases the tile's themed icons and stops its heartbeat; the grid calls
// it when the tile leaves the watch set, while the widgets are still in the tree.
func (t *Tile) Dispose() {
	assert.Assert(t.tick != 0, "a tile stops the heartbeat it started", t.stream.Name)

	t.picture.RemoveTickCallback(t.tick)
	t.tick = 0
	t.icons.Release()
	logger.Debugf("tile closed for %q", t.stream.Name)
}
