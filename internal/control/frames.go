package control

import (
	"errors"
	"io"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/wire"
)

// The frame channel: the second service on the same socket, carrying handles and never
// pixels (docs/ipc-api.md, "What crosses, and in what shape").
//
// It is a separate server type rather than more methods on Server, because the two
// services answer to different rules. Every ControlService method is a read, one of the
// named effects, or the event stream; this one is a loan of memory with a lifetime, and
// putting it on the same type would make that division a comment rather than a shape.
//
// Nothing here decides what is drawn. A subscription names a decode StartReceive
// already opened, and the size it carries is a count of pixels rather than a layout.

// FrameServer is the FrameService implementation.
//
// Like Server it holds no state of its own: a subscription's whole state lives in the
// backend that lent it, and this is the pipe between that and one gRPC call.
type FrameServer struct {
	screensharev1.UnimplementedFrameServiceServer

	backend Backend
}

// NewFrames returns a frame service in front of backend.
func NewFrames(backend Backend) *FrameServer {
	assert.IsNotNil(backend, "a frame service serves a backend")
	return &FrameServer{backend: backend}
}

// Frames is one consumer's subscription to one decode.
//
// The call's shape is what the protocol needs rather than a convenience: the loan and
// the release have to travel together, because a release on a second call could outlive
// the subscription it belongs to and would free a slot of a pool that is gone.
//
// The two directions run on two goroutines, which is what gRPC's bidirectional streams
// require: Send is this one's alone and Recv is the reader's, and neither is safe to
// share. What joins them is the subscription, whose own methods take its lock.
func (s *FrameServer) Frames(stream screensharev1.FrameService_FramesServer) error {
	first, err := stream.Recv()
	if err != nil {
		// A consumer that hung up before saying what it wanted is not an error worth
		// reporting: it is a window that closed between opening the call and filling it.
		return nil
	}

	subscribe := first.GetSubscribe()
	if subscribe == nil {
		return invalidArgument("the first message on a frame stream says which decode it is for")
	}
	key := wire.WatchKeyOf(subscribe.GetStream())
	if key.StreamName == "" {
		return invalidArgument("no stream named to receive frames of")
	}
	if key.Transport == "" {
		return invalidArgument("no transport named to receive frames of '%s' over", key.StreamName)
	}

	frames, err := s.backend.SubscribeFrames(key)
	if err != nil {
		// The world is not ready rather than the request being malformed: the decode is
		// opened by StartReceive, and a shell that asked for one and lost the race asks
		// again rather than being told it named something impossible.
		return failedPrecondition("cannot draw '%s' over %s: %v", key.StreamName, key.Transport, err)
	}
	// Closed on every way out, including the ones that never reach the loop below. It is
	// what frees the pool, so a consumer that died is a consumer whose GPU memory comes
	// back rather than one that leaks it for the life of the stream.
	defer frames.Close()

	logger.Debugf("control: drawing '%s' over %s", key.StreamName, key.Transport)

	hungUp := make(chan error, 1)
	go func() { hungUp <- readRequests(stream, frames) }()

	for {
		select {
		case event, open := <-frames.Events():
			if !open {
				return endOf(stream, frames.Err())
			}
			message := wire.FrameEventOf(event)
			if message == nil {
				continue
			}
			if err := stream.Send(message); err != nil {
				return err
			}
		case err := <-hungUp:
			if err == nil || errors.Is(err, io.EOF) {
				// The consumer closed its half, which is what a tile that went away does.
				return nil
			}
			return err
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// readRequests drives the consumer's half of the call until it ends.
//
// A message that is not one of the three is dropped rather than refused. The contract
// is additive, so a consumer built against a later minor may say something this build
// has no case for, and ending its stream over that would turn a forward-compatible
// addition into a broken tile.
func readRequests(stream screensharev1.FrameService_FramesServer, frames FrameStream) error {
	for {
		request, err := stream.Recv()
		if err != nil {
			return err
		}

		switch {
		case request.GetRelease() != nil:
			release := request.GetRelease()
			frames.Release(release.GetGeneration(), int(release.GetSlot()), release.GetSerial())
		case request.GetRenderSize() != nil:
			size := request.GetRenderSize()
			frames.SetRenderSize(int(size.GetWidth()), int(size.GetHeight()))
		}
	}
}

// endOf says why the frames stopped, on the call the consumer is reading.
//
// The call itself succeeds. A pipeline that ended is not a failed request - the
// subscription did what it was for and then the stream it drew from was over - and the
// same fact reaches every shell as ReceiveExit on the event stream.
func endOf(stream screensharev1.FrameService_FramesServer, err error) error {
	if err == nil {
		return nil
	}
	if sendErr := stream.Send(wire.FrameEndOf(err.Error())); sendErr != nil {
		return sendErr
	}
	return nil
}
