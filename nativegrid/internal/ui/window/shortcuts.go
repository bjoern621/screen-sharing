package window

import (
	"fmt"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// spotDigits is how far the number row reaches into the grid.
// Zero is left out: keeping the row in order would make it the tenth tile, which
// no key on it says.
const spotDigits = 9

// accelSidebar is named because the header toggle shares its action and spells
// the key out in its tooltip.
// Every other accelerator lives in the table below and nowhere else.
const accelSidebar = "<Control>b"

// binding is one of the window's keyboard controls: the accelerator GTK parses,
// the sentence a widget bound to the same action carries as its tooltip, and
// what pressing it does (actions.go).
//
// run reports whether the binding acted. One that finds nothing to do leaves the
// key alone rather than swallowing it, so Escape outside fullscreen and outside
// the spotlight still reaches whatever else would have taken it.
type binding struct {
	accel string
	what  string
	run   func(c *chrome) bool
}

// bindings is the window's whole keyboard table, one entry per key.
func bindings() []binding {
	table := []binding{
		{
			accel: "Escape",
			what:  "Leave fullscreen, or leave the spotlight when the window is not in one.",
			run:   (*chrome).leave,
		},
		{
			accel: "F11",
			what:  "Give the window the whole screen. Escape or F11 gives it back.",
			run:   (*chrome).toggleFullscreen,
		},
		{
			accel: accelSidebar,
			what:  "Show or hide the stream sidebar, which gives the streams the window's edges.",
			run:   (*chrome).toggleSidebar,
		},
	}
	// The digits differ only in the tile they reach, so their row is described
	// once and filled in.
	for n := 1; n <= spotDigits; n++ {
		table = append(table, binding{
			accel: strconv.Itoa(n),
			what:  fmt.Sprintf("Spotlight tile %d of the grid, or leave the spotlight when that tile already holds it.", n),
			run:   func(c *chrome) bool { return c.spotlightNth(n) },
		})
	}
	for _, b := range table {
		assert.IsNotNil(b.run, "a binding acts on the window", b.accel)
	}
	return table
}

// bindKeys installs the table on the window.
//
// The controller is local to the window, so a binding fires only while this
// window has the focus, and it stays in the bubble phase, where the focused
// widget has already had the key: a digit typed into a watch-leg field is that
// field's, not a spotlight.
func (c *chrome) bindKeys() {
	assert.IsNotNil(c.win, "a binding belongs to a window")

	keys := gtk.NewShortcutController()
	keys.SetScope(gtk.ShortcutScopeLocal)
	keys.SetPropagationPhase(gtk.PhaseBubble)

	table := bindings()
	for _, b := range table {
		key, mods, ok := gtk.AcceleratorParse(b.accel)
		assert.Assert(ok, "a binding names an accelerator GTK can parse", b.accel)

		action := gtk.NewCallbackAction(func(gtk.Widgetter, *glib.Variant) bool { return b.run(c) })
		keys.AddShortcut(gtk.NewShortcut(gtk.NewKeyvalTrigger(key, mods), action))
	}
	c.win.AddController(keys)
	logger.Debugf("%d window shortcuts bound", len(table))
}

// tip is the tooltip of a widget that shares its action with an accelerator:
// what the binding does, and the key that does it, spelled the way the platform
// writes it. The table stays the one place a binding is described.
func tip(accel string) string {
	for _, b := range bindings() {
		if b.accel != accel {
			continue
		}
		key, mods, ok := gtk.AcceleratorParse(accel)
		assert.Assert(ok, "a binding names an accelerator GTK can parse", accel)

		return b.what + " (" + gtk.AcceleratorGetLabel(key, mods) + ")"
	}
	assert.Never("a tooltip names an accelerator the table binds", accel)
	return ""
}
