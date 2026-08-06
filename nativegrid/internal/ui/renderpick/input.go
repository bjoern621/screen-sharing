package renderpick

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"
)

// wireInput connects the one signal the control has. Only New calls it, so the
// list carries one handler for the life of the picker.
//
// The commit lands on the next pass of the loop rather than inside the emission.
// A pick is answered by the model with a change the mounting surface draws back
// through Draw, which rebuilds these rows where the table moved, the activated row
// among them. Doing that from inside the row's own activation is destroying the
// widget whose handler is still running.
//
// It is the same rule the tile area keeps for a reorder, which lands on the next
// pass rather than reparenting widgets from inside the drag callback that asked for
// it (grid.View.relayout).
//
// The row is read here and carried by position, not read again later: the pass the
// commit runs in is one the model can have moved the rows in, and the chain the
// viewer clicked is the one to send.
func (p *Picker) wireInput() {
	p.list.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		assert.IsNotNil(row, "an activated row exists")

		i := row.Index()
		p.dispatch(func() { p.commit(i) })
	})
}

// commit carries the row the viewer activated to the model, and nothing else: the
// model answers with the change the mounting surface redraws from, so what the
// control shows is always the chain in force rather than the one that was asked for.
//
// A row that cannot be taken is drawn back rather than sent. GTK activates neither by
// click nor by keyboard a row that is neither activatable nor selectable, so an
// activation landing on one came from somewhere else, and abandoning it is drawing the
// choice that holds.
//
// The same answers a position the table no longer holds. The commit runs a pass after
// the activation, and a roster push or a backend offering other chains in between
// leaves the row that was clicked standing for nothing.
func (p *Picker) commit(i int) {
	if i < 0 || i >= len(p.entries) || !p.entries[i].available {
		p.Draw(p.drawn)
		return
	}
	p.pick(p.entries[i].name)
}
