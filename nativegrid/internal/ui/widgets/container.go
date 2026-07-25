package widgets

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// ClearBox and ClearGrid empty a container before it is filled again, so a widget
// that moved never lands in a second parent. GTK4 has no container interface, so
// the removal call belongs to the concrete widget and each gets its own helper.

func ClearBox(b *gtk.Box) {
	for {
		c := b.FirstChild()
		if c == nil {
			return
		}
		b.Remove(c)
	}
}

func ClearGrid(g *gtk.Grid) {
	for {
		c := g.FirstChild()
		if c == nil {
			return
		}
		g.Remove(c)
	}
}
