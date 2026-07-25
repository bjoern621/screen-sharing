package stats

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// line is one rendered row: its box, so it can hide, and its value label.
type line struct {
	row   *gtk.Box
	value *gtk.Label
	hides bool
}

// set writes one value, hiding the row or showing the placeholder when there is
// none.
func (l *line) set(v string) {
	if v == "" {
		if l.hides {
			l.row.SetVisible(false)
			return
		}
		v = unknown
	}
	l.row.SetVisible(true)
	l.value.SetText(v)
	// Codec descriptions outrun the card, so they ellipsize; the tooltip keeps the
	// full value reachable, and short values carry none.
	if len(v) > valueChars {
		l.value.SetTooltipText(v)
	} else {
		l.value.SetTooltipText("")
	}
}

// newBlock starts a titled block inside the card: a rule, the heading, and the rows
// appended after them, boxed together so hiding the block takes its heading and rule
// with it.
func newBlock(parent *gtk.Box, title string, rule bool) *gtk.Box {
	b := gtk.NewBox(gtk.OrientationVertical, 2)
	if rule {
		b.Append(gtk.NewSeparator(gtk.OrientationHorizontal))
	}
	head := gtk.NewLabel(title)
	head.AddCSSClass("stat-head")
	head.SetXAlign(0)
	b.Append(head)
	parent.Append(b)
	return b
}

// newLine appends one key/value row to a block.
func newLine(block *gtk.Box, key string, hides bool) *line {
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
	block.Append(row)
	return &line{row: row, value: v, hides: hides}
}
