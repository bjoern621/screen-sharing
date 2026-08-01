package stats

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// line is one rendered row: its box, so it can hide, and its value label.
//
// The explanation is not kept here. Every pass writes it with the value it
// explains, so a row whose reading is a verdict can explain the verdict it shows.
type line struct {
	row   *gtk.Box
	value *gtk.Label
	hides bool
}

// tipTarget is a widget a tooltip can be put on.
type tipTarget interface {
	SetTooltipText(text string)
	SetHasTooltip(hasTooltip bool)
}

// setTip writes a tooltip, or takes the widget out of the tooltip query where
// there is nothing to say. Empty text still counts as a tooltip in GTK, and a
// widget that has one ends the query instead of passing it to the ancestor that
// does.
func setTip(w tipTarget, text string) {
	if text == "" {
		w.SetHasTooltip(false)
		return
	}
	w.SetTooltipText(text)
}

// set writes one value under the explanation of it, hiding the row or showing the
// placeholder when there is no value.
func (l *line) set(v, tip string) {
	if v == "" {
		if l.hides {
			l.row.SetVisible(false)
			return
		}
		v = unknown
	}
	l.row.SetVisible(true)
	l.value.SetText(v)
	setTip(l.row, tip)
	// Codec descriptions outrun the card, so they ellipsize. The value then carries
	// the full text above the row's explanation; a value that fits carries no
	// tooltip of its own and the row's reaches the pointer.
	if len(v) > valueChars {
		setTip(l.value, joinTip(v, tip))
	} else {
		setTip(l.value, "")
	}
}

// newBlock starts a titled block inside the card: a rule, the heading, and the rows
// appended after them, boxed together so hiding the block takes its heading and rule
// with it.
func newBlock(parent *gtk.Box, title, tip string, rule bool) *gtk.Box {
	b := gtk.NewBox(gtk.OrientationVertical, 2)
	if rule {
		b.Append(gtk.NewSeparator(gtk.OrientationHorizontal))
	}
	head := gtk.NewLabel(title)
	head.AddCSSClass("stat-head")
	head.SetXAlign(0)
	setTip(head, tip)
	b.Append(head)
	parent.Append(b)
	return b
}

// newLine appends one key/value row to a block. The row's box holds the tooltip, so
// it covers the key, the value and the gap between them alike.
func newLine(block *gtk.Box, key, tip string, hides bool) *line {
	k := gtk.NewLabel(key)
	k.AddCSSClass("stat-key")
	k.SetXAlign(0)

	v := gtk.NewLabel(unknown)
	v.SetXAlign(1)
	v.SetHExpand(true)
	v.SetEllipsize(pango.EllipsizeEnd)
	v.SetMaxWidthChars(valueChars)

	row := gtk.NewBox(gtk.OrientationHorizontal, 16)
	row.Append(k)
	row.Append(v)
	setTip(row, tip)
	block.Append(row)
	return &line{row: row, value: v, hides: hides}
}
