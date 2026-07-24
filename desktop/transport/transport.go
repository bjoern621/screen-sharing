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
	"slices"

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

// GstMuxName is the element name every GstPublisher gives its muxer, so a
// publish pipeline can attach further branches (an audio track) with "mux.".
const GstMuxName = "mux"

// GstPublisher is a transport that serializes to the muxer and sink elements
// terminating a GStreamer pipeline. The muxer element carries name=GstMuxName.
type GstPublisher interface {
	GstSink(s settings.Stream) []string
}

// Watcher is a transport that yields the input URL a player opens to view a
// stream.
type Watcher interface {
	WatchURL(s settings.Stream, streamName string) string
}

// GstWatcher is a transport that serializes to the GStreamer source elements a
// receiving pipeline decodes from, the watch-side counterpart of GstPublisher.
type GstWatcher interface {
	GstSource(s settings.Stream, streamName string) []string
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

// CanFFmpegPublish reports whether the named transport can terminate an ffmpeg
// publish command (implements FFmpegPublisher). An unknown name reports false.
func CanFFmpegPublish(name string) bool {
	t, ok := Get(name)
	if !ok {
		return false
	}
	_, ok = t.(FFmpegPublisher)
	return ok
}

// CanGstPublish reports whether the named transport can terminate a GStreamer
// publish pipeline (implements GstPublisher). An unknown name reports false.
func CanGstPublish(name string) bool {
	t, ok := Get(name)
	if !ok {
		return false
	}
	_, ok = t.(GstPublisher)
	return ok
}

// WatchURL returns the viewer input URL for the named transport, and false when
// that transport has no watch form. The transport is named explicitly, not read
// from s.Transport: a stream is received over a transport chosen independently
// of the one it was published through, since the relay re-serves every ingested
// stream on all its listeners.
func WatchURL(name string, s settings.Stream, streamName string) (string, bool) {
	t, ok := Get(name)
	if !ok {
		return "", false
	}
	w, ok := t.(Watcher)
	if !ok {
		return "", false
	}
	return w.WatchURL(s, streamName), true
}

// GstSource returns the GStreamer source elements a receiving pipeline decodes
// the named stream from, and false when the transport has no GStreamer watch
// form. As with WatchURL, the transport is named explicitly, not read from
// s.Transport.
func GstSource(name string, s settings.Stream, streamName string) ([]string, bool) {
	t, ok := Get(name)
	if !ok {
		return nil, false
	}
	g, ok := t.(GstWatcher)
	if !ok {
		return nil, false
	}
	return g.GstSource(s, streamName), true
}

// Names lists all registered transports for the UI dropdown. The registry is a
// map, so the list is sorted for a stable dropdown order.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// WatchNames lists the transports a stream can be received over: every
// registered transport that implements Watcher, sorted. It is independent of
// the publish transport, so a stream published over one protocol is offered for
// watching over all protocols the relay re-serves it on. A transport with no
// watch form (WebRTC, whose playback needs WHEP) is excluded.
func WatchNames() []string {
	names := make([]string, 0, len(registry))
	for name, t := range registry {
		if _, ok := t.(Watcher); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}
