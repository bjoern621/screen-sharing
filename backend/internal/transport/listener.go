package transport

import (
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

// Probed is a listener a check reaches on a route of its own rather than on its bare address.
//
// One name fronts several HTTP legs, and a path no route claims is answered by whichever server
// handles the rest, so a check dialling the bare listener would report on another leg
// (deploy/Caddyfile).
// A route its own server owns answers for itself, refusals included.
type Probed interface {
	ProbeURL(s settings.Settings) string
}

// checkPath is the one path segment a check addresses, matching no stream.
//
// What is asked is whether the route's own server answers,
// which a relay answers before it looks for a stream:
// a request carrying no credential is refused at the credential,
// and a method the route does not take is refused at the method.
// This machine's own stream path would put a group's prefix in every row instead.
const checkPath = "mirrorme-check"

// Probes is where a check dials each transport, keyed by the registry name.
// The listener itself for a transport naming no route of its own.
func Probes(s settings.Settings) map[string]string {
	out := make(map[string]string, len(registry))
	for name, address := range Listeners(s) {
		out[name] = address
		if p, ok := registry[name].(Probed); ok {
			out[name] = p.ProbeURL(s)
		}
		assert.Assert(out[name] != "", "a checked leg names where it is dialled", name)
	}
	return out
}
