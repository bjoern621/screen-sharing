package decode

import (
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/receive"
)

// What the backend may ask of the host, and what a read answers.

func TestOpenReportsWhatItBuilt(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")

	handle, err := h.client.Open(id, streamOf("desk"), receive.Open{ToneMap: true}, Events{})
	if err != nil {
		t.Fatalf("opening a decode: %v", err)
	}
	if !handle.ToneMap() {
		t.Error("a decode built with the tone-mapping rung reports it was not")
	}

	states := h.client.Snapshot()
	state, present := states[id]
	if !present {
		t.Fatalf("the snapshot holds %d decode(s) and not %s", len(states), id)
	}
	if state.Stats.Decoder != "fakedec" {
		t.Errorf("the snapshot reports decoder %q", state.Stats.Decoder)
	}
	if state.Ended {
		t.Error("a running decode reports that it ended")
	}
}

func TestOpenTwiceBuildsOne(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")

	h.open(t, id, "desk", Events{})
	first := h.decoder(t, "desk")

	h.open(t, id, "desk", Events{})
	if second := h.decoder(t, "desk"); second != first {
		t.Error("opening one decode twice built two pipelines")
	}
	if states := h.client.Snapshot(); len(states) != 1 {
		t.Errorf("the host holds %d decodes after two opens of one", len(states))
	}
}

func TestOpenCarriesTheRefusal(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.Open(StreamID(refusedStream, "rtsp"), streamOf(refusedStream), receive.Open{}, Events{})
	if err == nil {
		t.Fatal("a decode the host refused opened")
	}
	if err.Error() != "this stream carries no picture" {
		t.Errorf("the refusal reached the backend as %q", err)
	}
}

func TestStopIsRepeatable(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")

	h.open(t, id, "desk", Events{})
	h.client.Stop(id)

	if states := h.client.Snapshot(); len(states) != 0 {
		t.Errorf("the host holds %d decodes after the one it had was stopped", len(states))
	}
	if !h.decoder(t, "desk").wasStopped() {
		t.Error("stopping a decode left its pipeline running")
	}

	// The state the call names already holds, so the repeat is success.
	h.client.Stop(id)
}

func TestSetAudioRefusesADecodeThatIsNotOpen(t *testing.T) {
	h := newHarness(t)

	if err := h.client.SetAudio(StreamID("desk", "rtsp"), 0.5, false); err == nil {
		t.Fatal("setting the loudness of a decode nothing is running succeeded")
	}
}

func TestSetAudioReachesTheDecoder(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")

	h.open(t, id, "desk", Events{})
	if err := h.client.SetAudio(id, 0.25, true); err != nil {
		t.Fatalf("setting the loudness: %v", err)
	}

	state := h.client.Snapshot()[id]
	if state.Volume != 0.25 || !state.Muted {
		t.Errorf("the decode plays at %v muted=%v", state.Volume, state.Muted)
	}
}

func TestADecodeThatEndsKeepsItsReasonUntilItIsStopped(t *testing.T) {
	h := newHarness(t)
	id := StreamID("desk", "rtsp")

	ended := make(chan string, 1)
	h.open(t, id, "desk", Events{OnEnd: func(message string) { ended <- message }})

	h.decoder(t, "desk").onEnd("the relay stopped carrying it")

	select {
	case message := <-ended:
		if message != "the relay stopped carrying it" {
			t.Errorf("the end reached the backend as %q", message)
		}
	case <-time.After(wait):
		t.Fatal("a decode that ended reported nothing to the backend")
	}

	state, present := h.client.Snapshot()[id]
	if !present {
		t.Fatal("a decode that ended left the host before it was stopped, taking its reason with it")
	}
	if !state.Ended || state.EndMessage != "the relay stopped carrying it" {
		t.Errorf("the snapshot reports ended=%v reason=%q", state.Ended, state.EndMessage)
	}

	h.client.Stop(id)
	if _, present := h.client.Snapshot()[id]; present {
		t.Error("stopping a decode that had ended left it in the host")
	}
}

// The three kinds share one host and one address space, so nothing keyed by a stream may collide
// with a preview.
func TestTheThreeKindsAreSeparateDecodes(t *testing.T) {
	h := newHarness(t)

	h.open(t, StreamID("desk", "rtsp"), "desk", Events{})
	h.open(t, PreviewID(), "own", Events{})
	h.open(t, MonitorID(0), "screen", Events{})

	states := h.client.Snapshot()
	if len(states) != 3 {
		t.Fatalf("the host holds %d decodes after one of each kind", len(states))
	}
	for _, id := range []ID{StreamID("desk", "rtsp"), PreviewID(), MonitorID(0)} {
		if _, present := states[id]; !present {
			t.Errorf("the snapshot holds no %s", id)
		}
	}
}
