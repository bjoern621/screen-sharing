package discordrpc

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Discord's own bound on stating an activity, past which the client closes the connection.
const (
	sendWindow     = 20 * time.Second
	sendsPerWindow = 5
)

// window is when the sends inside the last sendWindow happened.
//
// Held per connection because the socket answers no question about it:
// a send over the bound is not refused, it ends the connection.
type window struct {
	at []time.Time
}

// take spends one of the window's sends, and answers false where the window is spent.
//
// A spent window drops the send rather than waiting it out.
// The caller states the activity it wants every pass,
// so the one skipped here is stated again a pass later,
// where waiting would hold the poll loop for as long as twenty seconds (internal/app).
func (w *window) take(now time.Time) bool {
	assert.Assert(!now.IsZero(), "a send is dated")

	kept := w.at[:0]
	for _, at := range w.at {
		if now.Sub(at) < sendWindow {
			kept = append(kept, at)
		}
	}
	w.at = kept

	if len(w.at) >= sendsPerWindow {
		return false
	}
	w.at = append(w.at, now)

	assert.Assert(len(w.at) <= sendsPerWindow, "a window holds the sends it allows", len(w.at))
	return true
}
