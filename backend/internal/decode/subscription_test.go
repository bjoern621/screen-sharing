package decode

import (
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/receive"
)

// The frame channel across the process boundary: what a pool carries, and how a subscription ends.

func TestFramesAndReleasesCross(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")
	h.open(t, id, "desk", Events{})

	sub, err := h.client.Subscribe(id)
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Close()

	hosted := h.decoder(t, "desk").subscription(t)
	hosted.events <- receive.Event{Pool: &receive.Pool{
		Generation: 1,
		Kind:       receive.HandleDMABufFD,
		Format:     receive.ShareFormatRGBA,
		Width:      1920,
		Height:     1080,
		FDSocket:   "/run/user/1000/pool.sock",
		Slots:      []receive.Slot{{Index: 0}, {Index: 1}, {Index: 2}},
	}}
	hosted.events <- receive.Event{Frame: &receive.Frame{Generation: 1, Slot: 2, Serial: 7, Dropped: 1}}

	// The socket the consumer dials for the descriptors is the whole of what a pool carries about
	// where the memory is, so a pool that lost it is a consumer with nothing to import.
	pool := nextEvent(t, sub)
	if pool.Pool == nil {
		t.Fatal("the first event carried no pool")
	}
	if pool.Pool.FDSocket != "/run/user/1000/pool.sock" || pool.Pool.Width != 1920 {
		t.Errorf("the pool crossed as %+v", pool.Pool)
	}
	if len(pool.Pool.Slots) != 3 {
		t.Errorf("the pool crossed with %d slots", len(pool.Pool.Slots))
	}

	frame := nextEvent(t, sub)
	if frame.Frame == nil {
		t.Fatal("the second event carried no frame")
	}
	if frame.Frame.Slot != 2 || frame.Frame.Serial != 7 || frame.Frame.Dropped != 1 {
		t.Errorf("the frame crossed as %+v", frame.Frame)
	}

	sub.Release(1, 2, 7)
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if back := hosted.releases(); len(back) == 1 {
			if back[0] != (released{1, 2, 7}) {
				t.Errorf("the release reached the host as %+v", back[0])
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("a released slot never reached the host")
}

func TestSubscribingToADecodeThatIsNotOpenFails(t *testing.T) {
	h := newHarness(t)

	sub, err := h.client.Subscribe(StreamID("desk", "rtsp"))
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Close()

	if _, open := <-sub.Events(); open {
		t.Fatal("subscribing to a decode nothing is running handed over a frame")
	}
	if sub.Err() == nil {
		t.Error("a subscription that found no decode ended without a reason")
	}
}

// A pipeline that ended tells the consumer on the call it is reading, as it does in one process.
func TestAPipelineThatEndsTellsItsConsumer(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")
	h.open(t, id, "desk", Events{})

	sub, err := h.client.Subscribe(id)
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Close()

	h.decoder(t, "desk").subscription(t).end("the decoder gave up")

	select {
	case _, open := <-sub.Events():
		if open {
			t.Fatal("a subscription of a pipeline that ended handed over a frame")
		}
	case <-time.After(wait):
		t.Fatal("a subscription of a pipeline that ended never closed")
	}
	if err := sub.Err(); err == nil || err.Error() != "the decoder gave up" {
		t.Errorf("the subscription ended with %v", err)
	}
}

// The failure the process split exists for: the host aborts and every consumer is told, rather than
// the backend going down with it.
func TestAHostThatDiesEndsEverySubscription(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")
	h.open(t, id, "desk", Events{})

	sub, err := h.client.Subscribe(id)
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Close()

	// Waiting for the host to have subscribed keeps the shutdown from racing the handshake.
	h.decoder(t, "desk").subscription(t)
	// What a host that aborted does to its consumers, which is every descriptor closing at once.
	h.listener.Close()
	h.host.Close()

	select {
	case _, open := <-sub.Events():
		if open {
			t.Fatal("a subscription outlived the host it was reading")
		}
	case <-time.After(wait):
		t.Fatal("a subscription of a host that went never ended")
	}
	if sub.Err() == nil {
		t.Error("a subscription ended by its host going carried no reason")
	}
}
