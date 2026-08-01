// Package renderpick is the control that picks a render chain: a dropdown over the
// chains the decode backend offers, each row carrying what that chain does and what
// it says about the colour it produces.
//
// It is mounted twice. The sidebar's header holds the window's default, a tile's
// control bar the override of one stream, and the second form leads with an entry
// that hands the choice back to the default. Both are the same control with the same
// commit model: the pick is the change, applied at once, because a chain is fixed
// when the pipeline is parsed and the stream restarts on it either way.
//
// A chain this machine cannot run is greyed rather than left out, with what is
// missing in its tooltip, so an offer that cannot be taken explains itself
// (docs/tooltips.md, "Availability notes are additive"). GTK carries a tooltip on
// any widget, so the row's own label holds it and there is no wrapper to build.
//
// Two directions, one function each. Draw (render.go) is the only path from the
// model into these widgets, and the dropdown's own signal (input.go) the only one
// out of them, which puts a change to the model rather than to a widget.
package renderpick

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
)

// Picker is one render-chain dropdown.
type Picker struct {
	drop  *gtk.DropDown
	model *gtk.StringList
	// entries are the rows the model holds, in its order: the table the list factory
	// binds a recycled row out of, and the one a selection is read back through. Draw
	// refills it on every pass.
	entries []entry
	// drawn is the last state drawn, and what a pick the control refuses returns to.
	drawn Choice
	// withDefault is the leading entry that hands the choice back to the window's
	// default, which a stream's picker offers and the picker of the default itself
	// does not.
	withDefault bool
	// syncing suppresses the dropdown's own signal while a draw writes the model's
	// chain into it, so a redraw cannot read as a viewer picking one.
	syncing bool
	pick    func(name string)
}

// New builds a picker. withDefault adds the leading entry that follows the window's
// default; pick carries a choice to the model, which answers with the change the
// mounting surface draws back.
func New(withDefault bool, pick func(name string)) *Picker {
	assert.IsNotNil(pick, "a picked render chain goes somewhere")

	p := &Picker{
		model:       gtk.NewStringList(nil),
		withDefault: withDefault,
		pick:        pick,
	}
	// The rows leave here empty. The model is filled by the first Draw, which is also
	// what the button below shows: a picker nobody has drawn yet stands for no chain.
	p.drop = gtk.NewDropDown(p.model, nil)
	p.drop.SetListFactory(p.rowFactory())
	p.wireInput()
	return p
}

// Widget is the dropdown, for a popover to hold.
func (p *Picker) Widget() gtk.Widgetter { return p.drop }
