package control

import (
	"errors"
	"fmt"
	"io"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/wire"
)

// The frame channel: the second service on the same socket, carrying handles and never pixels
// (docs/ipc-api.md, "What crosses, and in what shape").
//
// A server type of its own rather than more methods on Server, because the two answer to different
// rules: every ControlService method is a read, a named effect or the event stream, and this one is
// a loan of memory with a lifetime.
// One type would leave that division a comment rather than a shape.
//
// Nothing here decides what is drawn.
// A subscription names a picture something else opened, and the size it carries is a count of
// pixels rather than a layout.

// FrameServer is the FrameService implementation.
//
// It holds no state of its own, as Server holds none: a subscription's whole state lives in the
// backend that lent it, and this is the pipe between that and one gRPC call.
type FrameServer struct {
	screensharev1.UnimplementedFrameServiceServer

	backend Backend
}

func NewFrames(backend Backend) *FrameServer {
	assert.IsNotNil(backend, "a frame service serves a backend")
	return &FrameServer{backend: backend}
}

// Frames is one consumer's subscription to one picture.
// The first message names which, and one naming none is INVALID_ARGUMENT.
//
// Bidirectional because the protocol needs it: the loan and the release travel on one call, since a
// release arriving on a second one could outlive the subscription it belongs to and free a slot of
// a pool that is gone.
//
// The two directions run on two goroutines, as gRPC's bidirectional streams require.
// Send belongs to this goroutine and Recv to readRequests, and neither is safe to share.
// The subscription joins them, and its own methods take its lock.
func (s *FrameServer) Frames(stream screensharev1.FrameService_FramesServer) error {
	first, err := stream.Recv()
	if err != nil {
		// A consumer that hung up before saying what it wanted is a window that closed between opening
		// the call and filling it, not a failure to report.
		return nil
	}

	subscribe := first.GetSubscribe()
	if subscribe == nil {
		return invalidArgument("the first message on a frame stream says which decode it is for")
	}
	source, named := wire.FrameSourceOf(subscribe)
	if !named {
		return invalidArgument("a frame stream draws from a relay decode, the publish preview or a monitor preview, and this one names none of them")
	}
	// A relay decode is named by a pair, and half of one names nothing that could exist.
	// The other two need no such check: the publish preview is named by the choice itself,
	// which is the point of its having no key, and a monitor index reaching no output is refused
	// where the preview is looked for, since whether an index is one of this machine's is a fact
	// about the machine rather than about the request.
	if source.Kind == wire.FrameSourceRelay {
		if source.Stream.StreamName == "" {
			return invalidArgument("no stream named to receive frames of")
		}
		if source.Stream.Transport == "" {
			return invalidArgument("no transport named to receive frames of '%s' over", source.Stream.StreamName)
		}
	}

	frames, err := s.subscribe(source)
	if err != nil {
		// The world is not ready rather than the request being malformed: a relay decode is opened by
		// StartReceive and the publish preview by the publish itself, so a shell that asked for either
		// and lost the race asks again instead of being told it named something impossible.
		return failedPrecondition("cannot draw %s: %v", describe(source), err)
	}
	// Closed on every way out, the ones that never reach the loop below included.
	// The close frees the pool, so a consumer that died gives its GPU memory back rather than holding
	// it for the life of the stream.
	defer frames.Close()

	logger.Debugf("control: drawing %s", describe(source))

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
				// The consumer closed its half, as a tile that went away does.
				return nil
			}
			return err
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// subscribe opens what the request named.
//
// The kinds are dispatched here rather than by the backend, being different questions: which decode
// of several, the one preview a publish can have, and which screen.
// A single method taking a key would need a key the previews do not have.
func (s *FrameServer) subscribe(source wire.FrameSource) (FrameStream, error) {
	switch source.Kind {
	case wire.FrameSourcePublishPreview:
		return s.backend.SubscribePreviewFrames()
	case wire.FrameSourceMonitorPreview:
		return s.backend.SubscribeMonitorFrames(source.Monitor)
	default:
		return s.backend.SubscribeFrames(source.Stream)
	}
}

// describe names what a subscription draws from, for a log line and for the refusal a consumer
// reads.
// Written once because both read it, and because the previews have no pair to print.
func describe(source wire.FrameSource) string {
	switch source.Kind {
	case wire.FrameSourcePublishPreview:
		return "the local preview of the running stream"
	case wire.FrameSourceMonitorPreview:
		return fmt.Sprintf("monitor %d", source.Monitor)
	default:
		return fmt.Sprintf("'%s' over %s", source.Stream.StreamName, source.Stream.Transport)
	}
}

// readRequests drives the consumer's half of the call until it ends.
//
// A message no case here covers is dropped rather than refused.
// The contract is additive, so a consumer built against a later minor may send something this build
// does not know, and ending its stream over that would turn a compatible addition into a broken
// tile.
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

// endOf says why the frames stopped, on the call the consumer is reading, and ends it successfully.
//
// A pipeline that ended is not a failed request: the subscription did what it was for and the
// stream it drew from was then over.
// The same fact reaches every shell as ReceiveExit on the event stream.
func endOf(stream screensharev1.FrameService_FramesServer, err error) error {
	if err == nil {
		return nil
	}
	if sendErr := stream.Send(wire.FrameEndOf(err.Error())); sendErr != nil {
		return sendErr
	}
	return nil
}
