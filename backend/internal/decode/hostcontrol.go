package decode

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/receive"
)

// The control connection: what the backend may ask of the host, and the reads it answers.

// serveControl answers calls until the backend closes the connection, which ends the host.
// A host outliving its backend holds the GPU and the frame sockets with nothing driving it.
func (h *Host) serveControl(c *conn) {
	defer h.doneOnce.Do(func() { close(h.done) })

	for {
		var req request
		if err := c.recv(&req); err != nil {
			return
		}
		if err := c.send(h.apply(req)); err != nil {
			return
		}
	}
}

// apply is one control call, and the whole of what the backend may ask.
func (h *Host) apply(req request) response {
	switch req.Op {
	case opOpen:
		return h.applyOpen(req)
	case opStop:
		h.applyStop(req.ID)
		return response{}
	case opSetAudio:
		return h.applySetAudio(req)
	case opSnapshot:
		return response{States: h.snapshot()}
	default:
		// A request this host has no arm for is the backend and the child disagreeing about
		// the contract, which one executable cannot do.
		assert.Never("a control request names an op this host serves", req.Op)
		return response{}
	}
}

// applyOpen opens a decode, and answers what it was built with.
//
// A decode already open is success and builds nothing, whatever it was asked for: what a decode
// is built from settles at the open, and the backend changes it by stopping first.
// An entry that ended is replaced, its pipeline being gone.
func (h *Host) applyOpen(req request) response {
	h.mu.Lock()
	entry, present := h.decoders[req.ID]
	if present && !entry.ended {
		toneMap := entry.decoder.ToneMap()
		h.mu.Unlock()
		return response{ToneMap: toneMap}
	}
	h.mu.Unlock()

	id := req.ID
	decoder, err := h.factory(req.Stream, req.Open, receive.Events{
		OnLive: func() { h.emit(lifecycleMessage{ID: id, Kind: lifeLive}) },
		OnEnd:  func(message string) { h.decodeEnded(id, message) },
	})
	if err != nil {
		return response{Err: err.Error()}
	}

	h.mu.Lock()
	// A second open that raced this one wins, and this pipeline is stopped rather than left
	// with nothing pointing at it.
	if held, present := h.decoders[id]; present && !held.ended {
		h.mu.Unlock()
		decoder.Stop()
		return response{ToneMap: held.decoder.ToneMap()}
	}
	h.decoders[id] = &hosted{decoder: decoder}
	h.mu.Unlock()

	return response{ToneMap: decoder.ToneMap()}
}

// applyStop closes one decode, and succeeds where none is open.
// It collects an entry that ended by itself, which is what takes the reason out of the set.
func (h *Host) applyStop(id ID) {
	h.mu.Lock()
	entry, present := h.decoders[id]
	delete(h.decoders, id)
	h.mu.Unlock()

	if !present || entry.ended {
		return
	}
	entry.decoder.Stop()
}

// applySetAudio refuses a decode that is not open, as SetReceiveAudio does.
func (h *Host) applySetAudio(req request) response {
	h.mu.Lock()
	entry, present := h.decoders[req.ID]
	h.mu.Unlock()

	if !present || entry.ended {
		return response{Err: fmt.Sprintf("nothing is decoding %s", req.ID)}
	}
	entry.decoder.SetAudio(req.Volume, req.Muted)
	return response{}
}

// snapshot reads every decode the host holds.
// Read off the decoders on the call rather than accumulated, the rule the receive state follows.
func (h *Host) snapshot() map[ID]State {
	h.mu.Lock()
	held := make(map[ID]*hosted, len(h.decoders))
	for id, entry := range h.decoders {
		held[id] = entry
	}
	h.mu.Unlock()

	out := make(map[ID]State, len(held))
	for id, entry := range held {
		state := State{Ended: entry.ended, EndMessage: entry.endMessage}
		if !entry.ended {
			state.Stats = entry.decoder.Stats()
			state.ToneMap = entry.decoder.ToneMap()
			state.Volume, state.Muted, state.HasAudio = entry.decoder.Audio()
			state.PeakDB, state.RMSDB, state.HasLevel = entry.decoder.Level()
			state.Pointer, state.HasPointer = entry.decoder.Pointer()
		}
		out[id] = state
	}
	return out
}

// decodeEnded records a pipeline that stopped by itself and reports it.
// The entry stays until the backend stops it, carrying the reason.
func (h *Host) decodeEnded(id ID, message string) {
	h.mu.Lock()
	entry, present := h.decoders[id]
	if present {
		entry.ended, entry.endMessage = true, message
	}
	h.mu.Unlock()

	if !present {
		return
	}
	h.emit(lifecycleMessage{ID: id, Kind: lifeEnd, Message: message})
}
