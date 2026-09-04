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
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The relay snapshot is this process's to keep, whether or not anything is asking for it
// (api/proto/screenshare/v1/control.proto).
// A snapshot kept by whoever asked stays at its opening value once nothing asks,
// and a shell then draws "the relay could not be reached" beside a relay that is up.

// indexAt serves one ready stream off the group service's index,
// where a member's app reads what the relay carries:
// the relay's own API is not a member's to read (docs/plan.md).
//
// It binds the port a relay on this network derives, groupd's own,
// because a group service is addressed rather than configured (settings.GroupService).
// A machine already using that port has nothing to poll here,
// and the test says so rather than asserting against whatever else answers on it.
func indexAt(t *testing.T) string {
	t.Helper()

	const host = "127.0.0.1"
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(settings.GroupServicePort)))
	if err != nil {
		t.Skipf("the group service port %d is not free here: %v", settings.GroupServicePort, err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prefix":"group/","streams":[{"name":"bob","ready":true,"tracks":"H264","format":"h264"}]}`))
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	return host
}

// backendAt is a backend pointed at one relay, carrying the fields a poll touches and nothing else:
// the settings say where to ask, the client asks, and the broker is where the answer is announced.
func backendAt(host string) *App {
	return &App{
		events:    events.New(),
		groups:    groupclient.New(),
		relayStop: make(chan struct{}),
		// A listing is a group's, so the machine asking for one holds a key.
		settings: settings.Settings{Relay: settings.Relay{Host: host, GroupKey: aGroupKey}},
	}
}

// awaitSnapshot waits for the poll to record a reachable relay,
// and fails instead of waiting forever.
// It waits on a goroutine's first pass rather than retrying anything:
// the poll fetches before its first tick, so one loopback round trip bounds it.
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

// A listing is a group's, so a machine holding no key asks for none,
// and what it records is the snapshot of a relay nobody asked rather than a refusal to read out.
func TestAMachineInNoGroupAsksForNoListing(t *testing.T) {
	// A loopback address of its own, so a listing that was asked for would fail and say so.
	a := backendAt("127.0.0.2")
	a.settings.Relay.GroupKey = ""

	status := a.relayStatusFor(a.settings)
	if status.Reachable {
		t.Error("a machine holding no group key read a listing")
	}
	if status.Error != "" {
		t.Errorf("the snapshot carries the failure %q, want none to report", status.Error)
	}
}

// Nothing reads and nothing subscribes here, and the snapshot still becomes the relay's answer,
// which the shell's commit gate and viewer roster read.
func TestTheRelayIsPolledWithNobodyAsking(t *testing.T) {
	a := backendAt(indexAt(t))
	a.startRelayPoll()
	defer a.stopRelayPoll()

	status := awaitSnapshot(t, a)
	if len(status.Paths) != 1 || status.Paths[0].Name != "bob" {
		t.Errorf("recorded snapshot carries %+v, want the one stream the index reported", status.Paths)
	}
	if status.Error != "" {
		t.Errorf("a reachable relay recorded the error %q, want none", status.Error)
	}
}

// A second start would ask the relay twice as often,
// halving the interval a byte delta is divided by.
// A second stop would close a closed channel and take the process down.
func TestPollingIsStartedOnceAndStoppedOnce(t *testing.T) {
	a := backendAt(indexAt(t))
	a.startRelayPoll()
	a.startRelayPoll()

	awaitSnapshot(t, a)

	a.stopRelayPoll()
	a.stopRelayPoll()
}

// The failure is recorded rather than the last good answer left standing,
// because "the relay is down" is a thing the screen has to say (docs/ipc-api.md, "Errors").
func TestAnUnreachableRelayIsRecordedAsASnapshot(t *testing.T) {
	// A loopback address of its own, so nothing this test file starts answers on it.
	a := backendAt("127.0.0.2")
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

// A leg the stream's format does not cross is refused before an address reaches the desktop,
// and the refusal names the legs that would have carried it.
// The relay serves H.265 over HLS and not over WebRTC,
// so a page opened on the WHEP leg would load, connect and show nothing.
//
// Only the refusal is asserted:
// the accepting path ends in whatever the machine opens an address with,
// which is not a thing a test may start.
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
