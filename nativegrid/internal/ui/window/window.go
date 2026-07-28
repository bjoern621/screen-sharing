// Package window assembles the grid window: the stream sidebar beside the tile
// area, under the theme both follow.
//
// What is left when the two views are taken away is the window's own: the
// keyboard bindings (shortcuts.go) and the geometry it reopens at
// (geometry.go).
package window

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/dnd"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/grid"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/sidebar"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/theme"
)

// The window's own geometry and the sidebar's share of it. The size is what a
// run with nothing remembered opens at.
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

// chrome is the window around the views: the widgets a shortcut acts on and the
// state the geometry is read off.
type chrome struct {
	win    *adw.ApplicationWindow
	split  *adw.OverlaySplitView
	toggle *gtk.ToggleButton
	sess   *session.Session
	store  layout.WindowStore
	// persist folds a burst of geometry changes into one write, and written holds
	// what that write put on file (geometry.go).
	persist *idle.Coalescer
	written layout.WindowState
}

// New builds the window and subscribes its views to the model.
//
// The remembered arrangement is restored here, after the views are in place and
// before the window maps: a restored watch draws exactly like a clicked one, and the
// tiles it opens are there from the first frame the window draws. The remembered
// geometry lands before the map for the same reason.
func New(app *adw.Application, sess *session.Session, dispatch idle.Dispatch) *adw.ApplicationWindow {
	assert.IsNotNil(app, "a window belongs to an application")
	assert.IsNotNil(sess, "a window draws a session")

	theme.LoadStyle()

	c := &chrome{
		win:  adw.NewApplicationWindow(&app.Application),
		sess: sess,
		// The window's geometry lands in the file the session keeps the arrangement
		// in, through a store of its own: the session's store is the session's, and
		// FileStore merges the two records rather than sharing an instance.
		store: layout.NewFileStore(),
	}
	c.win.SetTitle(title)

	// One drag controller for both surfaces: they show the same order, so a drag
	// started on either previews and commits in both.
	drag := dnd.New(sess)
	streams := sidebar.New(sess, drag, dispatch)
	tiles := grid.New(sess, drag, dispatch)
	sess.Observe(streams)
	sess.Observe(tiles)

	c.split = adw.NewOverlaySplitView()
	c.split.SetSidebar(streams.Widget())
	c.split.SetContent(c.content(tiles))
	c.split.SetSidebarWidthFraction(sidebarFraction)
	c.split.SetMinSidebarWidth(sidebarMinWidth)
	c.win.SetContent(c.split)

	c.restoreGeometry()
	c.trackGeometry(dispatch)
	c.bindKeys()

	sess.Restore()
	followTheme(c.win)
	return c.win
}

// content is the tile area under a flat header bar. The header carries the sidebar
// toggle at the start and the window close button at the end.
func (c *chrome) content(tiles *grid.View) gtk.Widgetter {
	c.toggle = gtk.NewToggleButton()
	c.toggle.SetChild(chromeIcons.Image("layout-sidebar", toggleIconSize, theme.Foreground))
	c.toggle.SetTooltipText(tip(accelSidebar))
	// Which state the button opens in is restoreGeometry's, so the sidebar it
	// stands for is set in one place.
	c.toggle.ConnectToggled(func() { c.split.SetShowSidebar(c.toggle.Active()) })

	header := adw.NewHeaderBar()
	header.SetShowTitle(false)
	header.PackStart(c.toggle)

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
