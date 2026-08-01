package renderpick

// wireInput connects the one signal the control has. Only New calls it, so the
// dropdown carries one handler for the life of the picker.
func (p *Picker) wireInput() {
	p.drop.NotifyProperty("selected", func() {
		if p.syncing {
			return
		}
		p.commit()
	})
}

// commit carries the row the dropdown shows to the model, and nothing else: the
// model answers with the change the mounting surface redraws from, so what the
// control shows is always the chain in force rather than the one that was asked for.
//
// A row that cannot be taken is drawn back rather than sent. GTK refuses the click
// and the keyboard on a row whose list item is neither selectable nor activatable, so
// a selection landing on one came from somewhere else, and abandoning it is drawing
// the choice that holds.
func (p *Picker) commit() {
	e, ok := p.selected()
	if !ok || !e.available {
		p.Draw(p.drawn)
		return
	}
	p.pick(e.name)
}

// selected is the row the dropdown shows, and false while it shows none: an emptied
// model leaves the selection at no position at all.
func (p *Picker) selected() (entry, bool) {
	i := int(p.drop.Selected())
	if i < 0 || i >= len(p.entries) {
		return entry{}, false
	}
	return p.entries[i], true
}
