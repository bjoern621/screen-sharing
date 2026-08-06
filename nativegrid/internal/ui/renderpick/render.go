package renderpick

import (
	"slices"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// rowPadding is the space a row keeps around its label, so the list reads as a menu
// rather than as lines of text.
const rowPadding = 6

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

// Draw is the picker's render function: it puts one Choice on the list, rows and
// selection alike.
//
// The rows are replaced only where the table moved, so a second pass on an unchanged
// Choice writes the same selection into the same rows and rebuilds nothing.
func (p *Picker) Draw(c Choice) {
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

	p.list.SelectRow(p.rows[i])
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

// refill replaces the list's rows with the ones a Choice derives.
//
// The table and the widgets are written together, because a row is read back by its
// position in both (Draw, commit): a table naming other rows than the list holds
// would pick the wrong chain.
func (p *Picker) refill(entries []entry) {
	for _, row := range p.rows {
		p.list.Remove(row)
	}
	p.entries = entries
	p.rows = make([]*gtk.ListBoxRow, 0, len(entries))

	for _, e := range entries {
		row := buildRow(e)
		p.list.Append(row)
		p.rows = append(p.rows, row)
	}
	assert.Assert(len(p.rows) == len(p.entries), "a row per entry", len(p.rows), len(p.entries))
}

// buildRow is one row of the list: the chain's label, and the tooltip that says what
// the chain does.
//
// A chain this machine cannot run is shown and not offered. The row refuses to be
// taken, and the label rather than the row carries the greying and the tooltip: GTK
// draws no tooltip for an insensitive widget, so a row greyed as a whole would be one
// whose reason for being inert cannot be read.
func buildRow(e entry) *gtk.ListBoxRow {
	label := gtk.NewLabel(e.label)
	label.SetXAlign(0)
	label.SetMarginTop(rowPadding)
	label.SetMarginBottom(rowPadding)
	label.SetMarginStart(rowPadding)
	label.SetMarginEnd(rowPadding)
	label.SetTooltipText(e.tip)
	label.SetSensitive(e.available)

	row := gtk.NewListBoxRow()
	row.SetChild(label)
	row.SetActivatable(e.available)
	row.SetSelectable(e.available)
	return row
}
