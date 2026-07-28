package sidebar

import (
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// build wraps the list in the scroller, the header the window shows it in, and
// the app bar under both.
func (v *View) build() gtk.Widgetter {
	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(v.list)
	scroll.SetVExpand(true)

	header := adw.NewHeaderBar()
	header.SetTitleWidget(v.title)

	view := adw.NewToolbarView()
	view.AddTopBar(header)
	view.SetContent(scroll)
	view.AddBottomBar(v.app.Widget())
	return view
}

// drawTitle states the open-tile count beside the heading, the way the web roster's
// badge does. An empty subtitle collapses, so a grid with nothing open shows the
// heading alone, like the web shows no badge.
func (v *View) drawTitle() {
	n := v.sess.Watching()
	if n == 0 {
		v.title.SetSubtitle("")
		return
	}
	v.title.SetSubtitle(fmt.Sprintf("%d watching", n))
}
