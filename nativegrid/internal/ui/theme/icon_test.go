package theme

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The color the icons are rasterized in, picked so each channel differs and a
// swapped byte order shows up as a wrong channel rather than a wrong shade.
const testColor = "#204080"

// TestRasterizeVendoredIcons draws every icon the binary carries. The rasterizer
// only reports a refused SVG feature through the parse error rasterize asserts
// on, so an icon vendored with an unsupported construct fails here rather than
// as an empty square in the window.
func TestRasterizeVendoredIcons(t *testing.T) {
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		t.Fatalf("read vendored icons: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the binary carries no vendored icons")
	}

	for _, e := range entries {
		name := e.Name()[:len(e.Name())-len(".svg")]
		t.Run(name, func(t *testing.T) {
			img := rasterize(name, 24, testColor)
			if covered(img) == 0 {
				t.Error("rasterized to a fully transparent image")
			}
		})
	}
}

// TestRasterizeColor proves the requested color reaches the pixels in the byte
// order gdk.MemoryR8G8B8A8Premultiplied reads them in. The check runs on a fully
// opaque pixel, where premultiplication is the identity and the stored bytes are
// the color itself.
func TestRasterizeColor(t *testing.T) {
	img := rasterize("video", 24, testColor)

	found := false
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] != 0xff {
			continue
		}
		found = true
		if got := [3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}; got != [3]uint8{0x20, 0x40, 0x80} {
			t.Fatalf("opaque pixel is R=%#02x G=%#02x B=%#02x, want R=0x20 G=0x40 B=0x80", got[0], got[1], got[2])
		}
	}
	if !found {
		t.Error("no fully opaque pixel, so the stroke never reached full coverage")
	}
}

// TestRasterizeScales proves the size argument reaches the geometry instead of
// the icon being drawn at its 24 box and cropped. A stroke scales with the box,
// so the larger icon covers a larger share of its image.
func TestRasterizeScales(t *testing.T) {
	small := rasterize("video", 16, testColor)
	large := rasterize("video", 64, testColor)

	if small.Bounds().Dx() != 16 || large.Bounds().Dx() != 64 {
		t.Fatalf("images are %dx%d and %dx%d", small.Bounds().Dx(), small.Bounds().Dy(), large.Bounds().Dx(), large.Bounds().Dy())
	}
	if covered(large) <= covered(small) {
		t.Errorf("64px icon covers %d pixels, 16px one covers %d", covered(large), covered(small))
	}
}

// TestTextureMatchesRaster proves the pixels survive the handover to GDK: a
// texture that read the buffer at the wrong stride or in the wrong channel order
// would still be a valid texture, and the difference is only visible once it is
// drawn. Reading it back through GDK's own PNG writer compares what GDK holds
// against what the rasterizer produced.
func TestTextureMatchesRaster(t *testing.T) {
	const size = 24

	want := rasterize("video", size, testColor)
	path := filepath.Join(t.TempDir(), "icon.png")
	if !render("video", size, testColor).SaveToPNG(path) {
		t.Fatal("GDK refused to write the texture as a PNG")
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open written texture: %v", err)
	}
	defer f.Close()

	got, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode written texture: %v", err)
	}
	if got.Bounds() != want.Bounds() {
		t.Fatalf("texture is %v, rasterized image is %v", got.Bounds(), want.Bounds())
	}

	for y := range size {
		for x := range size {
			// Opaque pixels only: PNG carries straight alpha and the raster
			// carries premultiplied, so the two agree exactly at full alpha and
			// differ by rounding everywhere else.
			wr, wg, wb, wa := want.At(x, y).RGBA()
			if wa != 0xffff {
				continue
			}
			if gr, gg, gb, _ := got.At(x, y).RGBA(); gr != wr || gg != wg || gb != wb {
				t.Fatalf("pixel (%d,%d) is %#04x%#04x%#04x in the texture, %#04x%#04x%#04x in the raster", x, y, gr, gg, gb, wr, wg, wb)
			}
		}
	}
}

// covered counts the pixels the rasterizer touched.
func covered(img *image.RGBA) int {
	n := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			n++
		}
	}
	return n
}
