// Package transport abstracts how encoded video leaves the machine and how a
// viewer reaches a stream.
//
// Implementations register themselves in init() (see srt.go). Adding raw UDP,
// WebRTC or anything else later means adding one file here - no caller changes.
//
// The base Transport is engine-neutral: it only identifies itself. How a stream
// is serialized for a given publish or watch engine is a separate capability
// interface (FFmpegPublisher, GstPublisher, Watcher). No engine is privileged in
// the base contract. A transport implements one capability per engine it can
// drive, and an engine asks for its own serialization through the matching
// package helper. A transport that carries several engines implements several
// capabilities; one that carries a single engine implements only that one.
package transport

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/settings"
)

// Transport identifies a way video leaves the machine. The publish and watch
// serializations are the capability interfaces below.
type Transport interface {
	// Name is the registry key, shown in the UI transport dropdown.
	Name() string
}

// FFmpegPublisher is a transport that serializes to ffmpeg output args, e.g.
// ["-f","mpegts","srt://..."], appended to the ffmpeg encoder command.
type FFmpegPublisher interface {
	PublishArgs(s settings.Stream) []string
}

// GstPublisher is a transport that serializes to the muxer and sink elements
// terminating a GStreamer pipeline.
type GstPublisher interface {
	GstSink(s settings.Stream) []string
}

// Watcher is a transport that yields the input URL a player opens to view a
// stream.
type Watcher interface {
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

// PublishArgs returns the ffmpeg output args for the configured transport, and
// false when it cannot publish through ffmpeg.
func PublishArgs(s settings.Stream) ([]string, bool) {
	t, ok := Get(s.Transport)
	if !ok {
		return nil, false
	}
	p, ok := t.(FFmpegPublisher)
	if !ok {
		return nil, false
	}
	return p.PublishArgs(s), true
}

// GstSink returns the GStreamer muxer and sink elements for the configured
// transport, and false when it cannot terminate a GStreamer pipeline.
func GstSink(s settings.Stream) ([]string, bool) {
	t, ok := Get(s.Transport)
	if !ok {
		return nil, false
	}
	g, ok := t.(GstPublisher)
	if !ok {
		return nil, false
	}
	return g.GstSink(s), true
}

// WatchURL returns the viewer input URL for the configured transport, and false
// when it has no watch form.
func WatchURL(s settings.Stream, streamName string) (string, bool) {
	t, ok := Get(s.Transport)
	if !ok {
		return "", false
	}
	w, ok := t.(Watcher)
	if !ok {
		return "", false
	}
	return w.WatchURL(s, streamName), true
}

// Names lists all registered transports for the UI dropdown.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
