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
// The interval the level elements post at rather than a rate chosen here, so one tick carries one
// measurement: a faster cadence would send a figure twice and a slower one would step over figures
// that were taken.
// One constant, read from the package that measures (docs/viewer-architecture.md).
const levelCadence = receive.LevelInterval

// SubscribeAudioLevels carries how loud every decode is, on a fixed cadence, for as long as the
// shell holds the call.
// A figure is dBFS: at most zero, and silence negative infinity.
//
// A stream of its own rather than an event kind, and the difference is cadence.
// Subscribe carries whole states when something changed; a level changes continuously, so folding
// it in would push the receive state at metering rate and re-render every consumer of that state
// for a figure none of them reads.
//
// A read rather than an effect, which is why it sits on this service and not on the frame channel.
// That channel carries frames alone, a level is not one, and frequency is not what put anything
// there.
//
// Each tick is the whole set, read through to the backend, never a delta.
// A shell that joined late, missed a tick or fell behind is correct again on the next one.
// Nothing is queued for a slow reader: the ticker's channel drops the ticks a blocked Send ran
// past, so what arrives late is the present rather than a backlog of the past.
//
// It sends whether or not anything is decoding.
// An empty tick states that nothing is carrying audio, which is what lets a shell take a meter
// away; going silent instead would be indistinguishable from a backend that stopped answering.
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
			// Not a failure and not reported as one, as Subscribe treats the same event.
			return nil
		case <-ticker.C:
			if err := out.Send(wire.AudioLevels(s.backend.AudioLevels())); err != nil {
				// A failed send is the transport saying the client is gone.
				// It carries a status already, so it travels unchanged.
				return err
			}
		}
	}
}
