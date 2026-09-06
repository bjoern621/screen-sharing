package transport

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/settings"
)

// Every transport reaches the relay on a listener of some kind, so one stating none is a leg
// nothing can report on: a reachability check dials what this answers and skips what it does not
// (internal/reach).
func TestEveryTransportStatesWhereItsListenerAnswers(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	listeners := Listeners(s)
	for name := range AllFormats() {
		if _, ok := listeners[name]; !ok {
			t.Errorf("transport %q states no listener, so nothing can say whether it answers", name)
		}
	}
}

// A listener address is where the protocol answers and nothing more: no stream, no credential.
// A path on it would make every reader that appends one address a stream inside a stream.
func TestAListenerAddressCarriesNoStreamAndNoCredential(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Relay.Token = "a-token"

	for name, url := range Listeners(s) {
		if strings.HasSuffix(url, "/") {
			t.Errorf("%s answers at %q, which ends in the separator every address builder adds", name, url)
		}
		if strings.Contains(url, "a-token") {
			t.Errorf("%s answers at %q, which carries the credential", name, url)
		}
	}
}

// An HTTP leg is checked with a request its own server serves, which is what makes the answer 2xx.
// One name fronts several of them and sends a path no route claims to whichever server handles
// the rest, so the bare listener would have one leg reporting another's health (deploy/Caddyfile).
func TestEveryHttpLegIsCheckedWithARequestItsOwnServerServes(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	want := map[string]Probe{
		"hls":    {Method: "GET", URL: "https://relay.example/" + checkPath + "/"},
		"webrtc": {Method: "OPTIONS", URL: "https://relay.example/" + checkPath + "/whep"},
		"moq":    {Method: "GET", URL: "https://relay.example:8892/" + checkPath + "/"},
	}
	probes := Probes(s)
	for name, probe := range want {
		if probes[name] != probe {
			t.Errorf("%s is checked with %v, want %v", name, probes[name], probe)
		}
	}
}

// A leg whose listener answers no routes is dialled at the listener itself.
// RTSP, RTMP and SRT carry no paths a check can address before a session exists.
func TestALegNamingNoRequestIsCheckedAtItsListener(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	probes := Probes(s)
	for name, listener := range Listeners(s) {
		probe, ok := probes[name]
		if !ok {
			t.Errorf("%s is dialled nowhere, so a check can say nothing about it", name)
			continue
		}
		if _, named := registry[name].(Probed); !named && probe.URL != listener {
			t.Errorf("%s is checked at %q, want its listener %q", name, probe.URL, listener)
		}
	}
}

// The checked path lives in the group the settings name, which is the whole of what a relay token
// grants: a name outside it is answered at the credential whatever the listener is doing.
func TestTheCheckedPathLivesInTheGroup(t *testing.T) {
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Relay.GroupKey = key.String()

	prefix := s.Relay.Path(checkPath)
	if prefix == checkPath {
		t.Fatalf("a group key derives no prefix, so this asserts nothing")
	}
	for name, probe := range Probes(s) {
		if _, named := registry[name].(Probed); !named {
			continue
		}
		if !strings.Contains(probe.URL, prefix) {
			t.Errorf("%s is checked at %q, which lies outside the group at %q", name, probe.URL, prefix)
		}
	}
}

// The checked path names no stream of that group, so a check reads the same on a machine that
// publishes and one that watches.
func TestTheCheckedPathIsNoStream(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Relay.DisplayName = "bjoern"

	for name, probe := range Probes(s) {
		if strings.Contains(probe.URL, s.PublishPath()) {
			t.Errorf("%s is checked at %q, which names this machine's own stream", name, probe.URL)
		}
	}
}
