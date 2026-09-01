package app

import (
	"sync"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/decode"
	"bjoernblessin.de/screenshare/internal/pointer"
)

// The pointer position, held between the child that reads it and the shells that draw it.
//
// The child reports faster than any frame rate and a shell reads on a cadence of its own,
// so one position is held and nothing is queued: a queue would deliver where the mouse was.
//
// Nothing here converts.
// The child reports a fraction of the captured picture, the one space both capture backends answer
// in and the one a viewer can place without knowing what was captured at what size
// (gstrun/pointersource.go).

// pointerState is the newest position and whether one has been read.
type pointerState struct {
	mu sync.Mutex
	at pointer.Spot
	// held is false until the first reading,
	// so a shell subscribing before it is told nothing rather than the origin.
	held bool
}

func (p *pointerState) take(at pointer.Spot) {
	p.mu.Lock()
	p.at, p.held = at, true
	p.mu.Unlock()
}

// clear forgets the reading, which a publish ending means:
// the position belonged to that capture,
// and holding it would draw a pointer over a stopped stream.
func (p *pointerState) clear() {
	p.mu.Lock()
	p.at, p.held = pointer.Spot{}, false
	p.mu.Unlock()
}

func (p *pointerState) read() (pointer.Spot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.at, p.held
}

// Pointer is where the publishing machine's pointer is, as a fraction of the picture it publishes.
//
// False wherever there is nothing honest to say: no publish in force,
// a cursor mode that draws the pointer into the frames or leaves it out,
// or a child that has reported no position.
// A shell draws nothing on false rather than guessing,
// which separates "not there" from "not being sent".
func (a *App) Pointer() (pointer.Spot, bool) {
	at, held := a.pointerAt.read()
	if !held {
		return pointer.Spot{}, false
	}

	a.procMu.Lock()
	live, _ := a.livePublishLocked()
	a.procMu.Unlock()
	if live == nil || live.Publish.Cursor != cursor.Metadata {
		return pointer.Spot{}, false
	}
	return at, true
}

// StreamPointer is where the publisher of a watched stream has the pointer,
// and false where its frames carry none.
//
// Read off the decode rather than held here.
// The position rides in the frames, no leg over the relay carrying a channel beside the picture,
// so the decode holding those frames is the one that can answer (receive/pointer.go).
// A stream nothing is decoding reports none, as one whose publisher sends none does.
func (a *App) StreamPointer(ref StreamRef) (pointer.Spot, bool) {
	state, decoding := a.decodes.Snapshot()[decode.StreamID(ref.Name, ref.Transport)]
	if !decoding || state.Ended || !state.HasPointer {
		return pointer.Spot{}, false
	}
	return state.Pointer, true
}
