package app

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/receive"
)

// SubscribeFrames opens one consumer's view of a running decode's frames.
//
// It opens no decode.
// StartReceive is the effect that does, and a subscription that started one would be the frame
// channel deciding that a stream should be received - which is a tile's decision wearing a
// transport's name, and the distinction the whole contract turns on (docs/ipc-api.md).
//
// A stream nothing is decoding is therefore a refusal rather than a wait.
// The shell asks for the decode first and subscribes once that call has answered,
// and the two staying separate is what lets a decode outlive every window drawing it:
// a shell that closed does not stop a stream, and a shell that opened again finds it running.
//
// Several consumers may subscribe to one decode, and each gets a pool of its own.
// Two tiles drawing one stream is two copies on the GPU rather than one buffer with two owners,
// which is what keeps a slow consumer from holding a slot the other one needs.
func (a *App) SubscribeFrames(streamName, transportName string) (*receive.Subscription, error) {
	assert.Assert(streamName != "", "a frame consumer names the stream it draws")
	assert.Assert(transportName != "", "a frame consumer names the leg the stream is decoded from", streamName)

	a.procMu.Lock()
	receiver, present := a.receivers[WatchKey{Name: streamName, Transport: transportName}]
	a.procMu.Unlock()

	if !present {
		return nil, fmt.Errorf("nothing is decoding %s over %s", streamName, transportName)
	}
	return receiver.Subscribe(), nil
}
