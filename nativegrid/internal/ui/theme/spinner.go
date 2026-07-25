package theme

import (
	"math"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// The spinner keeps the loader-2 glyph's proportions in a 24 box: a 2px stroke on
// a 9px radius, and a 270-degree arc with round caps, at any size.
const (
	spinnerStroke = 2.0 / 24
	spinnerRadius = 9.0 / 24
	spinnerArcEnd = math.Pi
	spinnerTurn   = 1_000_000 // one revolution per second, in frame-clock microseconds
)

// Spinner draws the Tabler loader-2 glyph and rotates it from the frame clock: the
// web app's animate-spin, 1s linear, on IconLoader2. It is drawn rather than
// rasterized because GTK CSS cannot animate transforms. The color is the widget's
// CSS color, so a .tile-spinner class or the theme foreground applies unchanged.
type Spinner struct {
	area *gtk.DrawingArea
}

func NewSpinner(size int) *Spinner {
	s := &Spinner{area: gtk.NewDrawingArea()}
	s.area.SetSizeRequest(size, size)
	s.area.SetHAlign(gtk.AlignCenter)
	s.area.SetVAlign(gtk.AlignCenter)
	s.area.SetDrawFunc(s.draw)
	s.area.AddTickCallback(func(_ gtk.Widgetter, _ gdk.FrameClocker) bool {
		s.area.QueueDraw()
		return true
	})
	return s
}

// Widget is the spinner's drawing area, for a container to hold.
func (s *Spinner) Widget() *gtk.DrawingArea { return s.area }

func (s *Spinner) draw(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
	size := math.Min(float64(w), float64(h))
	fg := s.area.StyleContext().Color()
	cr.SetSourceRGBA(float64(fg.Red()), float64(fg.Green()), float64(fg.Blue()), float64(fg.Alpha()))

	clock := gdk.BaseFrameClock(s.area.FrameClock())
	turn := float64(clock.FrameTime()%spinnerTurn) / spinnerTurn

	cr.Translate(float64(w)/2, float64(h)/2)
	cr.Rotate(turn * 2 * math.Pi)
	cr.SetLineWidth(size * spinnerStroke)
	cr.SetLineCap(cairo.LineCapRound)
	cr.Arc(0, 0, size*spinnerRadius, -math.Pi/2, spinnerArcEnd)
	cr.Stroke()
}
