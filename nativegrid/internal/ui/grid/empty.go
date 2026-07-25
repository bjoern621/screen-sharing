package grid

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// emptyIcons holds the empty page's icon. The page lives as long as the window, so
// its icon stays registered for the process lifetime and the set is never released.
var emptyIcons theme.Icons

// emptyState mirrors the web grid's empty page: a circular muted badge with the
// Tabler video icon, and one muted sentence.
func emptyState() gtk.Widgetter {
	icon := emptyIcons.Image("video", 22, theme.Muted)
	icon.SetHAlign(gtk.AlignCenter)
	icon.SetVAlign(gtk.AlignCenter)
	icon.SetHExpand(true)
	icon.SetVExpand(true)

	badge := gtk.NewBox(gtk.OrientationVertical, 0)
	badge.AddCSSClass("empty-icon")
	badge.SetHAlign(gtk.AlignCenter)
	badge.Append(icon)

	inner := gtk.NewBox(gtk.OrientationVertical, gap)
	inner.AddCSSClass("empty-state")
	inner.SetHAlign(gtk.AlignCenter)
	inner.SetVAlign(gtk.AlignCenter)
	inner.Append(badge)
	inner.Append(gtk.NewLabel("pick a stream in the sidebar to start watching"))

	outer := gtk.NewBox(gtk.OrientationVertical, 0)
	outer.SetHExpand(true)
	outer.SetVExpand(true)
	outer.SetHAlign(gtk.AlignCenter)
	outer.SetVAlign(gtk.AlignCenter)
	outer.Append(inner)
	return outer
}
