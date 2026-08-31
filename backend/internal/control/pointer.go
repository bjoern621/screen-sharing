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
// The reader's own interval, not a rate chosen here, so one tick is one reading:
// faster would send the same position twice, slower would step over positions that were taken.
// Not the frame rate: a position costs no frame,
// so a 240 Hz pointer over a 30 fps stream is the win over drawing it into the picture.
const pointerCadence = pointer.Interval

// SubscribePointer carries where the publishing machine's pointer is,
// for as long as the shell holds the call.
//
// A stream of its own for the reason the levels are one, and one degree more so:
// folded into Subscribe it would push the whole publish state at pointer rate,
// for a figure nothing else reads.
//
// Every tick is the position read through to the backend and never a delta,
// so a shell that joined late or fell behind is right again on the next one.
// Nothing queues either: the ticker's channel drops the ticks a blocked Send ran past,
// so a slow reader receives the present instead of a queue of the past.
// A stale position is the one worth dropping: it says where the mouse was, not where it is.
//
// A publish reporting no position sends nothing here and the stream stays open.
// The cursor mode can change under a subscription,
// and a shell made to resubscribe on every change would hold a pointer from the mode before.
func (s *Server) SubscribePointer(req *screensharev1.SubscribePointerRequest, out grpc.ServerStreamingServer[screensharev1.PointerPosition]) error {
	assert.IsNotNil(out, "a pointer subscription writes to the client's stream")
	assert.Assert(pointerCadence > 0, "the pointer ticks at a positive interval", pointerCadence)

	ticker := time.NewTicker(pointerCadence)
	defer ticker.Stop()

	// The last position sent, so an unmoved pointer sends nothing:
	// the separate stream carries what moves, and an unchanged position every reader already holds.
	//
	// Three fields rather than the message, a message carrying a lock that copying it would copy.
	// The moment of the reading is left out: what is compared is what a viewer draws from,
	// and a pointer that has not moved has not moved however often it was read.
	var lastX, lastY int32
	var lastVisible, sent bool

	done := out.Context().Done()
	for {
		select {
		case <-done:
			// A client gone or one that stopped listening ends this call normally.
			// No failure and nothing reported, as in Subscribe.
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
				// A failed send is the transport reporting the client gone.
				// The error carries a status already, so it goes back unwrapped.
				return err
			}
			lastX, lastY, lastVisible, sent = message.GetX(), message.GetY(), message.GetVisible(), true
		}
	}
}
