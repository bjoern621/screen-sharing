// Package window assembles the grid window: the stream sidebar beside the tile
// area, under the theme both follow.
package window

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/dnd"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/grid"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/sidebar"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The window's own geometry and the sidebar's share of it.
const (
	title           = "Native grid"
	defaultWidth    = 1100
	defaultHeight   = 720
	sidebarFraction = 0.22
	sidebarMinWidth = 200
	toggleIconSize  = 20
)

// chromeIcons holds the window chrome's icons, which stay registered for the process
// lifetime: the chrome outlives every tile.
var chromeIcons theme.Icons

// New builds the window and subscribes its views to the model.
//
// The remembered arrangement is restored here, after the views are in place and
// before the window maps: a restored watch draws exactly like a clicked one, and the
// tiles it opens are there from the first frame the window draws.
func New(app *adw.Application, sess *session.Session, dispatch idle.Dispatch) *adw.ApplicationWindow {
	assert.IsNotNil(app, "a window belongs to an application")
	assert.IsNotNil(sess, "a window draws a session")

	theme.LoadStyle()

	win := adw.NewApplicationWindow(&app.Application)
	win.SetTitle(title)
	win.SetDefaultSize(defaultWidth, defaultHeight)

	// One drag controller for both surfaces: they show the same order, so a drag
	// started on either previews and commits in both.
	drag := dnd.New(sess)
	streams := sidebar.New(sess, drag, dispatch)
	tiles := grid.New(sess, drag, dispatch)
	sess.Observe(streams)
	sess.Observe(tiles)

	split := adw.NewOverlaySplitView()
	split.SetSidebar(streams.Widget())
	split.SetContent(content(split, tiles))
	split.SetSidebarWidthFraction(sidebarFraction)
	split.SetMinSidebarWidth(sidebarMinWidth)
	win.SetContent(split)

	sess.Restore()
	followTheme(win)
	return win
}

// content is the tile area under a flat header bar. The header carries the sidebar
// toggle at the start and the window close button at the end.
func content(split *adw.OverlaySplitView, tiles *grid.View) gtk.Widgetter {
	toggle := gtk.NewToggleButton()
	toggle.SetChild(chromeIcons.Image("layout-sidebar", toggleIconSize, theme.Foreground))
	toggle.SetActive(true)
	toggle.ConnectToggled(func() { split.SetShowSidebar(toggle.Active()) })

	header := adw.NewHeaderBar()
	header.SetShowTitle(false)
	header.PackStart(toggle)

	view := adw.NewToolbarView()
	view.AddTopBar(header)
	view.SetContent(tiles.Widget())
	return view
}

// followTheme keeps the window on the system theme. The .dark class switches the
// flattened dark token values in style.css, and the themed Tabler icons re-render,
// since their color is baked in at rasterization.
func followTheme(win *adw.ApplicationWindow) {
	manager := adw.StyleManagerGetDefault()
	apply := func() {
		if manager.Dark() {
			win.AddCSSClass("dark")
		} else {
			win.RemoveCSSClass("dark")
		}
		theme.ApplyDark(manager.Dark())
	}
	apply()
	manager.NotifyProperty("dark", apply)
}
