package window

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
)

// restoreGeometry opens the window at the size and with the sidebar the last run
// left.
// It runs before the window maps, so the first frame is drawn at the remembered
// size instead of resized into it afterwards.
// Fullscreen is not part of the record: a grid spawned straight into fullscreen
// would cover the app that spawned it, so every run opens windowed and fullscreen
// is a mode entered by hand (actions.go).
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

// showSidebar is the one writer of the sidebar's visibility.
// It puts both halves of the control in the state asked for: the split shows the
// sidebar, and the header button stands for what the split shows.
//
// Every path that changes the sidebar comes through here, whether it started at
// the button, at Ctrl+B, at the restored geometry or at the split collapsing on
// its own.
// Writing one half and letting its notify carry the other leaves the pair
// converging by the order the signals happen to arrive in.
//
// The write is guarded because setting either half notifies back into this
// function.
// The reentrant call has nothing left to do: the outer one is already setting the
// state it would ask for.
func (c *chrome) showSidebar(shown bool) {
	assert.IsNotNil(c.split, "the sidebar sits in a split view")
	assert.IsNotNil(c.toggle, "the sidebar has a header button standing for it")

	if c.settingSidebar {
		return
	}
	c.settingSidebar = true
	c.split.SetShowSidebar(shown)
	c.toggle.SetActive(shown)
	c.settingSidebar = false

	assert.Assert(c.split.ShowSidebar() == c.toggle.Active(),
		"the header button reads back the split's sidebar", shown, c.split.ShowSidebar(), c.toggle.Active())

	// The sidebar gone hands the tile area the width the sidebar held and the margin
	// the grid keeps under a frame, so a tile meets the window on the sides its aspect
	// fills. Nothing else moves: the header stays the frame it is, and whether the
	// window is windowed, maximized or fullscreen is the header buttons' and F11's
	// (actions.go).
	c.tiles.SetFlush(!shown)
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
	assert.IsNotNil(dispatch, "geometry writes are deferred to a UI loop")

	c.persist = idle.New(dispatch, c.writeGeometry)

	for _, prop := range []string{"default-width", "default-height", "maximized"} {
		c.win.NotifyProperty(prop, c.persist.Schedule)
	}
	// The split view is the state the sidebar is really in, whether Ctrl+B, the
	// header button or a collapse put it there.
	// A collapse is the one of those that changes it without being asked, so what
	// it landed on goes back through the writer to take the button with it.
	c.split.NotifyProperty("show-sidebar", func() {
		c.showSidebar(c.split.ShowSidebar())
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
	state := c.written
	state.SidebarShown = c.split.ShowSidebar()
	// A fullscreen window is not reporting a shape to reopen at: the toplevel state
	// it carries is the compositor's for as long as fullscreen lasts, and what the
	// window had before it is what the next run opens on. The sidebar is the
	// window's own in either mode, so only the shape is held back.
	if !c.win.IsFullscreen() {
		state.Width, state.Height = c.win.DefaultSize()
		state.Maximized = c.win.IsMaximized()
	}
	if state == c.written {
		return
	}
	c.store.SaveWindow(state)
	c.written = state
	logger.Debugf("window geometry written: %dx%d (maximized=%t, sidebar=%t)",
		state.Width, state.Height, state.Maximized, state.SidebarShown)
}
