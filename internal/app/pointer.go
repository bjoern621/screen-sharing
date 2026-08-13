package app

import (
	"sync"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/settings"
)

// Where the pointer is, held between the child that reads it and the shells that draw it.
//
// The child reports at its own rate, which is faster than any frame rate, and a shell reads on a
// cadence of its own.
// So what is held is the newest position and nothing else: a queue would deliver where the mouse
// was, and a pointer is the one figure where only the present is worth having.
//
// The position is turned into the captured picture's own pixels here, because this is the one place
// that holds both halves - what the child read, in the display server's coordinates,
// and which screen the publish is reading and how it is scaled.
// A viewer has neither, so a position that crossed as the desktop's would be one nothing could
// place.

// pointerState is the newest position, with the settings it was read under.
type pointerState struct {
	mu sync.Mutex
	// at is the newest position, in the display server's own pixels.
	at pointer.Position
	// held reports whether anything has been read at all, so a shell that subscribes before the first
	// reading is told nothing rather than told the origin.
	held bool
}

// take records one reading.
func (p *pointerState) take(at pointer.Position) {
	p.mu.Lock()
	p.at, p.held = at, true
	p.mu.Unlock()
}

// clear forgets what was read, which is what a publish ending means: the position belonged to that
// capture, and holding it would draw a pointer over a stream that has stopped.
func (p *pointerState) clear() {
	p.mu.Lock()
	p.at, p.held = pointer.Position{}, false
	p.mu.Unlock()
}

// read is the newest position and whether there is one.
func (p *pointerState) read() (pointer.Position, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.at, p.held
}

// Pointer is where the publishing machine's pointer is, in the pixels the stream carries.
//
// It answers false wherever there is nothing honest to say: no publish in force,
// a cursor mode that draws the pointer into the frames or leaves it out, or a capture whose child
// has not reported a position yet.
// A shell holding the answer knows the difference between "not there" and "not being sent",
// which is what lets it draw nothing rather than draw a guess.
func (a *App) Pointer() (pointer.Position, bool) {
	at, held := a.pointerAt.read()
	if !held {
		return pointer.Position{}, false
	}

	a.procMu.Lock()
	live, _ := a.livePublishLocked()
	a.procMu.Unlock()
	if live == nil || live.Publish.Cursor != cursor.Metadata {
		return pointer.Position{}, false
	}

	return a.pointerInPicture(*live, at), true
}

// pointerInPicture turns a position in the display server's pixels into one in the picture the
// stream carries.
//
// Two steps, and both are the publisher's own facts.
// The captured screen's origin is subtracted, because a desktop composes its outputs onto one
// coordinate space and a capture reads one of them; then the scale the settings ask for is applied,
// because what the stream carries is the scaled picture and a viewer's pixels are its pixels.
//
// A monitor the enumeration does not report leaves the position where it is.
// That is the same answer the bitrate prediction gives for an unpriced monitor:
// what is missing is this machine's own answer about its screens, and inventing an origin would
// place the pointer somewhere nothing measured.
func (a *App) pointerInPicture(s settings.Settings, at pointer.Position) pointer.Position {
	monitor, known := display.At(s.Publish.Monitor)
	if known {
		at.X -= monitor.OffsetX
		at.Y -= monitor.OffsetY
		// Outside the captured screen is outside the picture, whatever the display server says about its
		// own root: a pointer on the other monitor is not over this capture.
		if at.X < 0 || at.Y < 0 || at.X >= monitor.Width || at.Y >= monitor.Height {
			at.Visible = false
		}
	}

	size, scaled, err := s.Publish.OutputSize()
	if err != nil || !scaled || !known || monitor.Width <= 0 || monitor.Height <= 0 {
		return at
	}
	at.X = at.X * size.Width / monitor.Width
	at.Y = at.Y * size.Height / monitor.Height
	return at
}
