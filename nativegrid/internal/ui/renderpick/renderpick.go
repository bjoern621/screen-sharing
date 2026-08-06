// Package renderpick is the control that picks a render chain: a list of the chains
// the decode backend offers, each row carrying what that chain does and what it says
// about the colour it produces.
//
// It is mounted twice. The sidebar's header holds the window's default, a tile's
// control bar the override of one stream, and the second form leads with an entry
// that hands the choice back to the default. Both are the same control with the same
// commit model: the pick is the change, applied at once, because a chain is fixed
// when the pipeline is parsed and the stream restarts on it either way.
//
// It is a list and not a GtkDropDown, which is the control the choice would otherwise
// be drawn with. Both surfaces mount this inside a popover, and a dropdown opens a
// popup of its own inside that popover. On Windows GTK 4.22 does not give the grab
// back to the outer popover when such an inner popup closes, and the popover then
// stops closing on a click outside it and has to be dismissed with Escape. It
// reproduces on stock GTK with a GtkMenuButton, a GtkPopover and a GtkDropDown and
// nothing else, so it is not this window's to fix; what this window can do is not nest
// the popups. A list has no popup, and it costs one click instead of two.
//
// A chain this machine cannot run is greyed rather than left out, with what is
// missing in its tooltip, so an offer that cannot be taken explains itself
// (docs/tooltips.md, "Availability notes are additive"). GTK carries a tooltip on
// any widget, so the row's own label holds it and there is no wrapper to build.
//
// Two directions, one function each. Draw (render.go) is the only path from the
// model into these widgets, and the list's own activation (input.go) the only one
// out of them, which puts a change to the model rather than to a widget.
package renderpick

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
)

// Picker is one render-chain list.
type Picker struct {
	list *gtk.ListBox
	// rows are the list's rows, in its order, beside the entries they were built
	// from: the table a selection is read back through, and the widgets Draw marks
	// the chain in force on. Draw replaces both together.
	rows    []*gtk.ListBoxRow
	entries []entry
	// drawn is the last state drawn, and what a pick the control refuses returns to.
	drawn Choice
	// withDefault is the leading entry that hands the choice back to the window's
	// default, which a stream's picker offers and the picker of the default itself
	// does not.
	withDefault bool
	// dispatch carries a pick to the next pass of the UI loop, which is what keeps the
	// model change and the redraw answering it out of the row's own activation: that
	// redraw replaces the rows, the activated one among them (input.go).
	dispatch idle.Dispatch
	pick     func(name string)
}

// New builds a picker. withDefault adds the leading entry that follows the window's
// default; pick carries a choice to the model, which answers with the change the
// mounting surface draws back, on the loop pass after the one the pick was made in.
func New(withDefault bool, dispatch idle.Dispatch, pick func(name string)) *Picker {
	assert.IsNotNil(dispatch, "a picked render chain reaches the model on a pass of the UI loop")
	assert.IsNotNil(pick, "a picked render chain goes somewhere")

	p := &Picker{
		list:        gtk.NewListBox(),
		withDefault: withDefault,
		dispatch:    dispatch,
		pick:        pick,
	}
	// The rows leave here empty. The list is filled by the first Draw: a picker nobody
	// has drawn yet stands for no chain.
	//
	// The selection is what marks the chain in force, so the list carries one and the
	// rows of the chains this machine cannot run are left out of it (Draw). Only
	// activation is listened to; a selection is written by Draw and means nothing on its
	// own.
	p.list.SetSelectionMode(gtk.SelectionSingle)
	p.list.AddCSSClass("render-chains")
	p.wireInput()
	return p
}

// Widget is the list, for a popover to hold.
func (p *Picker) Widget() gtk.Widgetter { return p.list }
