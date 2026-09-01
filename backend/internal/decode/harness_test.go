package decode

import (
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/receive"
)

// A host and a client talking over a socket, with no child process between them.

type harness struct {
	host     *Host
	client   *Client
	listener net.Listener

	// opened records every decode the factory built, keyed by the stream's name.
	mu     sync.Mutex
	opened map[string]*fakeDecoder
}

// refusedStream is the name the factory refuses, so a test can exercise the refusal crossing.
const refusedStream = "refused"

func newHarness(t *testing.T) *harness {
	t.Helper()

	socket := filepath.Join(t.TempDir(), "host.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("cannot listen on %s: %v", socket, err)
	}

	h := &harness{listener: listener, opened: map[string]*fakeDecoder{}}
	h.host = NewHost(h.build)
	go h.host.Serve(listener)

	// The socket is set rather than spawned for: the contract under test is what crosses it, and
	// a real child would need a GPU to open anything on.
	h.client = &Client{
		dir:    t.TempDir(),
		socket: socket,
		events: map[ID]Events{},
	}
	ctrl, err := h.client.dial(connControl)
	if err != nil {
		t.Fatalf("cannot open the control connection: %v", err)
	}
	h.client.ctrl = ctrl

	lifecycle, err := h.client.dial(connLifecycle)
	if err != nil {
		t.Fatalf("cannot open the lifecycle connection: %v", err)
	}
	go h.client.readLifecycle(lifecycle)

	t.Cleanup(func() {
		ctrl.Close()
		listener.Close()
		h.host.Close()
	})
	return h
}

// build is the harness's factory, standing in for a real pipeline.
func (h *harness) build(st receive.Stream, open receive.Open, ev receive.Events) (Decoder, error) {
	if st.Name == refusedStream {
		return nil, errors.New("this stream carries no picture")
	}

	d := &fakeDecoder{toneMap: open.ToneMap, onEnd: ev.OnEnd}
	d.stats.Decoder = "fakedec"
	d.stats.Frames = 1

	h.mu.Lock()
	h.opened[st.Name] = d
	h.mu.Unlock()

	if ev.OnLive != nil {
		ev.OnLive()
	}
	return d, nil
}

// decoder is the pipeline opened for one stream, failing rather than returning nil.
func (h *harness) decoder(t *testing.T, name string) *fakeDecoder {
	t.Helper()

	h.mu.Lock()
	defer h.mu.Unlock()

	d, present := h.opened[name]
	if !present {
		t.Fatalf("no decode was opened for %q", name)
	}
	return d
}

// subscription waits for the host to have subscribed to the decoder.
func (d *fakeDecoder) subscription(t *testing.T) *fakeSub {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		var sub *fakeSub
		if len(d.subs) > 0 {
			sub = d.subs[0]
		}
		d.mu.Unlock()

		if sub != nil {
			return sub
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the host opened no subscription on the decoder")
	return nil
}

func streamOf(name string) receive.Stream {
	return receive.Stream{Name: name, Transport: "rtsp", Source: "fakesrc"}
}

// open opens one decode and fails the test where it did not.
func (h *harness) open(t *testing.T, id ID, name string, ev Events) *Handle {
	t.Helper()

	handle, err := h.client.Open(id, streamOf(name), receive.Open{}, ev)
	if err != nil {
		t.Fatalf("opening the decode of %s: %v", id, err)
	}
	return handle
}

// nextEvent takes one event off a subscription, failing rather than hanging.
func nextEvent(t *testing.T, sub Subscription) receive.Event {
	t.Helper()

	select {
	case ev, open := <-sub.Events():
		if !open {
			t.Fatalf("the subscription ended: %v", sub.Err())
		}
		return ev
	case <-time.After(wait):
		t.Fatal("no event arrived")
		return receive.Event{}
	}
}
