package app

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/receive"
)

// SubscribeFrames opens one consumer's view of a running decode's frames.
//
// It opens no decode.
// StartReceive is the effect that does,
// and a subscription starting one would be the frame channel deciding that a stream be received,
// a tile's decision wearing a transport's name,
// and the distinction the whole contract turns on (docs/ipc-api.md).
// A stream nothing is decoding is refused rather than waited for.
//
// The shell asks for the decode first and subscribes once that call has answered.
// The two staying apart is what lets a decode outlive every window drawing it:
// a shell that closed stops no stream, and one that opened again finds it running.
//
// Several consumers may subscribe to one decode, and each gets a pool of its own.
// Two tiles on one stream are two copies on the GPU rather than one buffer with two owners,
// which keeps a slow consumer from holding a slot the other one needs.
func (a *App) SubscribeFrames(streamName, transportName string) (*receive.Subscription, error) {
	assert.Assert(streamName != "", "a frame consumer names the stream it draws")
	assert.Assert(transportName != "", "a frame consumer names the leg the stream is decoded from", streamName)

	a.procMu.Lock()
	receiver, present := a.receivers[StreamRef{Name: streamName, Transport: transportName}]
	a.procMu.Unlock()

	if !present {
		return nil, fmt.Errorf("nothing is decoding %s over %s", streamName, transportName)
	}
	return receiver.Subscribe(), nil
}
