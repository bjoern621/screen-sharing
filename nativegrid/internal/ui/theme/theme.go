// Package theme carries the look the window shares with the web app: the
// stylesheet, the Tabler icon set, and the color pairs both are drawn in
// (docs/design-language.md).
//
// GTK re-renders nothing on a theme flip that has a color baked into a texture,
// which a rasterized SVG has, so an icon registers here and is re-rasterized when
// the flip happens.
package theme

import (
	_ "embed"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
)

//go:embed style.css
var styleCSS string

// Pair is one color in both themes. The values are the web tokens: foreground
// (zinc-950/zinc-50), muted-foreground (zinc-500/zinc-400), destructive
// (red-600/red-400), primary (emerald-700/emerald-800).
type Pair struct {
	Light, Dark string
}

var (
	Foreground  = Pair{Light: "#09090b", Dark: "#fafafa"}
	Muted       = Pair{Light: "#71717a", Dark: "#a1a1aa"}
	Destructive = Pair{Light: "#dc2626", Dark: "#f87171"}
	Primary     = Pair{Light: "#047857", Dark: "#065f46"}
)

// A tile is black in both themes, so an icon drawn on one takes a fixed color
// instead of a pair: white for a marker beside a label, the dark destructive value
// for a failure.
const OnTile = "white"

var DestructiveOnTile = Destructive.Dark

// pick is the pair's value for the theme in force.
func (p Pair) pick(dark bool) string {
	assert.Assert(p.Light != "" && p.Dark != "", "a color pair carries both values")

	if dark {
		return p.Dark
	}
	return p.Light
}

// LoadStyle installs the stylesheet on the default display. It applies to every
// window the process opens.
func LoadStyle() {
	display := gdk.DisplayGetDefault()
	assert.IsNotNil(display, "a display is open before the stylesheet is loaded")

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(styleCSS)
	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
