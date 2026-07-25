package tile

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

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
func (t *Tile) buildControls() {
	t.volume = widgets.NewIconToggle(t.icons,
		widgets.Face{Icon: "volume", Color: theme.Foreground, Tooltip: "mute"},
		widgets.Face{Icon: "volume-off", Color: theme.Foreground, Tooltip: "unmute"},
		func() { t.setMuted(!t.muted) })
	// Hidden until the player reports audio (SetAudioAvailable).
	t.volume.Widget().SetVisible(false)

	t.statsToggle = widgets.NewIconToggle(t.icons,
		widgets.Face{Icon: "info-circle", Color: theme.Foreground, Tooltip: "stats"},
		widgets.Face{Icon: "info-circle", Color: theme.Primary, Tooltip: "hide stats"},
		t.toggleStats)

	t.spot = widgets.NewIconToggle(t.icons,
		widgets.Face{Icon: "maximize", Color: theme.Foreground, Tooltip: "spotlight"},
		widgets.Face{Icon: "minimize", Color: theme.Foreground, Tooltip: "back to grid"},
		t.hooks.ToggleSpot)

	disconnect := widgets.IconButton(t.icons,
		widgets.Face{Icon: "plug-connected-x", Color: theme.Destructive, Tooltip: "disconnect"},
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

	// Hovering the mute button reveals the slider; it stays out while the pointer is
	// anywhere on the controls and folds on leaving them (the tile's own leave
	// catches fast exits).
	volHover := gtk.NewEventControllerMotion()
	volHover.ConnectEnter(func(x, y float64) { t.volRevealer.SetRevealChild(true) })
	t.volume.Widget().AddController(volHover)

	barLeave := gtk.NewEventControllerMotion()
	barLeave.ConnectLeave(func() { t.volRevealer.SetRevealChild(false) })
	t.controls.AddController(barLeave)
}

// buildVolumeCard is the vertical slider that slides up over the bar.
func (t *Tile) buildVolumeCard() *gtk.Revealer {
	t.volScale = gtk.NewScaleWithRange(gtk.OrientationVertical, 0, 1, volumeSteps)
	t.volScale.SetInverted(true)
	t.volScale.SetSizeRequest(-1, volumeHeight)
	t.volScale.SetValue(1)
	t.volScale.ConnectValueChanged(func() {
		if t.current != nil {
			t.current.SetVolume(t.volScale.Value())
		}
	})

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

// setMuted mutes the player and marks the tile: the button's face flips and the
// corner chip grows the web tile's volume-off marker.
func (t *Tile) setMuted(muted bool) {
	t.muted = muted
	if t.current != nil {
		t.current.SetMuted(muted)
	}
	t.volume.SetOn(muted)
	t.chipMute.SetVisible(muted)
}
