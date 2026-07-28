package tile

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/widgets"
)

// The volume card's slider and its reveal, the web tile's hover-out slider.
const (
	volumeSteps    = 0.05
	volumeHeight   = 96
	revealDuration = 200
)

// buildControls assembles the bottom-center control bar and the volume slider card
// that slides up over it. The bar carries the web tile's controls in its order:
// mute, stats, spotlight, disconnect.
//
// The widgets are built and put together here.
// What they show is the render pass's (render.go) and what drives them is input.go's.
func (t *Tile) buildControls() {
	assert.IsNotNil(t.icons, "a control bar draws from the tile's icon set")

	t.volume = widgets.NewIconToggle(t.icons,
		widgets.Face{Icon: "volume", Color: theme.Foreground, Tooltip: "Silence this tile without stopping playback. The corner chip marks it while it lasts."},
		widgets.Face{Icon: "volume-off", Color: theme.Foreground, Tooltip: "Play this tile's audio again, at the volume the slider holds."},
		func() { t.setMuted(!t.muted) })

	t.statsToggle = widgets.NewIconToggle(t.icons,
		widgets.Face{Icon: "info-circle", Color: theme.Foreground, Tooltip: "Show the pipeline figures for this tile: what it subscribes to, the video on the wire, what the decoder makes of it, and the transport's own counters."},
		widgets.Face{Icon: "info-circle", Color: theme.Primary, Tooltip: "Hide the figures overlay and leave the picture alone."},
		t.toggleStats)

	t.spot = widgets.NewIconToggle(t.icons,
		widgets.Face{Icon: "maximize", Color: theme.Foreground, Tooltip: "Give this tile the whole grid. The other streams keep playing and return on leaving spotlight."},
		widgets.Face{Icon: "minimize", Color: theme.Foreground, Tooltip: "Return this tile to the grid beside the other streams."},
		t.hooks.ToggleSpot)

	disconnect := widgets.IconButton(t.icons,
		widgets.Face{Icon: "plug-connected-x", Color: theme.Destructive, Tooltip: "Stop watching this stream and close its tile. The stream keeps running at the relay."},
		t.hooks.Disconnect)

	bar := gtk.NewBox(gtk.OrientationHorizontal, 2)
	bar.AddCSSClass("controls-card")
	bar.Append(t.volume.Widget())
	bar.Append(t.statsToggle.Widget())
	bar.Append(t.spot.Widget())
	bar.Append(disconnect)

	t.controls = gtk.NewBox(gtk.OrientationVertical, 4)
	t.controls.AddCSSClass("tile-controls")
	t.controls.SetHAlign(gtk.AlignCenter)
	t.controls.SetVAlign(gtk.AlignEnd)
	t.controls.SetMarginBottom(8)
	t.controls.Append(t.buildVolumeCard())
	t.controls.Append(bar)
}

// buildVolumeCard is the vertical slider that slides up over the bar.
// Its range is the fraction a player takes, and the value on it is the tile's, written
// on every render pass.
func (t *Tile) buildVolumeCard() *gtk.Revealer {
	t.volScale = gtk.NewScaleWithRange(gtk.OrientationVertical, 0, fullVolume, volumeSteps)
	t.volScale.SetInverted(true)
	t.volScale.SetSizeRequest(-1, volumeHeight)

	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("vol-card")
	card.Append(t.volScale)

	t.volRevealer = gtk.NewRevealer()
	t.volRevealer.SetTransitionType(gtk.RevealerTransitionTypeSlideUp)
	t.volRevealer.SetTransitionDuration(revealDuration)
	t.volRevealer.SetChild(card)
	t.volRevealer.SetHAlign(gtk.AlignStart)
	return t.volRevealer
}
