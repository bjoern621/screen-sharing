package theme

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// The look's inputs that GTK keeps in its settings rather than in the
// environment. The ones it reads while it initializes are pinned before that,
// in internal/gtkenv.
//
// font is the family and size every widget that does not name one draws in,
// which in this window is all of them: style.css states colors and spacing and
// no family at all. It is pinned because the platform's answer is the desktop's
// preference on Linux and Segoe UI on Windows, and a different family is
// different metrics: text a fraction wider lays out the sidebar rows, the
// header and the stats card differently, so nothing else about the look can
// match while this does not. Cantarell is GNOME's own, which is what the Linux
// side already draws and what the Windows bundle carries a copy of.
//
// iconTheme is where libadwaita's own widgets take their icons from: the window
// buttons in the header, the split view's arrows, the dropdown chevrons. This
// window's own icons are the vendored Tabler SVGs (icon.go) and come from no
// theme at all, so what this pins is the share of the chrome the toolkit draws
// rather than the share the design language does.
const (
	font      = "Cantarell 11"
	iconTheme = "Adwaita"
)

// How a glyph outline becomes pixels. Slight hinting under grayscale
// antialiasing is what GNOME sets, so pinning these is what keeps the Windows
// side off its own defaults once gtkenv has put both on the same font map and
// rasterizer.
//
// The subpixel order is none rather than a layout, because subpixel
// antialiasing rasterizes a glyph for the panel it is drawn on, and the two
// machines being compared are not the same panel. Grayscale is the one answer
// that means the same thing on both.
const (
	antialias = 1
	hinting   = 1
	hintStyle = "hintslight"
	subpixel  = "none"
)

// PinSettings states the look GTK would otherwise take from the platform.
//
// It writes the default settings object, the one every window in the process
// shares, so a popover or a tooltip that is a toplevel of its own is drawn in
// the pinned font too. It runs before the stylesheet: style.css sizes some of
// what it draws in em, and an em is the font this settles.
func PinSettings() {
	settings := gtk.SettingsGetDefault()
	assert.IsNotNil(settings, "a display is open before its settings are pinned")

	settings.SetObjectProperty("gtk-font-name", font)
	settings.SetObjectProperty("gtk-icon-theme-name", iconTheme)
	settings.SetObjectProperty("gtk-xft-antialias", antialias)
	settings.SetObjectProperty("gtk-xft-hinting", hinting)
	settings.SetObjectProperty("gtk-xft-hintstyle", hintStyle)
	settings.SetObjectProperty("gtk-xft-rgba", subpixel)

	logger.Infof("look pinned: %s, %s icons, %s", font, iconTheme, hintStyle)
}
