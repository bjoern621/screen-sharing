package receive

import (
	"time"

	"bjoernblessin.de/screenshare/internal/framestamp"
	"bjoernblessin.de/screenshare/internal/pointer"
)

// Where the publishing machine's pointer is, as its frames carry it.
//
// Read off the bitstream and asked of nobody.
// A relay carries no channel beside the picture,
// so the position rides in the frames it belongs to (internal/framestamp).
// It therefore arrives with the picture it was read over rather than ahead of it.
//
// The rate is the frame rate, that being what the position rides.

// pointerState packs what one stamp said about the pointer into a single word,
// so a reader takes the three fields as one answer rather than as three reads a frame can land
// between.
// Layout: state in bits 32 and up, x in 16..31, y in 0..15.
const (
	pointerStateShift = 32
	pointerXShift     = 16
	pointerAxisMask   = 0xFFFF
)

// holdPointer takes what one frame said about the pointer.
func (t *decodeTrack) holdPointer(s framestamp.Stamp) {
	t.pointerHeld.Store(uint64(s.Pointer)<<pointerStateShift |
		uint64(s.PointerX)<<pointerXShift | uint64(s.PointerY))
	t.pointerAt.Store(s.At.UnixNano())
}

// pointer is where the publisher had the pointer on the newest stamped frame,
// and false where the frames carry no position.
//
// The moment is the publisher's own, the instant that frame left its encoder:
// the position was read over that picture, so it is as old as the picture is.
func (t *decodeTrack) pointer() (pointer.Spot, bool) {
	held := t.pointerHeld.Load()
	state := framestamp.PointerState(held >> pointerStateShift)
	if state == framestamp.PointerNone {
		return pointer.Spot{}, false
	}

	at := pointer.Spot{At: time.Unix(0, t.pointerAt.Load())}
	if state == framestamp.PointerAway {
		return at, true
	}
	at.X = float64(held>>pointerXShift&pointerAxisMask) / framestamp.PointerWhole
	at.Y = float64(held&pointerAxisMask) / framestamp.PointerWhole
	at.Visible = true
	return at, true
}

// Pointer is where the publishing machine's pointer is on the frames arriving,
// and false where they carry none.
//
// The video track alone, the position riding the coded picture.
// A stream this app did not publish carries no stamp and reports nothing,
// as does one whose publisher draws the pointer into the frames or leaves it out.
func (r *Receiver) Pointer() (pointer.Spot, bool) {
	return r.video.pointer()
}
