// Package tile is one stream's tile: the player's picture under a loading
// skeleton, the corner label chip, a failure page with a retry, and the media
// controls of the web tile that translate to a native window.
//
// Pop-out has no meaning here (the grid is its own window) and hide-video would
// need the roster's audio-only strip, so both stay web-only.
package tile

import (
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
// a tile that was live keeps its last frame under the failure message.
type visual struct {
	skeleton, chip, failure bool
}

// visualByState is the tile's chrome per state. The web tile hides the corner label
// while connecting because the skeleton already names the stream; this tile does the
// same. Idle is absent: a stream nobody watches has no tile.
var visualByState = map[session.State]visual{
	session.Loading: {skeleton: true},
	session.Live:    {chip: true},
	session.Failed:  {failure: true},
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

	skeleton     *gtk.Box
	chip         *gtk.Box
	chipMute     *gtk.Image
	failure      *gtk.Box
	failureLabel *gtk.Label

	controls    *gtk.Box
	volume      *widgets.IconToggle
	volRevealer *gtk.Revealer
	volScale    *gtk.Scale
	muted       bool

	statsToggle *widgets.IconToggle
	statsCard   *stats.Card
	statsOpen   bool
	poller      stats.Poller

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
	}
	t.picture.SetHExpand(true)
	t.picture.SetVExpand(true)

	t.buildSkeleton()
	t.buildChip()
	t.buildFailure()
	t.statsCard = stats.NewCard()
	t.buildControls()

	t.root = gtk.NewOverlay()
	t.root.AddCSSClass("tile")
	t.root.SetChild(t.picture)
	t.root.AddOverlay(t.skeleton)
	t.root.AddOverlay(t.chip)
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

	logger.Debugf("tile opened for %q", st.Name)
	return t
}

// Widget is the tile's root, for a container to hold and a drag source to attach
// to.
func (t *Tile) Widget() gtk.Widgetter { return t.root }

// SetState draws one watch state. message is the failure text, read only in the
// failed state.
func (t *Tile) SetState(s session.State, message string) {
	v, ok := visualByState[s]
	assert.Assert(ok, "a tile draws a watched state", s.String())

	t.skeleton.SetVisible(v.skeleton)
	t.chip.SetVisible(v.chip)
	t.failure.SetVisible(v.failure)
	if v.failure {
		t.failureLabel.SetText(message)
	}
}

// Attach hands the tile the player it draws. A retry arrives here with a fresh
// player, so the mute and volume the tile shows are applied to it rather than the
// tile falling back to what a new pipeline defaults to.
func (t *Tile) Attach(p player.Player) {
	assert.IsNotNil(p, "a tile draws a player", t.stream.Name)

	t.current = p
	t.picture.SetPaintable(p.Paintable())
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

// SetShape sizes the tile for the layout it is in.
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

// Dispose releases the tile's themed icons; the grid calls it when the tile leaves
// the watch set.
func (t *Tile) Dispose() {
	t.icons.Release()
	logger.Debugf("tile closed for %q", t.stream.Name)
}
