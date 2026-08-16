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
