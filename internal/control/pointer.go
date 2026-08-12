package control

import (
	"time"

	"google.golang.org/grpc"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/wire"
)

// pointerCadence is how often a pointer stream sends.
//
// It is the interval the reader takes positions at rather than a rate chosen here, so a tick
// is one reading: a faster cadence would send the same position twice, and a slower one would
// step over positions that were taken. It is deliberately not the frame rate, which is the
// whole reason the position is sent instead of drawn - a position costs no frame, so a 240 Hz
// pointer over a 30 fps stream is the win.
const pointerCadence = pointer.Interval

// SubscribePointer carries where the publishing machine's pointer is, for as long as the
// shell holds the call.
//
// A stream of its own for the reason the levels are one, and one degree more so: folding it
// into Subscribe would push the publish state at pointer rate for a figure nothing else
// reads.
//
// Each tick is the position read through to the backend, never a delta, so a shell that
// joined late or fell behind is correct again on the next one. Nothing is queued either: the
// ticker's own channel drops the ticks a blocked Send ran past, which is what makes a slow
// reader receive the present rather than a queue of the past - and a stale pointer position
// is the one thing worth dropping, since it describes where the mouse was and not where it
// is.
//
// A publish that is not sending positions sends nothing here, and the stream stays open. The
// cursor mode can change under a subscription, and a shell that had to resubscribe on every
// change would be a shell holding a pointer from the mode before.
func (s *Server) SubscribePointer(req *screensharev1.SubscribePointerRequest, out grpc.ServerStreamingServer[screensharev1.PointerPosition]) error {
	assert.IsNotNil(out, "a pointer subscription writes to the client's stream")
	assert.Assert(pointerCadence > 0, "the pointer ticks at a positive interval", pointerCadence)

	ticker := time.NewTicker(pointerCadence)
	defer ticker.Stop()

	// The last position sent, so an unmoved pointer sends nothing: a position that has not
	// changed is one every reader already has, and the whole reason for the separate stream
	// is that it carries what moves.
	//
	// The three fields rather than the message, because a message carries a lock and copying
	// one is copying that. What is compared is what a viewer draws from, and the moment each
	// was read is deliberately not part of it: a pointer that has not moved has not moved,
	// however many times it was read.
	var lastX, lastY int32
	var lastVisible, sent bool

	done := out.Context().Done()
	for {
		select {
		case <-done:
			// The client went away or stopped listening, which is how this call ends
			// normally. Not a failure and not reported as one, the way Subscribe treats the
			// same event.
			return nil
		case <-ticker.C:
			p, reporting := s.backend.Pointer()
			if !reporting {
				continue
			}
			message := wire.PointerPosition(p)
			if sent && message.GetX() == lastX && message.GetY() == lastY &&
				message.GetVisible() == lastVisible {
				continue
			}
			if err := out.Send(message); err != nil {
				// A failed send is the transport saying the client is gone. It carries a
				// status already, so it travels as it is.
				return err
			}
			lastX, lastY, lastVisible, sent = message.GetX(), message.GetY(), message.GetVisible(), true
		}
	}
}
