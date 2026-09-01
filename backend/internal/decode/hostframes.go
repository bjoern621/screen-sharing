package decode

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/logger"
)

// The two connections a decode drives rather than answers: what it reports unasked, and the frames
// it hands one consumer.

// lifecycleBuffer bounds the queue of what decodes report unasked.
//
// A decode reports its first frame once and its end once, so a backend reading its socket never
// approaches this.
// One that has stopped reading is one the queue cannot help, and the snapshot answers for it.
const lifecycleBuffer = 256

// serveLifecycle carries what decodes report unasked.
//
// One connection reads the queue.
// A backend that dialled again after losing one leaves the earlier loop to fail its next send and
// return, so a message can go to a connection nobody is reading; the snapshot carries the same
// facts, so that costs promptness alone.
func (h *Host) serveLifecycle(c *conn) {
	for msg := range h.lifecycle {
		if err := c.send(msg); err != nil {
			return
		}
	}
}

// emit queues one lifecycle message, dropping where the backend has stopped reading.
// The snapshot carries the same facts, so a drop costs promptness and never the fact.
func (h *Host) emit(msg lifecycleMessage) {
	select {
	case h.lifecycle <- msg:
	default:
		logger.Warnf("the decode host dropped a lifecycle message about %s", msg.ID)
	}
}

// serveFrames is one consumer's subscription, for as long as it holds the connection.
//
// The first message names the decode.
// Releases and render sizes then come the other way while the events go out, which is why
// the requests are read on a goroutine of their own.
func (h *Host) serveFrames(c *conn) {
	var req frameRequest
	if err := c.recv(&req); err != nil {
		return
	}
	if req.Op != frameSubscribe {
		logger.Warnf("a frame consumer opened with op %d instead of a subscribe", req.Op)
		return
	}

	h.mu.Lock()
	entry, present := h.decoders[req.ID]
	h.mu.Unlock()

	if !present || entry.ended {
		c.send(frameEvent{Err: fmt.Sprintf("nothing is decoding %s", req.ID)})
		return
	}

	sub := entry.decoder.Subscribe()
	defer sub.Close()

	go readFrameRequests(c, sub)

	for ev := range sub.Events() {
		if err := c.send(frameEvent{Pool: ev.Pool, Frame: ev.Frame}); err != nil {
			return
		}
	}
	// The producer ended the subscription, so the consumer is told on the call it is reading,
	// as receive.Receiver.endSubs does in one process.
	if err := sub.Err(); err != nil {
		c.send(frameEvent{Err: err.Error()})
	}
}

// readFrameRequests carries a consumer's releases and render sizes until the connection drops.
func readFrameRequests(c *conn, sub Subscription) {
	for {
		var req frameRequest
		if err := c.recv(&req); err != nil {
			return
		}
		switch req.Op {
		case frameRelease:
			sub.Release(req.Generation, req.Slot, req.Serial)
		case frameRenderSize:
			sub.SetRenderSize(req.Width, req.Height)
		default:
			logger.Warnf("a frame consumer sent op %d on an open subscription", req.Op)
		}
	}
}
