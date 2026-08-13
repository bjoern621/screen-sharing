package control

import (
	"time"

	"google.golang.org/grpc"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/wire"
)

// levelCadence is how often a level stream sends.
//
// It is the interval the level elements post at rather than a rate chosen here,
// so a tick is one measurement: a faster cadence would send the same figure twice and a slower one
// would step over figures that were taken.
// One constant, read from the package that measures (docs/viewer-architecture.md).
const levelCadence = receive.LevelInterval

// SubscribeAudioLevels carries how loud every decode is, on a fixed cadence,
// for as long as the shell holds the call.
//
// A stream of its own rather than an event kind, and the difference is cadence.
// Subscribe carries whole states when something changed; a level changes continuously,
// and folding it in would push the receive state at metering rate and make every consumer of that
// state re-render for a figure none of them reads.
//
// A read and not an effect, which is why it sits on this service rather than on the frame channel.
// That channel carries frames alone; a level is not one, and being frequent is not what put
// anything there.
//
// Each tick is the whole set, read through to the backend, never a delta.
// A shell that joined late, missed a tick or fell behind is correct again on the next one.
// Nothing is queued for a slow reader either: the ticker's own channel drops the ticks a blocked
// Send ran past, so what arrives late is the present rather than a backlog of the past.
//
// It sends whether or not anything is decoding.
// An empty tick is the statement that nothing is carrying audio, which a shell needs in order to
// take a meter away; a stream that went silent instead would be indistinguishable from a backend
// that stopped answering.
func (s *Server) SubscribeAudioLevels(req *screensharev1.SubscribeAudioLevelsRequest, out grpc.ServerStreamingServer[screensharev1.AudioLevels]) error {
	assert.IsNotNil(out, "a level subscription writes to the client's stream")
	assert.Assert(levelCadence > 0, "levels tick at a positive interval", levelCadence)

	ticker := time.NewTicker(levelCadence)
	defer ticker.Stop()

	done := out.Context().Done()
	for {
		select {
		case <-done:
			// The client went away or stopped listening, which is how this call ends normally.
			// Not a failure and not reported as one, the way Subscribe treats the same event.
			return nil
		case <-ticker.C:
			if err := out.Send(wire.AudioLevels(s.backend.AudioLevels())); err != nil {
				// A failed send is the transport saying the client is gone.
				// It carries a status already, so it travels as it is.
				return err
			}
		}
	}
}
