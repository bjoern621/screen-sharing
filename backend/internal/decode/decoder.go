package decode

import (
	"bjoernblessin.de/screenshare/internal/receive"
)

// What the host drives, stated as interfaces so the contract runs without a GPU.

// Decoder is one running decode, the half of receive.Receiver the host reaches.
type Decoder interface {
	ToneMap() bool
	Stats() receive.Stats
	Audio() (volume float64, muted bool, has bool)
	Level() (peakDB, rmsDB float64, ok bool)
	SetAudio(volume float64, muted bool)
	Subscribe() Subscription
	Stop() bool
}

// Subscription is one consumer's view of a decoder's frames.
// *receive.Subscription satisfies it on the host, the client's own does in the backend, and
// control.FrameStream is the same method set again.
type Subscription interface {
	Events() <-chan receive.Event
	Err() error
	Release(generation uint64, slot int, serial uint64)
	SetRenderSize(width, height int)
	Close()
}

// Factory opens one decode.
// openReceiver is what the child runs, and a test hands its own.
type Factory func(receive.Stream, receive.Open, receive.Events) (Decoder, error)

// receiverDecoder carries a receiver's Subscribe across the interface.
// Go returns no covariant type, so the method needs a wrapper even where *receive.Subscription
// already satisfies Subscription.
type receiverDecoder struct{ *receive.Receiver }

func (r receiverDecoder) Subscribe() Subscription { return r.Receiver.Subscribe() }

// openReceiver builds a real pipeline.
func openReceiver(st receive.Stream, open receive.Open, ev receive.Events) (Decoder, error) {
	r, err := receive.New(st, open, ev)
	if err != nil {
		return nil, err
	}
	return receiverDecoder{r}, nil
}
