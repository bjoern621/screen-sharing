package decode

import (
	"net"
	"path/filepath"
	"testing"

	"bjoernblessin.de/screenshare/internal/receive"
)

// Tone mapping is part of what a decode is built from,
// so asking for it again has to mean "is this already the case" and never "build it again".
//
// The trap is a machine that cannot tone-map at all.
// A decode there is built without the rung whatever it was asked for,
// so a caller comparing its own request against what ran finds the two different on every call,
// and tears down a pipeline that is already everything it can be.
// receive.WillToneMap is what they are compared through,
// and a handle carries what the host built rather than what the open asked for.

// realHarness is a host opening actual pipelines, and a client connected to it.
//
// A real pipeline rather than a stand-in, because what is asked is what a run was built with:
// a double would answer with whatever it was handed, the reading this is here to rule out.
func realHarness(t *testing.T) *Client {
	t.Helper()

	socket := filepath.Join(t.TempDir(), "host.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("cannot listen on %s: %v", socket, err)
	}

	host := NewReceiveHost()
	go host.Serve(listener)

	client := &Client{dir: t.TempDir(), socket: socket, events: map[ID]Events{}}
	ctrl, err := client.dial(connControl)
	if err != nil {
		t.Fatalf("cannot open the control connection: %v", err)
	}
	client.ctrl = ctrl

	t.Cleanup(func() {
		host.StopAll()
		ctrl.Close()
		listener.Close()
		host.Close()
	})
	return client
}

// aDecodeOf opens one real decode of a synthetic picture, on the chain that needs no display.
func aDecodeOf(t *testing.T, client *Client, toneMap bool) *Handle {
	t.Helper()

	handle, err := client.Open(
		StreamID("probe", "none"),
		receive.Stream{Name: "probe", Transport: "none", Source: "videotestsrc is-live=true", Raw: true},
		receive.Open{Chain: "cpu", ToneMap: toneMap},
		Events{})
	if err != nil {
		t.Fatalf("opening a decode of a synthetic picture: %v", err)
	}
	return handle
}

// A handle answering the ask rather than the build is the loop a machine with no rung would run:
// every repeat of the call would read as a decode that has to be rebuilt.
func TestAHandleReportsTheToneMappingThePipelineWasBuiltWith(t *testing.T) {
	for _, asked := range []bool{false, true} {
		client := realHarness(t)
		handle := aDecodeOf(t, client, asked)

		if built, wanted := handle.ToneMap(), receive.WillToneMap(asked); built != wanted {
			t.Errorf("a decode asked for %t was built with %t, where %t is what that ask builds",
				asked, built, wanted)
		}
		handle.Stop()
	}
}

// The snapshot is read through to the pipeline, so it answers the same as the handle.
// Two readings of one fact that disagree would put a tile and the call that opened it on different
// answers.
func TestTheSnapshotReportsTheSameToneMappingAsTheHandle(t *testing.T) {
	client := realHarness(t)
	handle := aDecodeOf(t, client, true)
	defer handle.Stop()

	state, present := client.Snapshot()[StreamID("probe", "none")]
	if !present {
		t.Fatal("the host holds no decode after one was opened on it")
	}
	if state.ToneMap != handle.ToneMap() {
		t.Errorf("the snapshot reports tone mapping %t and the handle %t", state.ToneMap, handle.ToneMap())
	}
}
