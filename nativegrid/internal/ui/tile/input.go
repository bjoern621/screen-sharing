package tile

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
)

// doubleClick is the press count that spotlights a tile.
const doubleClick = 2

// wireInput attaches the tile's controllers and gestures.
// Every handler writes a state field and calls the render pass, and none of them sets a
// widget property, so what the tile looks like stays decided in one place.
//
// It runs once, after the root and the controls exist: two of the controllers sit on
// the control bar.
func (t *Tile) wireInput() {
	assert.IsNotNil(t.root, "input is wired to a built tile", t.stream.Name)
	assert.IsNotNil(t.controls, "input is wired to a built control bar", t.stream.Name)
	assert.IsNotNil(t.volume, "input is wired to a built mute button", t.stream.Name)
	assert.IsNotNil(t.volScale, "input is wired to a built volume slider", t.stream.Name)

	// The controls fade in while the pointer is on the tile, the web tile's group-hover.
	// Leaving also folds the volume slider away, which catches a pointer that left the
	// bar too fast for the bar's own leave.
	hover := gtk.NewEventControllerMotion()
	hover.ConnectEnter(func(x, y float64) {
		t.hovered = true
		t.apply()
	})
	hover.ConnectLeave(func() {
		t.hovered, t.volOpen = false, false
		t.apply()
	})
	t.root.AddController(hover)

	// A double click is the second way to the spotlight, beside the control bar's button.
	// The gesture claims no event sequence, so a press that turns into a drag stays the
	// drag source's, which is attached to this same widget.
	spot := gtk.NewGestureClick()
	spot.SetButton(gdk.BUTTON_PRIMARY)
	spot.ConnectPressed(func(nPress int, x, y float64) {
		if nPress == doubleClick && !t.onOverlayControl(x, y) {
			t.hooks.ToggleSpot()
		}
	})
	t.root.AddController(spot)

	// Hovering the mute button slides the slider out.
	// It stays out while the pointer is anywhere on the controls, and folds on leaving
	// them.
	volHover := gtk.NewEventControllerMotion()
	volHover.ConnectEnter(func(x, y float64) {
		t.volOpen = true
		t.apply()
	})
	t.volume.Widget().AddController(volHover)

	barLeave := gtk.NewEventControllerMotion()
	barLeave.ConnectLeave(func() {
		t.volOpen = false
		t.apply()
	})
	t.controls.AddController(barLeave)

	// The slider is where a volume is chosen, so what it holds is read back into the tile
	// and travels from there to whichever player is attached.
	// The pass writes the value back to the slider that already holds it, which GTK drops
	// as an unchanged adjustment, so this does not feed itself.
	t.volScale.ConnectValueChanged(func() {
		t.level = t.volScale.Value()
		t.apply()
	})
}

// onOverlayControl reports whether a press landed on one of the pages the tile puts over
// the picture to be operated: the control bar, the open stats card, the failure page with
// its retry.
// Each answers a click of its own, and a second one on it is not a request to spotlight.
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
