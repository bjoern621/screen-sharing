package wire

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/receive"
)

// The frame channel's shapes on the contract.
//
// It is the same job the rest of this package does for the control surface:
// the receive package says what a pool and a frame are in its own terms, and this is where those
// terms become the message.
// Nothing here decides anything - a handle kind that has no contract value is a kind this build
// declared and did not carry, which is a broken internal contract rather than a condition to
// survive, so it asserts.

// FrameSourceKind names which of the three pictures a subscription draws from.
//
// It is an explicit discriminator rather than a nil pointer, which is what the two-armed version of
// this type used.
// Two arms can be told apart by whether the one that carries a key has it; three cannot,
// because the arm that carries nothing and the zero value would be the same value,
// and "the caller named nothing" and "the caller named the publish preview" are the difference
// between a refusal and a subscription.
type FrameSourceKind int

const (
	// FrameSourceRelay is a decode of one stream on one leg, which StartReceive opened.
	FrameSourceRelay FrameSourceKind = iota + 1
	// FrameSourcePublishPreview is the running publish's local decode of itself.
	FrameSourcePublishPreview
	// FrameSourceMonitorPreview is one of this machine's screens, read live.
	FrameSourceMonitorPreview
)

// FrameSource is what one frame subscription draws from.
//
// It is the domain side of screenshare.v1.FrameSubscribe, and each arm carries exactly what tells
// one of its own kind apart.
// A relay decode carries the pair that identifies it, because the relay re-serves each stream on
// all its listeners and one stream can be decoded over several protocols at once.
// The publish preview carries nothing, because at most one publish runs and the preview is part of
// it.
// A monitor preview carries the output's index, because a machine has as many screens as it has
// outputs.
type FrameSource struct {
	Kind FrameSourceKind
	// Stream identifies the relay decode, and is meaningful on FrameSourceRelay alone.
	Stream WatchKey
	// Monitor is the previewed output's index, meaningful on FrameSourceMonitorPreview alone.
	Monitor int
}

// FrameSourceOf reads what a subscription named back off the contract, and false where it named
// none of the three - a request the control service refuses with INVALID_ARGUMENT rather than
// guessing at.
//
// A relay decode with half a key is left as it arrived rather than rejected here:
// which half is missing is a sentence the service writes, and this is the shape it reads to write
// it.
func FrameSourceOf(m *screensharev1.FrameSubscribe) (FrameSource, bool) {
	switch {
	case m.GetStream() != nil:
		return FrameSource{Kind: FrameSourceRelay, Stream: WatchKeyOf(m.GetStream())}, true
	case m.GetPublishPreview() != nil:
		return FrameSource{Kind: FrameSourcePublishPreview}, true
	case m.GetMonitorPreview() != nil:
		return FrameSource{
			Kind:    FrameSourceMonitorPreview,
			Monitor: int(m.GetMonitorPreview().GetMonitor()),
		}, true
	default:
		return FrameSource{}, false
	}
}

// FrameEventOf is one thing a subscription said, as the message that carries it.
func FrameEventOf(event receive.Event) *screensharev1.FrameEvent {
	switch {
	case event.Pool != nil:
		return &screensharev1.FrameEvent{
			Event: &screensharev1.FrameEvent_Pool{Pool: framePoolOf(*event.Pool)},
		}
	case event.Frame != nil:
		return &screensharev1.FrameEvent{
			Event: &screensharev1.FrameEvent_Ready{Ready: frameReadyOf(*event.Frame)},
		}
	default:
		return nil
	}
}

// FrameEndOf is the sentence a subscription ended with, as the message that carries it.
func FrameEndOf(message string) *screensharev1.FrameEvent {
	return &screensharev1.FrameEvent{
		Event: &screensharev1.FrameEvent_End{End: &screensharev1.FrameEnd{Message: message}},
	}
}

// framePoolOf is one lent pool, generation included: the consumer needs it to tell a
// re-announcement from a repeat, and the backend needs it back on every release.
func framePoolOf(pool receive.Pool) *screensharev1.FramePool {
	slots := make([]*screensharev1.FrameSlot, 0, len(pool.Slots))
	for _, slot := range pool.Slots {
		planes := make([]*screensharev1.FramePlane, 0, len(slot.Planes))
		for _, plane := range slot.Planes {
			planes = append(planes, &screensharev1.FramePlane{
				Offset: plane.Offset,
				Stride: plane.Stride,
			})
		}
		slots = append(slots, &screensharev1.FrameSlot{
			Index:  uint32(slot.Index),
			Handle: slot.Handle,
			Planes: planes,
		})
	}

	return &screensharev1.FramePool{
		Generation:    pool.Generation,
		HandleType:    frameHandleTypeOf(pool.Kind),
		Format:        frameFormatOf(pool.Format),
		Width:         uint32(pool.Width),
		Height:        uint32(pool.Height),
		MemorySize:    pool.MemorySize,
		TopLeftOrigin: pool.TopLeftOrigin,
		Slots:         slots,
		ProducerKey:   pool.ProducerKey,
		ConsumerKey:   pool.ConsumerKey,
		FdSocket:      pool.FDSocket,
		Modifier:      pool.Modifier,
	}
}

func frameReadyOf(frame receive.Frame) *screensharev1.FrameReady {
	return &screensharev1.FrameReady{
		Generation: frame.Generation,
		Slot:       uint32(frame.Slot),
		Serial:     frame.Serial,
		Dropped:    frame.Dropped,
	}
}

// frameHandleTypeOf is the contract's value for a handle kind.
//
// An unlisted kind is unspecified rather than a panic, because a consumer that is told nothing
// about the handle refuses to import it, which is the same outcome one step earlier and without
// taking the backend down with it.
func frameHandleTypeOf(kind receive.HandleKind) screensharev1.FrameHandleType {
	switch kind {
	case receive.HandleD3D11GlobalShared:
		return screensharev1.FrameHandleType_FRAME_HANDLE_TYPE_D3D11_GLOBAL_SHARED
	case receive.HandleDMABufFD:
		return screensharev1.FrameHandleType_FRAME_HANDLE_TYPE_DMABUF_FD
	default:
		return screensharev1.FrameHandleType_FRAME_HANDLE_TYPE_UNSPECIFIED
	}
}

func frameFormatOf(format receive.ShareFormat) screensharev1.FrameFormat {
	switch format {
	case receive.ShareFormatRGBA:
		return screensharev1.FrameFormat_FRAME_FORMAT_R8G8B8A8_UNORM
	case receive.ShareFormatBGRA:
		return screensharev1.FrameFormat_FRAME_FORMAT_B8G8R8A8_UNORM
	default:
		return screensharev1.FrameFormat_FRAME_FORMAT_UNSPECIFIED
	}
}
