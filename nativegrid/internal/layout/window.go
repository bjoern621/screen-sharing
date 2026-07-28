package layout

// WindowState is the shape the grid window reopens at: the size it restores to,
// whether it opens maximized, and whether the stream sidebar is out.
//
// Fullscreen is not carried across runs.
// It is a mode taken for as long as one stream is worth the whole screen, not a
// shape the window keeps, and a grid the app spawns straight into fullscreen
// covers the app that spawned it with no frame left to say what is on top.
// F11 is one keystroke away in the run that wants it.
type WindowState struct {
	Width        int  `json:"windowWidth"`
	Height       int  `json:"windowHeight"`
	Maximized    bool `json:"windowMaximized"`
	SidebarShown bool `json:"sidebarShown"`
}

// Remembered reports whether the record carries a geometry to open on.
// A zero width or height is a run that never wrote one, which makes the rest of
// the record the zero value of a record nobody wrote rather than a shape
// somebody chose: the window's built-in defaults apply to all of it.
func (w WindowState) Remembered() bool { return w.Width > 0 && w.Height > 0 }

// WindowStore reads and writes the remembered window geometry. It is split from
// Store because the two records have different owners and are written at
// different moments, not because they are kept apart: FileStore puts both in one
// file.
//
// Neither call reports an error, for the reason Store gives: a geometry that
// cannot be read is a first run, and a write that fails costs the next run's
// size and nothing else.
type WindowStore interface {
	LoadWindow() WindowState
	SaveWindow(w WindowState)
}
