package renderpick

import (
	"slices"
	"strings"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// Choice is the state a picker draws: what the backend offers, the chain in force,
// and the default the leading entry names.
type Choice struct {
	// Chains is what the backend offers, in its own order. A picker with nothing
	// offered draws no chain row, and the surface that mounts it hides its control.
	Chains []player.Chain
	// Chosen is the chain the picker shows as picked, and "" for a stream's picker
	// that follows the window's default.
	Chosen string
	// Default is the chain the leading entry hands the choice back to. The entry names
	// it, so following the default says which chain that is.
	Default string
}

// entry is one row: the chain it picks, how it reads, and whether it can be taken.
// The leading entry names no chain, which is what "" stands for everywhere a chain
// name travels.
type entry struct {
	name      string
	label     string
	tip       string
	available bool
}

// Draw is the picker's render function: it puts one Choice on the dropdown, rows and
// selection alike.
//
// The rows are replaced only where the table moved, so a second pass on an unchanged
// Choice writes the same selection into the same rows, rebuilds nothing and leaks no
// handler.
func (p *Picker) Draw(c Choice) {
	// A splice moves the selection, and a written selection is not a viewer picking
	// one, so the whole pass is held out of the dropdown's own signal.
	p.syncing = true
	defer func() { p.syncing = false }()

	p.drawn = c
	entries := entriesOf(c, p.withDefault)
	if !slices.Equal(entries, p.entries) {
		p.refill(entries)
	}
	if len(p.entries) == 0 {
		// A backend offering nothing, on a picker with no default entry to lead with.
		// There is no row to select and nothing on screen to say it with.
		return
	}

	i := slices.IndexFunc(p.entries, func(e entry) bool { return e.name == c.Chosen })
	assert.Assert(i >= 0, "a picker offers the chain it is drawn on", c.Chosen)

	p.drop.SetSelected(uint(i))
}

// entriesOf is the rows one Choice draws: the leading default entry where the form
// carries one, then a row per chain the backend offers, in the backend's order.
func entriesOf(c Choice, withDefault bool) []entry {
	out := make([]entry, 0, len(c.Chains)+1)
	if withDefault {
		out = append(out, entry{label: defaultLabel(c.Default), tip: defaultTip(c.Default), available: true})
	}
	for _, ch := range c.Chains {
		out = append(out, entry{name: ch.Name, label: ch.Label, tip: rowTip(ch), available: ch.Available})
	}
	return out
}

// defaultLabel names the window's default on the entry that follows it, so picking it
// says which chain that is and not only that it is somebody else's decision. A
// default this side cannot name is one the backend decides on its own.
func defaultLabel(def string) string {
	if def == "" {
		return "Default"
	}
	return "Default (" + def + ")"
}

// defaultTip says what following the default means and where it is set, which is the
// one thing about the entry the chains' own tips do not cover.
func defaultTip(def string) string {
	tip := "Render this stream through the chain the window defaults to, which the sidebar's header sets, and move with it when it moves."
	if def == "" {
		return tip + " The window has chosen none, so the decode backend's own default stands."
	}
	return tip + " It is on " + def + " now."
}

// rowTip is what a chain's row says: the chain's own explanation, and under it the
// reason a chain this machine cannot run is inert. The two are joined by a blank line
// and the empty half dropped, the way the settings form appends an availability note
// (docs/tooltips.md).
func rowTip(c player.Chain) string {
	notes := []string{c.Tip}
	if !c.Available {
		notes = append(notes, "Unavailable: "+c.Reason)
	}
	return strings.Join(slices.DeleteFunc(notes, func(s string) bool { return s == "" }), "\n\n")
}

// refill replaces the dropdown's rows with the ones a Choice derives.
//
// The table is written before the model, because the splice recreates every row and
// the factory binds each of them out of the table while it runs.
func (p *Picker) refill(entries []entry) {
	removals := uint(len(p.entries))
	p.entries = entries

	labels := make([]string, 0, len(entries))
	for _, e := range entries {
		labels = append(labels, e.label)
	}
	assert.Assert(len(labels) == len(p.entries), "a label per row", len(labels), len(p.entries))

	p.model.Splice(0, removals, labels)
}

// rowFactory builds the popup's rows. The list view creates one widget per visible
// row and recycles it, so bind is the list refilling a row that already exists out of
// the table Draw wrote: it sets everything a row shows, the branch that greys it
// included.
//
// It is the popup's factory alone. The dropdown's button keeps the plain one GTK
// gives it, which draws the selected row's label and nothing else.
func (p *Picker) rowFactory() *gtk.ListItemFactory {
	f := gtk.NewSignalListItemFactory()
	f.ConnectSetup(func(o *coreglib.Object) {
		label := gtk.NewLabel("")
		label.SetXAlign(0)
		listItem(o).SetChild(label)
	})
	f.ConnectBind(func(o *coreglib.Object) {
		item := listItem(o)
		pos := int(item.Position())
		assert.Assert(pos >= 0 && pos < len(p.entries), "a bound row is one the table holds", pos, len(p.entries))

		e := p.entries[pos]
		label, ok := item.Child().(*gtk.Label)
		assert.Assert(ok, "a render-chain row holds the label its setup gave it", e.label)

		label.SetText(e.label)
		label.SetTooltipText(e.tip)
		// A chain this machine cannot run is shown and not offered: the row explains
		// itself in its tooltip and refuses to be taken.
		label.SetSensitive(e.available)
		item.SetSelectable(e.available)
		item.SetActivatable(e.available)
	})
	return &f.ListItemFactory
}

// listItem is the row a factory signal is about.
func listItem(o *coreglib.Object) *gtk.ListItem {
	assert.IsNotNil(o, "a list factory signal carries an object")

	item, ok := o.Cast().(*gtk.ListItem)
	assert.Assert(ok, "a list factory signal is about a list item")

	return item
}
