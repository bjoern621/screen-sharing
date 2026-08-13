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

// The relay snapshot is this process's to keep, whether or not anything is asking for it
// (api/proto/screenshare/v1/control.proto).
// A snapshot kept by whoever asked stays at its opening value once nothing asks, and a shell then
// draws "the relay could not be reached" beside a relay that is up.

// relayAt serves one ready path with no readers, in the shape MediaMTX answers in, and reports the
// address it listens on.
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

// backendAt is a backend pointed at one relay, carrying the fields a poll touches and nothing else:
// the settings say where to ask and the broker is where the answer is announced.
func backendAt(host string, port int) *App {
	return &App{
		events:    events.New(),
		relay:     relay.New(),
		relayStop: make(chan struct{}),
		settings:  settings.Settings{Relay: settings.Relay{Host: host, ApiPort: port}},
	}
}

// awaitSnapshot waits for the poll to record a reachable relay, and fails instead of waiting
// forever.
// It waits on a goroutine's first pass rather than retrying anything: the poll fetches before its
// first tick, so one loopback round trip bounds it.
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

// Nothing reads and nothing subscribes here, and the snapshot still becomes the relay's answer,
// which is what the shell's commit gate and viewer roster read.
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

// A second start would ask the relay twice as often and halve the interval a byte delta is divided
// by.
// A second stop would close a closed channel and take the process down.
func TestPollingIsStartedOnceAndStoppedOnce(t *testing.T) {
	host, port := relayAt(t)

	a := backendAt(host, port)
	a.startRelayPoll()
	a.startRelayPoll()

	awaitSnapshot(t, a)

	a.stopRelayPoll()
	a.stopRelayPoll()
}

// The failure is recorded rather than the last good answer left standing, because "the relay is
// down" is a thing the screen has to say (docs/ipc-api.md, "Errors").
func TestAnUnreachableRelayIsRecordedAsASnapshot(t *testing.T) {
	// An unreachable relay is a port nothing listens on.
	// Opening and closing a listener is how the machine names one it had free.
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

// A leg the stream's format does not cross is refused before an address reaches the desktop, and
// the refusal names the legs that would have carried it.
// The relay serves H.265 over HLS and not over WebRTC, so a page opened on the WHEP leg would load,
// connect and show nothing.
//
// Only the refusal is asserted: the accepting path ends in whatever the machine opens an address
// with, which is not a thing a test may start.
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
