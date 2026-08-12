package app

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The relay snapshot is this side's to keep, and these tests hold it to that.
//
// It used to be kept by whoever asked: a fetch happened only inside Live(), which the
// Wails frontend called every two seconds, and the control service answered
// GetRelayStatus from whatever that had last recorded. With the frontend gone nothing
// called it, so the snapshot stayed at its opening value - unreachable, no reason
// given - for as long as the process ran, and a shell drew "the relay could not be
// reached" beside a relay that was up. The contract had said the backend polls all
// along (api/proto/screenshare/v1/control.proto).

// relayAt serves one path list at a stopped relay's shape, and answers the address it
// is listening on.
func relayAt(t *testing.T) (string, int) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"bob","ready":true,"tracks":["H264"],"bytesReceived":0,"readers":[]}]}`))
	}))
	t.Cleanup(server.Close)

	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("test relay listening on an address that is not host:port: %v", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("test relay listening on a port that is not a number: %v", err)
	}
	return host, number
}

// backendAt is a backend pointed at one relay, with the two fields a poll touches and
// nothing else: the poll reads the settings for where to ask and announces what it
// found on the broker.
func backendAt(host string, port int) *App {
	return &App{
		events:    events.New(),
		relay:     relay.New(),
		relayStop: make(chan struct{}),
		settings:  settings.Settings{Relay: settings.Relay{Host: host, ApiPort: port}},
	}
}

// awaitSnapshot waits for the poll to have recorded a reachable relay, and fails
// rather than waiting forever. It is a wait on a goroutine's first pass, not a retry
// of anything: the poll fetches before it waits for its first tick, so this is bounded
// by one HTTP round trip over the loopback.
func awaitSnapshot(t *testing.T, a *App) relay.Status {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if status := a.lastRelayStatus(); status.Reachable {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("no relay snapshot was recorded within 5s: %+v", a.lastRelayStatus())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestTheRelayIsPolledWithNobodyAsking: the poll is the process's own. Nothing calls a
// read, nothing subscribes, and the recorded snapshot still becomes the relay's answer
// - which is the whole of what the shell's commit gate and viewer roster read.
func TestTheRelayIsPolledWithNobodyAsking(t *testing.T) {
	host, port := relayAt(t)

	a := backendAt(host, port)
	a.startRelayPoll()
	defer a.stopRelayPoll()

	status := awaitSnapshot(t, a)
	if len(status.Paths) != 1 || status.Paths[0].Name != "bob" {
		t.Errorf("recorded snapshot carries %+v, want the one path the relay reported", status.Paths)
	}
	if status.Error != "" {
		t.Errorf("a reachable relay recorded the error %q, want none", status.Error)
	}
}

// TestPollingIsStartedOnceAndStoppedOnce: both ends of the loop are idempotent, the
// way the control service's are. A second start would ask the relay twice as often and
// halve the interval the byte deltas are divided by; a second stop would close a closed
// channel and take the process down.
func TestPollingIsStartedOnceAndStoppedOnce(t *testing.T) {
	host, port := relayAt(t)

	a := backendAt(host, port)
	a.startRelayPoll()
	a.startRelayPoll()

	awaitSnapshot(t, a)

	a.stopRelayPoll()
	a.stopRelayPoll()
}

// TestAnUnreachableRelayIsRecordedAsASnapshot: the poll records the failure rather than
// leaving the last good answer standing, because "the relay is down" is a thing the
// screen has to say (docs/ipc-api.md, "Errors").
func TestAnUnreachableRelayIsRecordedAsASnapshot(t *testing.T) {
	// A port nothing is listening on, which is what an unreachable relay is. The
	// listener is opened and closed to be told a port the machine had free.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no free port to point an unreachable relay at: %v", err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	listener.Close()

	a := backendAt(host, number)
	a.startRelayPoll()
	defer a.stopRelayPoll()

	deadline := time.Now().Add(10 * time.Second)
	for a.relayLast.Load() == nil {
		if time.Now().After(deadline) {
			t.Fatal("no relay snapshot was recorded within 10s")
		}
		time.Sleep(10 * time.Millisecond)
	}

	status := a.lastRelayStatus()
	if status.Reachable {
		t.Errorf("a relay nothing is listening for reported reachable, want unreachable")
	}
	if status.Error == "" {
		t.Error("an unreachable relay was recorded with no reason, want the one the fetch gave")
	}
}

// A leg the stream's format does not cross is refused before an address reaches the
// desktop, and the sentence names the legs that would have carried it.
//
// It is the browser's half of the check StartWatch runs, asked about a third reader:
// the relay serves H.265 over HLS and refuses it over WebRTC, so a page opened on the
// WHEP leg would load, connect and show nothing. Only the refusal is asserted here -
// the accepting path ends in whatever the machine opens an address with, which is not
// a thing a test may start.
func TestABrowserPageIsRefusedOnALegTheFormatDoesNotCross(t *testing.T) {
	a := &App{events: events.New()}
	a.relayLast.Store(&relay.Status{
		Reachable: true,
		Paths:     []relay.Path{{Name: "bob", Ready: true, Format: "hevc"}},
	})

	err := a.OpenInBrowser("bob", "webrtc")
	if err == nil {
		t.Fatal("a WHEP page was opened for an H.265 stream, want it refused")
	}
	if !strings.Contains(err.Error(), "hls") {
		t.Errorf("refusal %q names no leg that would have carried the stream", err)
	}
}
