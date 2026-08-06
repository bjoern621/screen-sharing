package gtkenv

import (
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/logger"
)

// pangoBackend is the font map Pango resolves families through. Windows
// defaults to the win32 one; this is the name of the fontconfig one, which is
// what Linux already uses.
const pangoBackend = "fc"

// fontsConf is the fontconfig configuration a Windows bundle carries, named
// relative to the binary. It declares the bundled font directory and the
// rasterization settings, so a family this window asks for resolves to a file
// that shipped beside it rather than to whatever the machine happens to hold
// (scripts/bundle-windows.sh, docs/packaging.md).
var fontsConf = filepath.Join("share", "fonts", "fonts.conf")

// Pin states the renderer, the font map, and where the fonts behind it come
// from.
//
// The font stack is the largest single difference between a Windows window and
// a Linux one. Pango on Windows builds a PangoCairoWin32FontMap: it resolves
// families through GDI and rasterizes their glyphs with it, while Linux
// resolves through fontconfig and rasterizes with FreeType. The same string,
// shaped by the same HarfBuzz, then comes out as different outlines filled by a
// different rasterizer, and because no widget here declares a family, the one
// each platform calls its default decides the metrics that every widget sized
// off a text height is laid out from.
//
// The two variables are one mechanism and neither half works alone: the backend
// switch on its own reaches only the fonts Windows ships, since the stock
// fontconfig configuration scans the system font directory and nothing else,
// and the configuration on its own is read by a font map GTK is not using.
func Pin() {
	setIfUnset(rendererVar, renderer)
	setIfUnset(pangoBackendVar, pangoBackend)

	conf, ok := bundledFontsConf()
	if !ok {
		// A run out of the MSYS2 shell rather than out of a bundle. The window
		// still draws, on whatever fontconfig substitutes for the pinned family,
		// and saying so is what tells the two runs apart when their text does
		// not match.
		logger.Infof("no bundled %s: fonts come from this machine, so text may differ from a bundled run", fontsConf)
		return
	}
	setIfUnset(fontConfigVar, conf)
}

// bundledFontsConf is the configuration shipped beside the binary, and whether
// there is one.
func bundledFontsConf() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		logger.Warnf("cannot resolve the running binary, leaving %s alone: %v", fontConfigVar, err)
		return "", false
	}

	conf := filepath.Join(filepath.Dir(exe), fontsConf)
	if _, err := os.Stat(conf); err != nil {
		logger.Debugf("no font configuration at %s: %v", conf, err)
		return "", false
	}
	return conf, true
}
