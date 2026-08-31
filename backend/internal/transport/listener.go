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
