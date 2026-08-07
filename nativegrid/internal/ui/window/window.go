// Package window assembles the grid window: the stream sidebar beside the tile
// area, under the theme both follow.
//
// What is left when the two views are taken away is the window's own: the
// keyboard table (shortcuts.go), what its keys do (actions.go) and the geometry
// it reopens at (geometry.go).
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

// decoration is the window buttons the content header carries, in GTK's layout
// syntax: nothing before the colon, the platform's three after it.
//
// The desktop's own layout is overridden rather than followed, and what is overridden
// is where the buttons sit, not which ones there are. All three are named because a
// window is a window: minimising it is what a viewer does to get at what is behind it,
// and maximise is what the sidebar toggle and fullscreen between them still leave it
// needing, since neither gives back the shape between windowed and the whole screen. A
// desktop that puts its buttons at the start would stack them on the toggle, which is
// the one thing the side of the colon they are named on settles.
const decoration = ":minimize,maximize,close"

// chromeIcons holds the window chrome's icons, which stay registered for the process
// lifetime: the chrome outlives every tile.
var chromeIcons theme.Icons

// chrome is the window around the views: the widgets a shortcut acts on and the
// state the geometry is read off.
type chrome struct {
	win     *adw.ApplicationWindow
	split   *adw.OverlaySplitView
	toolbar *adw.ToolbarView
	header  *adw.HeaderBar
	toggle  *gtk.ToggleButton
	tiles   *grid.View
	sess    *session.Session
	store   layout.WindowStore
	// settingSidebar is up while showSidebar writes the two halves of the sidebar
	// control, whose property notifies land back in it (geometry.go).
	settingSidebar bool
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
	assert.IsNotNil(dispatch, "a window defers its deferred work to a UI loop")

	// Before the stylesheet, which sizes part of what it draws against the font
	// this settles.
	theme.PinSettings()
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
	c.tiles = grid.New(sess, drag, dispatch)
	sess.Observe(streams)
	sess.Observe(c.tiles)

	c.split = adw.NewOverlaySplitView()
	c.split.SetSidebar(streams.Widget())
	c.split.SetContent(c.content())
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
// toggle at the start and the window buttons at the end.
//
// The header is a frame the tiles sit under, and it holds whatever the sidebar is
// in: hiding the sidebar is a wider tile area, not a window with its controls taken
// away. The window's edges the tiles do reach are the grid's own margin to give up
// (grid.SetFlush).
func (c *chrome) content() gtk.Widgetter {
	assert.IsNotNil(c.tiles, "the window's content is the tile area")

	c.toggle = gtk.NewToggleButton()
	c.toggle.SetChild(chromeIcons.Image("layout-sidebar", toggleIconSize, theme.Foreground))
	c.toggle.SetTooltipText(tip(accelSidebar))
	// The button asks for a sidebar state like every other path does.
	// Which state it opens in is restoreGeometry's.
	c.toggle.ConnectToggled(func() { c.showSidebar(c.toggle.Active()) })

	c.header = adw.NewHeaderBar()
	c.header.SetShowTitle(false)
	c.header.SetDecorationLayout(decoration)
	c.header.PackStart(c.toggle)

	c.toolbar = adw.NewToolbarView()
	c.toolbar.AddTopBar(c.header)
	c.toolbar.SetContent(c.tiles.Widget())
	return c.toolbar
}

// followTheme keeps the window on the system theme. The .dark class switches the
// flattened dark token values in style.css, and the themed Tabler icons re-render,
// since their color is baked in at rasterization.
func followTheme(win *adw.ApplicationWindow) {
	assert.IsNotNil(win, "a theme is followed by a window")

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
