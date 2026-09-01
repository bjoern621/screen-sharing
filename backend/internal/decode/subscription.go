package decode

import (
	"errors"
	"sync"

	"bjoernblessin.de/screenshare/internal/receive"
)

// One consumer's subscription, as the backend sees it.
// A pool's descriptors never come this way: the pool names a socket of its own, which the consumer
// dials on the host directly (receive/descriptors_linux.go).

// eventBuffer bounds the queue this side builds up.
//
// A consumer slower than the decode fills it, and the reader then waits rather than dropping:
// a pool announcement lost that way would leave the consumer with frames and nowhere to import
// them from.
// The wait reaches the host through the socket, where its own subscription queue fills and
// the decode drops the frames it has nowhere to put, which is the backpressure one process has.
const eventBuffer = 8

// Subscribe opens one consumer's view of a decode's frames, on a connection of its own.
//
// It opens no decode and starts no host: a subscription draws from a decode already running, and
// nothing is decoding where no host is (docs/ipc-api.md).
func (c *Client) Subscribe(id ID) (Subscription, error) {
	conn, err := c.dial(connFrames)
	if err != nil {
		return nil, err
	}
	if err := conn.send(frameRequest{Op: frameSubscribe, ID: id}); err != nil {
		conn.Close()
		return nil, err
	}

	sub := &remoteSub{
		conn:   conn,
		events: make(chan receive.Event, eventBuffer),
		done:   make(chan struct{}),
	}
	go sub.read()
	return sub, nil
}

// remoteSub is one subscription on the host, as a consumer in this process sees it.
type remoteSub struct {
	conn   *conn
	events chan receive.Event
	// done closes with the events channel and unblocks a handover nobody is taking.
	done chan struct{}

	mu    sync.Mutex
	err   error
	ended bool
}

func (s *remoteSub) Events() <-chan receive.Event { return s.events }

func (s *remoteSub) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Release hands one slot back to the host.
// A send that fails is a subscription already over, which the reader reports.
func (s *remoteSub) Release(generation uint64, slot int, serial uint64) {
	s.conn.send(frameRequest{Op: frameRelease, Generation: generation, Slot: slot, Serial: serial})
}

func (s *remoteSub) SetRenderSize(width, height int) {
	s.conn.send(frameRequest{Op: frameRenderSize, Width: width, Height: height})
}

// Close ends the subscription from the consumer's side.
// Idempotent, and what a dropped shell connection runs.
//
// It ends the channel as well as the connection: a reader waiting to hand an event over has nobody
// left to hand it to.
func (s *remoteSub) Close() {
	s.finish(nil)
	s.conn.Close()
}

// read carries the host's events onto the channel, and ends the subscription with its reason.
func (s *remoteSub) read() {
	defer s.conn.Close()

	for {
		var ev frameEvent
		if err := s.conn.recv(&ev); err != nil {
			s.finish(errors.New("the decoding process stopped"))
			return
		}
		if ev.Err != "" {
			s.finish(errors.New(ev.Err))
			return
		}
		select {
		case s.events <- receive.Event{Pool: ev.Pool, Frame: ev.Frame}:
		case <-s.done:
			return
		}
	}
}

// finish ends the subscription once, with its reason.
func (s *remoteSub) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended {
		return
	}
	s.ended = true
	s.err = err
	close(s.done)
	close(s.events)
}
