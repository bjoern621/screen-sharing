package transport

import (
	"strings"
	"testing"

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

// An HTTP leg is dialled where its own server answers.
// One name fronts several of them and sends a path no route claims to whichever server handles
// the rest, so the bare listener would have one leg reporting another's health (deploy/Caddyfile).
func TestEveryHttpLegIsCheckedOnARouteItsOwnServerOwns(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	want := map[string]string{
		"hls":    "https://relay.example/" + checkPath + "/",
		"webrtc": "https://relay.example/" + checkPath + "/whep",
		"moq":    "https://relay.example:8892/" + checkPath + "/",
	}
	probes := Probes(s)
	for name, url := range want {
		if probes[name] != url {
			t.Errorf("%s is checked at %q, want %q", name, probes[name], url)
		}
	}
}

// A leg whose listener answers no routes is dialled at the listener itself.
// RTSP, RTMP and SRT carry no paths a check can address before a session exists.
func TestALegNamingNoRouteIsCheckedAtItsListener(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	probes := Probes(s)
	for name, listener := range Listeners(s) {
		route, ok := probes[name]
		if !ok {
			t.Errorf("%s is dialled nowhere, so a check can say nothing about it", name)
			continue
		}
		if _, routed := registry[name].(Probed); !routed && route != listener {
			t.Errorf("%s is checked at %q, want its listener %q", name, route, listener)
		}
	}
}

// The path a check dials matches no stream.
// A check asks whether a route's own server answers, which every relay answers before it looks
// for a stream, so dialling one would report a machine's own path rather than the relay.
func TestTheCheckedPathIsNoStream(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Relay.DisplayName = "bjoern"

	for name, url := range Probes(s) {
		if strings.Contains(url, s.PublishPath()) {
			t.Errorf("%s is checked at %q, which names this machine's own stream", name, url)
		}
	}
}
