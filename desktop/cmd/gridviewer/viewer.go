package main

import (
	"math"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"bjoernblessin.de/screenshare/watch"
)

// viewer is the single grid window: a stack whose "grid" page holds one tile
// per stream and whose "solo" page holds a spotlighted tile. Clicking a tile
// moves it between the pages.
type viewer struct {
	stack   *gtk.Stack
	grid    *gtk.Grid
	solo    *gtk.Box
	players []*player
	spotlit *tile
}

// tile is one stream's widget. It remembers its grid position for the return
// trip from the spotlight page.
type tile struct {
	widget         *gtk.Overlay
	errLabel       *gtk.Label
	col, row, span int
}

const css = `
window { background-color: black; }
.stream-name { background-color: rgba(0,0,0,0.55); color: white; padding: 2px 10px; border-radius: 0 0 8px 0; font-weight: bold; }
.stream-error { background-color: rgba(127,29,29,0.85); color: white; padding: 6px 12px; border-radius: 8px; }
`

// newViewer builds the window and starts one pipeline per stream. A stream
// whose pipeline cannot even parse still gets its tile, showing the error
// where the video would be, so one bad stream never blocks the rest.
func newViewer(app *gtk.Application, cfg watch.GridConfig) *viewer {
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(css)
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	v := &viewer{
		stack: gtk.NewStack(),
		grid:  gtk.NewGrid(),
		solo:  gtk.NewBox(gtk.OrientationVertical, 0),
	}
	v.grid.SetColumnHomogeneous(true)
	v.grid.SetRowHomogeneous(true)
	v.stack.SetTransitionType(gtk.StackTransitionTypeCrossfade)
	v.stack.SetTransitionDuration(150)
	v.stack.AddNamed(v.grid, "grid")
	v.stack.AddNamed(v.solo, "solo")

	// Near-square grid, row-major, the same shape as the wall and the WHEP
	// grid. Tiles span two homogeneous columns, so an incomplete last row
	// centers on a whole one-column offset.
	n := len(cfg.Streams)
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	rows := (n + cols - 1) / cols
	for i, st := range cfg.Streams {
		row := i / cols
		col := i % cols
		inRow := cols
		if row == rows-1 {
			inRow = n - row*cols
		}
		t := &tile{col: (cols - inRow) + 2*col, row: row, span: 2}

		pic := gtk.NewPicture()
		pic.SetHExpand(true)
		pic.SetVExpand(true)

		name := gtk.NewLabel(st.Name)
		name.AddCSSClass("stream-name")
		name.SetHAlign(gtk.AlignStart)
		name.SetVAlign(gtk.AlignStart)

		t.errLabel = gtk.NewLabel("")
		t.errLabel.AddCSSClass("stream-error")
		t.errLabel.SetHAlign(gtk.AlignCenter)
		t.errLabel.SetVAlign(gtk.AlignCenter)
		t.errLabel.SetWrap(true)
		t.errLabel.SetVisible(false)

		t.widget = gtk.NewOverlay()
		t.widget.SetChild(pic)
		t.widget.AddOverlay(name)
		t.widget.AddOverlay(t.errLabel)

		click := gtk.NewGestureClick()
		click.ConnectReleased(func(int, float64, float64) { v.toggleSpotlight(t) })
		t.widget.AddController(click)

		p, err := newPlayer(st, func(message string) {
			// The bus goroutine reports; the label is UI, so hop to the main loop.
			coreglib.IdleAdd(func() {
				t.errLabel.SetText(message)
				t.errLabel.SetVisible(true)
			})
		})
		if err != nil {
			t.errLabel.SetText(err.Error())
			t.errLabel.SetVisible(true)
		} else {
			pic.SetPaintable(p.paintable)
			v.players = append(v.players, p)
		}

		v.grid.Attach(t.widget, t.col, t.row, t.span, 1)
	}

	win := gtk.NewApplicationWindow(app)
	win.SetTitle("native grid (gtk)")
	win.SetDefaultSize(1280, 720)
	win.SetChild(v.stack)
	win.Present()
	return v
}

// toggleSpotlight flips between the grid and a single maximized tile. A widget
// has one parent, so the tile physically moves between the pages while the
// stack crossfades.
func (v *viewer) toggleSpotlight(t *tile) {
	if v.spotlit != nil {
		v.solo.Remove(v.spotlit.widget)
		v.grid.Attach(v.spotlit.widget, v.spotlit.col, v.spotlit.row, v.spotlit.span, 1)
		v.stack.SetVisibleChildName("grid")
		v.spotlit = nil
		return
	}
	v.grid.Remove(t.widget)
	v.solo.Append(t.widget)
	v.stack.SetVisibleChildName("solo")
	v.spotlit = t
}

// stop tears every pipeline down; called after the GTK main loop returns.
func (v *viewer) stop() {
	for _, p := range v.players {
		p.stop()
	}
}
