package transport

import (
	"net/http"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Listener is a transport the relay binds a listener for, yielding where that listener answers
// with no stream on it: "rtsps://relay:8322".
//
// Every address this protocol builds starts here, so its scheme and its port are stated once.
// Past the saving, a reachability check dials what a stream dials: a listener this deployment does
// not bind answers there rather than in a publish that waits out its connect window
// (internal/reach).
type Listener interface {
	ListenerURL(s settings.Settings) string
}

// Listeners is where each transport's listener answers on this relay, keyed by the registry name.
// A transport the relay binds no listener of its own for is absent.
func Listeners(s settings.Settings) map[string]string {
	out := make(map[string]string, len(registry))
	for name, t := range registry {
		l, ok := t.(Listener)
		if !ok {
			continue
		}
		url := l.ListenerURL(s)
		assert.Assert(url != "", "a listener names where it answers", name)
		out[name] = url
	}
	return out
}

// Probe is the request a check makes of one leg.
type Probe struct {
	// Method is the request's verb, meaningless on a leg carrying no HTTP.
	Method string
	// URL is what the probe dials.
	URL string
}

// Probed is a listener a check reaches on a request of its own rather than on its bare address.
//
// One name fronts several HTTP legs, and a path no route claims is answered by whichever server
// handles the rest, so a check dialling the bare listener would report on another leg
// (deploy/Caddyfile).
// A route its own server owns answers for itself, and a request the route serves answers 2xx.
type Probed interface {
	Probe(s settings.Settings) Probe
}

// checkPath is the stream name a check addresses, inside the group the settings name and matching
// no stream of that group.
//
// A name of its own rather than this machine's stream, so a check reads the same on a machine
// that publishes and one that watches.
// Inside the group because a relay token grants that group's prefix and nothing beside it,
// so a name outside it is answered at the credential whatever the listener is doing.
const checkPath = "mirrorme-check"

// Probes is the request a check makes of each transport, keyed by the registry name.
// A GET at the listener itself for a transport naming no request of its own.
func Probes(s settings.Settings) map[string]Probe {
	out := make(map[string]Probe, len(registry))
	for name, address := range Listeners(s) {
		out[name] = Probe{Method: http.MethodGet, URL: address}
		if p, ok := registry[name].(Probed); ok {
			out[name] = p.Probe(s)
		}
		assert.Assert(out[name].URL != "", "a checked leg names where it is dialled", name)
		assert.Assert(out[name].Method != "", "a checked leg names what it asks", name)
	}
	return out
}
