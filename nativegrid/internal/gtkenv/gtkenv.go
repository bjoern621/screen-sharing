// Package gtkenv pins the environment GTK takes its look from, before the
// library is initialized and reads it.
//
// The same binary draws the same widgets on every platform, because GTK renders
// each of them itself rather than calling the platform's. What differs between
// two machines running it is the input GTK resolves that drawing against: the
// font map Pango looks a family up in, and the renderer GSK rasterizes with.
// Left to the platform each answers differently, and the window differs with
// them, which is what makes a Windows run of this binary look unlike a Linux
// one even though neither draws a native widget.
//
// The pinning belongs here rather than in the window because GTK reads these
// once, while it initializes, so a value written after that is a value it has
// already used. The look's remaining inputs are settings rather than variables
// and are pinned where the settings are (internal/ui/theme).
//
// A value the environment already carries is kept. What is pinned is what an
// unconfigured run gets, not an override of a choice someone made on purpose:
// a machine whose driver cannot run the pinned renderer still has the door out.
package gtkenv

import (
	"github.com/diamondburned/gotk4/pkg/glib/v2"

	"bjoernblessin.de/go-utils/util/logger"
)

// The variables the look is read out of. Only the renderer is pinned off
// Windows; the other two describe a font stack that platform already resolves
// the way this one wants it (gtkenv_windows.go).
const (
	rendererVar     = "GSK_RENDERER"
	pangoBackendVar = "PANGOCAIRO_BACKEND"
	fontConfigVar   = "FONTCONFIG_FILE"
)

// renderer is the GSK renderer both platforms rasterize through.
//
// Told to choose, GTK takes the best backend the machine offers and prefers
// Vulkan where a driver exposes it, so one machine antialiases an edge through
// Vulkan and the next through GL. Which of them is faster is not the question
// here: the two produce different pixels, and the window is supposed to be the
// same one. GL is the floor every machine that runs the grid already meets,
// since the shipped render chain uploads its frames through it.
//
// "gl" is the current renderer under its current name: GTK 4.18 dropped the
// original GL renderer and gave the name to the one that had been "ngl".
const renderer = "gl"

// setIfUnset pins one variable and reports what it did, keeping whatever the
// environment already says.
//
// The write goes through GLib rather than os.Setenv because every reader of
// these is a C library. Go sets a variable by writing the Win32 environment
// block, which the C runtime never consults: it copies the environment once as
// it starts and answers getenv out of that copy, and Go's cgo does not sync the
// two on this platform. A variable pinned with os.Setenv is therefore one Pango
// cannot see, which is a pin that silently does nothing on the only platform it
// exists for. g_setenv writes both halves, and GTK, Pango and GLib here are all
// linked against the one C runtime whose copy it writes.
func setIfUnset(name, value string) {
	if set := glib.Getenv(name); set != "" {
		logger.Debugf("%s is already %q, leaving it", name, set)
		return
	}
	if !glib.Setenv(name, value, false) {
		logger.Warnf("cannot set %s to %q", name, value)
		return
	}
	logger.Debugf("%s pinned to %q", name, value)
}
