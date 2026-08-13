package app

import (
	"sync"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The pointer position, held between the child that reads it and the shells that draw it.
//
// The child reports faster than any frame rate and a shell reads on a cadence of its own, so one
// position is held and nothing is queued: a queue would deliver where the mouse was.
//
// The conversion into the captured picture's pixels happens here, the one place holding both halves:
// the reading in the display server's coordinates, and which screen the publish reads and at what
// scale.
// A viewer holds neither, so a position crossing as the desktop's would be one nothing could place.

// pointerState is the newest position and whether one has been read.
type pointerState struct {
	mu sync.Mutex
	// at is in the display server's own pixels.
	at pointer.Position
	// held is false until the first reading, so a shell subscribing before it is told nothing rather
	// than told the origin.
	held bool
}

func (p *pointerState) take(at pointer.Position) {
	p.mu.Lock()
	p.at, p.held = at, true
	p.mu.Unlock()
}

// clear forgets the reading, which is what a publish ending means: the position belonged to that
// capture, and holding it would draw a pointer over a stopped stream.
func (p *pointerState) clear() {
	p.mu.Lock()
	p.at, p.held = pointer.Position{}, false
	p.mu.Unlock()
}

func (p *pointerState) read() (pointer.Position, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.at, p.held
}

// Pointer is where the publishing machine's pointer is, in the pixels the stream carries.
//
// False wherever there is nothing honest to say: no publish in force, a cursor mode that draws the
// pointer into the frames or leaves it out, or a child that has reported no position.
// A shell draws nothing on false rather than guessing, which is what separates "not there" from
// "not being sent".
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
// Two steps, both off the publisher's own facts.
// The captured screen's origin comes off first, because a desktop composes its outputs onto one
// coordinate space and a capture reads one of them.
// The settings' scale goes on second, because the stream carries the scaled picture and a viewer's
// pixels are its pixels.
//
// A monitor the enumeration does not report leaves the position untouched, the answer the bitrate
// prediction gives for an unpriced monitor: inventing an origin would place the pointer somewhere
// nothing measured.
func (a *App) pointerInPicture(s settings.Settings, at pointer.Position) pointer.Position {
	monitor, known := display.At(s.Publish.Monitor)
	if known {
		at.X -= monitor.OffsetX
		at.Y -= monitor.OffsetY
		// Outside the captured screen is outside the picture, whatever the display server reports for
		// its own root: a pointer on another monitor is not over this capture.
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
