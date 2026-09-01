package gstrun

import (
	"sync"
	"time"

	"bjoernblessin.de/screenshare/internal/pointer"
)

// pointerHold is the newest reading and the picture it was taken over.
//
// Pixels are held rather than the fraction they are reported as: a reading taken before the caps
// arrive has no picture to be a fraction of, and holding it raw makes it answerable the moment
// they do.
//
// One position and no queue.
// A source reports faster than any reader asks, so a queue would hand out where the pointer was.
type pointerHold struct {
	mu sync.Mutex
	// x and y are in the captured picture's own pixels, from its top left.
	x, y int
	// read is false until the first reading, so a reader before it is told nothing rather than
	// the origin.
	read    bool
	visible bool
	at      time.Time
	// width and height are the captured picture's, 0 until the caps arrive.
	width, height int
}

func (h *pointerHold) take(x, y int, visible bool) {
	h.mu.Lock()
	h.x, h.y, h.visible, h.at, h.read = x, y, visible, time.Now(), true
	h.mu.Unlock()
}

func (h *pointerHold) size(width, height int) {
	h.mu.Lock()
	h.width, h.height = width, height
	h.mu.Unlock()
}

// spot is the reading as a fraction of the captured picture, and false where there is none to give.
//
// A position outside the picture is carried as one that is not visible.
// ximagesrc reads the whole X root and the crop takes one output out of it,
// so the pointer on another head of the same display lands outside.
// The portal answers for the region it captures and reports the pointer leaving it,
// which is the same fact.
// Nothing is clamped: an edge a marker is stuck against says "here" for as long as the pointer
// is away.
//
// Safe on a nil hold, which is a run whose capture answers no position.
func (h *pointerHold) spot() (pointer.Spot, bool) {
	if h == nil {
		return pointer.Spot{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.read || h.width <= 0 || h.height <= 0 {
		return pointer.Spot{}, false
	}
	if !h.visible || h.x < 0 || h.y < 0 || h.x >= h.width || h.y >= h.height {
		return pointer.Spot{At: h.at}, true
	}
	return pointer.Spot{
		X:       float64(h.x) / float64(h.width),
		Y:       float64(h.y) / float64(h.height),
		At:      h.at,
		Visible: true,
	}, true
}
