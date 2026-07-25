package theme

import (
	"embed"
	"strconv"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// The icons are Tabler outline SVGs vendored from the web frontend's
// @tabler/icons package, so native and web surfaces show the same glyphs
// (docs/design-language.md, "Icons").
//
//go:embed icons/*.svg
var iconFS embed.FS

// The Tabler source SVGs are drawn in a 24 box and take their color from
// currentColor, which librsvg resolves to black, so both are substituted before
// rasterizing.
const (
	iconBox         = "24"
	iconColorSource = "currentColor"
)

// icon is one themed icon: an image whose texture is re-rasterized when the theme
// flips.
type icon struct {
	img   *gtk.Image
	name  string
	size  int
	color Pair
}

// registry holds every themed icon in the process, so a theme flip reaches all of
// them. Icons on window chrome stay for the process lifetime; the icons of a tile
// leave with it (Icons.Release).
var registry = map[*icon]struct{}{}

// dark is the theme the last ApplyDark saw, so an icon created after the window
// (a tile's) renders for the theme in force immediately.
var dark bool

// Icons is the themed icons of one widget tree. A tile takes its icons from its
// own set and releases them when it leaves the watch set, which is what keeps the
// registry from growing with every watch.
type Icons struct {
	owned []*icon
}

// Image adds a themed icon to the set and returns its image.
func (s *Icons) Image(name string, size int, color Pair) *gtk.Image {
	ic := &icon{img: gtk.NewImage(), name: name, size: size, color: color}
	ic.img.SetPixelSize(size)
	ic.apply(dark)

	registry[ic] = struct{}{}
	s.owned = append(s.owned, ic)
	return ic.img
}

// Release drops the set's icons from the registry. Their images keep the color
// they were last rendered in, which is what a widget on its way out shows.
func (s *Icons) Release() {
	for _, ic := range s.owned {
		delete(registry, ic)
	}
	logger.Tracef("released %d themed icons", len(s.owned))
	s.owned = nil
}

// FixedImage is a Tabler icon in one color, for a surface that ignores the theme:
// a tile is black in both.
func FixedImage(name string, size int, color string) *gtk.Image {
	img := gtk.NewImage()
	img.SetPixelSize(size)
	img.SetFromPaintable(render(name, size, color))
	return img
}

// ApplyDark re-renders every registered icon for the theme in force.
func ApplyDark(isDark bool) {
	dark = isDark
	for ic := range registry {
		ic.apply(isDark)
	}
	logger.Debugf("theme applied to %d icons (dark=%t)", len(registry), isDark)
}

func (i *icon) apply(dark bool) {
	i.img.SetFromPaintable(render(i.name, i.size, i.color.pick(dark)))
}

// render rasterizes a vendored Tabler SVG at one color and size. A missing icon or
// an SVG librsvg rejects is a build-time mistake, not a runtime condition: the
// icons ship inside the binary.
func render(name string, size int, color string) *gdk.Texture {
	assert.Assert(size > 0, "an icon is rasterized at a positive size", name, size)

	raw, err := iconFS.ReadFile("icons/" + name + ".svg")
	assert.IsNil(err, "vendored icon", name)

	box := strconv.Itoa(size)
	svg := strings.ReplaceAll(string(raw), iconColorSource, color)
	svg = strings.Replace(svg, `width="`+iconBox+`"`, `width="`+box+`"`, 1)
	svg = strings.Replace(svg, `height="`+iconBox+`"`, `height="`+box+`"`, 1)

	tex, err := gdk.NewTextureFromBytes(glib.NewBytes([]byte(svg)))
	assert.IsNil(err, "rasterized icon", name)
	return tex
}
