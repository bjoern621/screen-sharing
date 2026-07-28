package theme

import (
	"bytes"
	"embed"
	"image"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// The icons are Tabler outline SVGs vendored from the web frontend's
// @tabler/icons package, so native and web surfaces show the same glyphs
// (docs/design-language.md, "Icons").
//
//go:embed icons/*.svg
var iconFS embed.FS

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

// render rasterizes a vendored Tabler SVG at one color and size, as the texture a
// GtkImage draws.
func render(name string, size int, color string) *gdk.MemoryTexture {
	img := rasterize(name, size, color)

	// image.RGBA is alpha-premultiplied in R, G, B, A byte order, which is the
	// layout the format names, so the pixels go over untouched. NewBytes copies
	// them, so the texture outlives the Go image.
	return gdk.NewMemoryTexture(size, size, gdk.MemoryR8G8B8A8Premultiplied,
		glib.NewBytes(img.Pix), uint(img.Stride))
}

// rasterize draws one vendored Tabler SVG into an RGBA image.
//
// The rasterizer is a Go one rather than GTK's: gdk-pixbuf carries no SVG loader
// of its own and reaches librsvg through a cache named in GDK_PIXBUF_MODULE_FILE,
// which rests the icons on a variable inherited from whatever spawns the grid and
// on a loader Windows has no equivalent of. In-process, they are a property of
// the binary.
//
// A missing or unparsable icon is a build-time mistake: the icons ship inside the
// binary.
func rasterize(name string, size int, color string) *image.RGBA {
	assert.Assert(size > 0, "an icon is rasterized at a positive size", name, size)

	raw, err := iconFS.ReadFile("icons/" + name + ".svg")
	assert.IsNil(err, "vendored icon", name)

	// The Tabler sources take their color from currentColor, which resolves to
	// black on its own.
	svg, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader(raw), color, oksvg.StrictErrorMode)
	assert.IsNil(err, "parsed icon", name)

	// The sources are drawn in the 24 box their viewBox declares. Targeting the
	// pixel size scales the geometry, stroke width included, so an icon is drawn
	// at its size rather than scaled after the fact.
	svg.SetTarget(0, 0, float64(size), float64(size))

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	svg.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)
	return img
}
