package window

import (
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
)

// restoreGeometry opens the window on the shape the last run left it in.
// It runs before the window maps, so the first frame is drawn at the remembered
// size instead of resized into it afterwards.
// A run with nothing remembered opens at the built-in size with the sidebar out.
func (c *chrome) restoreGeometry() {
	state := c.store.LoadWindow()
	if !state.Remembered() {
		c.win.SetDefaultSize(defaultWidth, defaultHeight)
		c.showSidebar(true)
		return
	}
	c.win.SetDefaultSize(state.Width, state.Height)
	if state.Maximized {
		c.win.Maximize()
	}
	c.showSidebar(state.SidebarShown)
	c.written = state
	logger.Debugf("window opens at %dx%d (maximized=%t, sidebar=%t)",
		state.Width, state.Height, state.Maximized, state.SidebarShown)
}

// showSidebar puts both halves of the sidebar control in one state: the split
// shows the sidebar, the header button reads back what it shows.
func (c *chrome) showSidebar(shown bool) {
	c.split.SetShowSidebar(shown)
	c.toggle.SetActive(shown)
}

// trackGeometry writes the geometry back as it changes.
//
// A resize reaches the window as a change of its default size, GTK's own record
// of the shape a window returns to, so the two size properties and the maximized
// flag are the whole of what a size or state change looks like from here.
// The writes are coalesced, because dragging a window edge reports a new size on
// every frame and the file is shared with the arrangement's writer.
// A close request writes on the spot: the loop a coalescer defers to ends with it.
func (c *chrome) trackGeometry(dispatch idle.Dispatch) {
	c.persist = idle.New(dispatch, c.writeGeometry)

	for _, prop := range []string{"default-width", "default-height", "maximized"} {
		c.win.NotifyProperty(prop, c.persist.Schedule)
	}
	// The split view is the state the sidebar is really in, whether Ctrl+B, the
	// header button or a collapse put it there.
	c.split.NotifyProperty("show-sidebar", func() {
		c.toggle.SetActive(c.split.ShowSidebar())
		c.persist.Schedule()
	})
	c.win.ConnectCloseRequest(func() bool {
		c.writeGeometry()
		// false lets the close go through: the geometry is recorded here, not
		// defended.
		return false
	})
}

// writeGeometry saves the shape the window holds now, unless that is the shape
// already on file: one resize notifies per property, and a property set to the
// value it already holds notifies too.
func (c *chrome) writeGeometry() {
	// A fullscreen window is not reporting a shape to reopen at.
	// The toplevel state it carries is the compositor's for as long as fullscreen
	// lasts, and fullscreen itself is not carried across runs, so what the window
	// had before it is what the next run opens on, and that is already on file.
	if c.win.IsFullscreen() {
		return
	}
	width, height := c.win.DefaultSize()
	state := layout.WindowState{
		Width:        width,
		Height:       height,
		Maximized:    c.win.IsMaximized(),
		SidebarShown: c.split.ShowSidebar(),
	}
	if state == c.written {
		return
	}
	c.store.SaveWindow(state)
	c.written = state
	logger.Debugf("window geometry written: %dx%d (maximized=%t, sidebar=%t)",
		state.Width, state.Height, state.Maximized, state.SidebarShown)
}
