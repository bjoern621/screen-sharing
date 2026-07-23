// Package transport abstracts how encoded video leaves the machine and how a
// viewer reaches a stream.
//
// Implementations register themselves in init() (see srt.go). Adding raw UDP,
// WebRTC or anything else later means adding one file here - no caller changes.
package transport

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/settings"
)

// Transport turns stream settings into ffmpeg output args (publish side)
// and an input URL (watch side).
type Transport interface {
	// Name is the registry key, shown in the UI transport dropdown.
	Name() string
	// PublishArgs returns the muxer + destination args appended to the
	// encoder command, e.g. ["-f","mpegts","srt://..."].
	PublishArgs(s settings.Stream) []string
	// WatchURL returns the input URL a player opens to view streamName.
	WatchURL(s settings.Stream, streamName string) string
}

var registry = map[string]Transport{}

// Register adds a transport to the registry.
// Registering the same name twice is a programming error.
func Register(t Transport) {
	_, exists := registry[t.Name()]
	assert.Assert(!exists, "transport registered twice", t.Name())

	registry[t.Name()] = t
}

// Get returns the transport registered under name.
func Get(name string) (t Transport, ok bool) {
	t, ok = registry[name]
	return t, ok
}

// Names lists all registered transports for the UI dropdown.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
