package tile

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/renderpick"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/widgets"
)

// The volume card's slider and its reveal, the web tile's hover-out slider.
const (
	volumeSteps    = 0.05
	volumeHeight   = 96
	revealDuration = 200
)

// The render-chain control's geometry, the icon at the size every control on the bar
// is drawn at.
const (
	renderIconSize = 16
	renderMargin   = 12
	renderSpacing  = 8
)

// renderTip describes the control on the bar. It says what the choice is about and
// what it costs, and each row of the picker says what that chain does.
const renderTip = "Render chain this tile draws through: what scales and converts its decoded frames on the way to the screen, where it does that, and what it says about the colour they arrive in. " +
	"Picking one restarts this tile on it, since a chain is fixed when the pipeline is built. The sidebar's header holds the chain every other tile follows."

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
	bar.Append(t.buildRenderButton())
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

// buildRenderButton is the bar's render-chain control: a button whose popover holds
// the picker of this stream's own chain, leading with the entry that follows the
// window's default.
//
// It is on the bar rather than in the sidebar row's watch-leg popover on purpose.
// That popover composes a whole leg and commits it on Apply, and one popover with two
// commit models is a trap: a chain is one change, and this control applies it the
// moment it is picked.
func (t *Tile) buildRenderButton() *gtk.MenuButton {
	assert.IsNotNil(t.hooks.SetRenderChain, "a control bar moves its stream's render chain through the tile's hooks", t.stream.Name)

	t.renderPick = renderpick.New(true, t.hooks.SetRenderChain)

	body := gtk.NewBox(gtk.OrientationVertical, renderSpacing)
	body.SetMarginTop(renderMargin)
	body.SetMarginBottom(renderMargin)
	body.SetMarginStart(renderMargin)
	body.SetMarginEnd(renderMargin)
	body.Append(t.renderPick.Widget())

	popover := gtk.NewPopover()
	popover.SetChild(body)

	t.renderButton = gtk.NewMenuButton()
	t.renderButton.AddCSSClass("flat")
	t.renderButton.SetChild(t.icons.Image("video", renderIconSize, theme.Foreground))
	t.renderButton.SetTooltipText(renderTip)
	t.renderButton.SetPopover(popover)
	return t.renderButton
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
