package wire

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/receive"
)

// The frame channel's shapes on the contract.
//
// The same job the rest of this package does for the control surface: the receive package says what
// a pool and a frame are in its own terms, and this is where those terms become the message.
// Nothing here decides anything beyond the shape.

// FrameSourceKind names which of the three pictures a subscription draws from.
//
// An explicit discriminator rather than a nil pointer: with three arms, the one that carries
// nothing and the zero value would be one value, and "the caller named nothing" and "the caller
// named the publish preview" are the difference between a refusal and a subscription.
type FrameSourceKind int

const (
	// FrameSourceRelay is a decode of one stream on one leg, opened by StartReceive.
	FrameSourceRelay FrameSourceKind = iota + 1
	// FrameSourcePublishPreview is the publish's own local decode, opened by the publish itself.
	FrameSourcePublishPreview
	// FrameSourceMonitorPreview is one of this machine's screens, opened by StartMonitorPreview.
	FrameSourceMonitorPreview
)

// FrameSource is what one frame subscription draws from.
//
// The domain side of screenshare.v1.FrameSubscribe, each arm carrying exactly what tells one of its
// own kind apart.
// A relay decode carries the pair that identifies it, the relay re-serving each stream on all its
// listeners and one stream being decodable over several protocols at once.
// The publish preview carries nothing, at most one publish running and the preview being part of
// it.
// A monitor preview carries the output's index, a machine having as many screens as outputs.
type FrameSource struct {
	Kind FrameSourceKind
	// Stream identifies the relay decode, read on FrameSourceRelay alone.
	Stream WatchKey
	// Monitor is the previewed output's index, read on FrameSourceMonitorPreview alone.
	Monitor int
}

// FrameSourceOf reads what a subscription named back off the contract, and false where it named
// none of the three, which the control service refuses with INVALID_ARGUMENT rather than guess at.
//
// A relay decode with half a key is left as it arrived rather than rejected here: which half is
// missing is a sentence the service writes, and this is the shape it reads to write it.
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

// FrameEventOf is one thing a subscription said, as the message carrying it, and nil for an event
// that holds neither a pool nor a frame.
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

// FrameEndOf is the sentence a subscription ended on.
func FrameEndOf(message string) *screensharev1.FrameEvent {
	return &screensharev1.FrameEvent{
		Event: &screensharev1.FrameEvent_End{End: &screensharev1.FrameEnd{Message: message}},
	}
}

// framePoolOf is one lent pool, generation included: the consumer tells a re-announcement from a
// repeat by it, and every release is checked against it.
// Slot indices are the pool's own and start again on the next generation.
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
// An unlisted kind crosses as unspecified rather than panicking: a consumer told nothing about the
// handle refuses to import it, which is the same outcome one step earlier and without taking the
// backend down with it.
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
