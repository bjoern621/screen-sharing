// Package widgets holds the controls the window's surfaces share, so a tile
// button and a header button are one shape described twice rather than two.
package widgets

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// iconSize is the glyph size of a control button, the web app's h-4 icon.
const iconSize = 16

// The pages of an IconToggle's icon stack.
const (
	pageOff = "off"
	pageOn  = "on"
)

// Face is one face of an icon control: the glyph, the color pair it is drawn in,
// and the tooltip that describes what pressing it does now.
type Face struct {
	Icon    string
	Color   theme.Pair
	Tooltip string
}

// IconButton is a flat icon button, the shape every control on a tile takes.
func IconButton(icons *theme.Icons, f Face, click func()) *gtk.Button {
	assert.IsNotNil(icons, "a control's icon belongs to a set that can release it")

	b := gtk.NewButton()
	b.AddCSSClass("flat")
	b.SetChild(icons.Image(f.Icon, iconSize, f.Color))
	b.SetTooltipText(f.Tooltip)
	if click != nil {
		b.ConnectClicked(click)
	}
	return b
}

// IconToggle is a flat icon button with two faces, swapped by SetOn: the mute
// button's speaker, the stats button's tint, the spotlight button's arrows. Both
// faces are rasterized up front and swapped in a stack, so a flip costs no
// rendering.
type IconToggle struct {
	button *gtk.Button
	stack  *gtk.Stack
	faces  map[bool]Face
}

func NewIconToggle(icons *theme.Icons, off, on Face, click func()) *IconToggle {
	assert.IsNotNil(icons, "a control's icons belong to a set that can release them")

	t := &IconToggle{
		stack:  gtk.NewStack(),
		faces:  map[bool]Face{false: off, true: on},
		button: gtk.NewButton(),
	}
	t.stack.AddNamed(icons.Image(off.Icon, iconSize, off.Color), pageOff)
	t.stack.AddNamed(icons.Image(on.Icon, iconSize, on.Color), pageOn)

	t.button.AddCSSClass("flat")
	t.button.SetChild(t.stack)
	if click != nil {
		t.button.ConnectClicked(click)
	}
	t.SetOn(false)
	return t
}

// Widget is the toggle's button, for a container to hold and a controller to
// attach to.
func (t *IconToggle) Widget() *gtk.Button { return t.button }

// SetOn shows the face of the given state, tooltip included.
func (t *IconToggle) SetOn(on bool) {
	page := pageOff
	if on {
		page = pageOn
	}
	t.stack.SetVisibleChildName(page)
	t.button.SetTooltipText(t.faces[on].Tooltip)
}
