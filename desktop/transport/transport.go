// Package transport abstracts how encoded video leaves the machine and how a
// viewer reaches a stream.
//
// A stream crosses two independent legs, and each names its own protocol: the
// publish leg (publisher to relay) and the watch leg (relay to viewer). They need
// not agree, because the relay re-serves every ingested stream on all its
// listeners, so a stream published over SRT can be watched over RTSP. The publish
// leg is settings.Stream.Transport, read by the publish-side helpers; the watch
// leg is passed by name to WatchURL and GstSource, never read off the settings.
// Any identifier in this package that does not say which leg it means belongs to
// whichever leg its caller is on.
//
// Implementations register themselves in init() (see srt.go). Adding raw UDP or
// anything else later means adding one file here - no caller changes.
//
// The base Transport is engine-neutral: it names itself and the bitstream
// formats it carries. How a stream is serialized for a given publish or watch
// engine is a separate capability interface (FFmpegPublisher, GstPublisher,
// Watcher, GstWatcher). No engine is privileged in the base contract. A
// transport implements one capability per engine it can drive, and an engine
// asks for its own serialization through the matching package helper. A
// transport that carries several engines implements several capabilities; one
// that carries a single engine implements only that one.
package transport

import (
	"fmt"
	"slices"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// Formats is one protocol's carriage, per leg: the bitstream formats that reach
// the relay through it, and the ones the relay serves back out of it. The names
// are capabilities.Codec.Format values ("h264", "hevc", "av1", "vp9", "vp8").
//
// The two legs are separate sets because a relay listener is not symmetric. HLS
// serves what it cannot ingest, and WHIP ingest is narrower than the WHEP
// playback of the same WebRTC entry. An empty set is a leg the protocol has no
// form of here, and that leg's serialization capability is absent with it.
//
// A format belongs in a set when the protocol has a payload mapping for it and
// the relay implements that mapping on this listener, which is one fact about
// the wire rather than one per codec: the encoder that produced the bitstream
// does not change what the protocol can carry.
//
// One list governs both publish engines, so where they serialize different sets
// it states the narrower one (WebRTC, held to ffmpeg's WHIP muxer). A transport
// whose engines are further apart declares the publish capability for one of
// them alone and states that engine's set, which is the honest form where the
// other engine's muxer would refuse most of the list (RTMP, ffmpeg-only).
type Formats struct {
	Publish []string `json:"publish"`
	Watch   []string `json:"watch"`
}

// Transport identifies one protocol, leg-neutral: the same registry entry can
// carry a stream to the relay and from it. The publish and watch serializations
// are the capability interfaces below.
type Transport interface {
	// Name is the registry key, shown in the UI transport dropdown.
	Name() string
	// Formats names the bitstream formats this protocol carries, per leg.
	Formats() Formats
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

// PublishValidator is a transport with publish-leg settings of its own, whose
// legal values only it knows. Both publish engines call ValidatePublishSettings
// beside ValidatePublish, so a value the protocol does not take stops the
// publish here rather than in the process that was handed it.
type PublishValidator interface {
	ValidatePublishSettings(s settings.Stream) error
}

// Watcher is a transport that yields the input URL a player opens to view a
// stream.
type Watcher interface {
	WatchURL(s settings.Stream, streamName string) string
}

// GstWatcher is a transport that serializes to the source elements a receiving
// GStreamer pipeline decodes from. The fragment ends at the encoded stream;
// the receiver appends its own decode and sink elements. It is the watch-side
// counterpart of GstPublisher.
type GstWatcher interface {
	GstSource(s settings.Stream, streamName string) []string
}

var registry = map[string]Transport{}

// Register adds a transport to the registry.
// Registering the same name twice is a programming error.
func Register(t Transport) {
	assert.IsNotNil(t, "a registry entry is a transport")
	assert.Assert(t.Name() != "", "a transport names itself")
	// A protocol that carries nothing on either leg is a row no caller can reach:
	// both serialization sides read the format sets to decide what they offer.
	f := t.Formats()
	assert.Assert(len(f.Publish) > 0 || len(f.Watch) > 0, "a transport carries a format on a leg", t.Name())

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
	args := p.PublishArgs(s)
	assert.Assert(len(args) > 0, "a publishing transport yields ffmpeg output args", s.Transport)
	return args, true
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
	sink := g.GstSink(s)
	assert.Assert(len(sink) > 0, "a publishing transport yields muxer and sink elements", s.Transport)
	return sink, true
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

// GstSource returns the GStreamer source elements a receiving pipeline decodes
// a stream from over the named transport, and false when that transport has no
// GStreamer watch form. Like WatchURL, the transport is named explicitly
// rather than read from s.Transport: the receive transport is chosen
// independently of the publish one.
func GstSource(name string, s settings.Stream, streamName string) ([]string, bool) {
	t, ok := Get(name)
	if !ok {
		return nil, false
	}
	g, ok := t.(GstWatcher)
	if !ok {
		return nil, false
	}
	// The receiving side asserts that a stream carries a source fragment, so an
	// implementation that yields none fails here rather than in the grid process.
	src := g.GstSource(s, streamName)
	assert.Assert(len(src) > 0, "a watching transport yields source elements", name, streamName)
	return src, true
}

// CanGstWatch reports whether the named transport can feed a receiving
// GStreamer pipeline (implements GstWatcher). An unknown name reports false.
func CanGstWatch(name string) bool {
	t, ok := Get(name)
	if !ok {
		return false
	}
	_, ok = t.(GstWatcher)
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
	url := w.WatchURL(s, streamName)
	assert.Assert(url != "", "a watching transport yields a viewer URL", name, streamName)
	return url, true
}

// Names lists all registered transports, sorted. The registry is a map, so the
// list is sorted for a stable order. It spans both legs: a caller that means one
// of them asks PublishNames, WatchNames or GstWatchNames instead.
func Names() []string {
	return namesWhere(func(Transport) bool { return true })
}

// PublishNames lists the transports a stream can be published over: those an
// engine here can serialize a publish command for. A watch-only protocol (HLS,
// which the relay serves but does not ingest) is excluded, so it never reaches
// the publish dropdown to be greyed out with a reason no capture backend could
// lift.
func PublishNames() []string {
	return namesWhere(func(t Transport) bool {
		_, ff := t.(FFmpegPublisher)
		_, gst := t.(GstPublisher)
		return ff || gst
	})
}

// WatchNames lists the transports a viewer program can be pointed at: every
// registered transport that implements Watcher, sorted. It is independent of
// the publish transport, so a stream published over one protocol is offered for
// watching over all protocols the relay re-serves it on. A transport whose
// playback no viewer program opens by URL (WebRTC, which needs WHEP signaling)
// is excluded; GstWatchNames answers the receiving-pipeline question instead.
func WatchNames() []string {
	return namesWhere(func(t Transport) bool { _, ok := t.(Watcher); return ok })
}

// GstWatchNames lists the transports a receiving GStreamer pipeline can decode
// from: every registered transport that implements GstWatcher, sorted. It is the
// native grid's list, and it is wider than WatchNames wherever an element speaks
// a signaling protocol no player URL expresses.
func GstWatchNames() []string {
	return namesWhere(func(t Transport) bool { _, ok := t.(GstWatcher); return ok })
}

func namesWhere(keep func(Transport) bool) []string {
	names := make([]string, 0, len(registry))
	for name, t := range registry {
		if keep(t) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// FormatsOf returns what the named transport carries per leg, and false for a
// name the registry does not know.
func FormatsOf(name string) (Formats, bool) {
	t, ok := Get(name)
	if !ok {
		return Formats{}, false
	}
	return t.Formats(), true
}

// AllFormats maps every registered transport to its carriage, for a UI that
// needs the whole table at once. The frontend greys a codec the selected
// transport cannot publish and states which watch legs carry a stream, both off
// this map, so the rules there and the refusals here read one source.
func AllFormats() map[string]Formats {
	out := make(map[string]Formats, len(registry))
	for name, t := range registry {
		out[name] = t.Formats()
	}
	return out
}

// CanPublishFormat reports whether the named transport carries a bitstream
// format to the relay. An unknown transport carries nothing.
func CanPublishFormat(name, format string) bool {
	f, ok := FormatsOf(name)
	return ok && slices.Contains(f.Publish, format)
}

// CanWatchFormat reports whether the relay re-serves a bitstream format over the
// named transport. An unknown transport carries nothing.
func CanWatchFormat(name, format string) bool {
	f, ok := FormatsOf(name)
	return ok && slices.Contains(f.Watch, format)
}

// PublishNamesFor lists the transports that carry a bitstream format on the
// publish leg, sorted. A format no transport carries yields an empty list, which
// is the settings form's cue that the codec has no publish path at all.
func PublishNamesFor(format string) []string {
	return namesWhere(func(t Transport) bool {
		return slices.Contains(t.Formats().Publish, format)
	})
}

// WatchNamesFor lists the transports a viewer program can receive a bitstream
// format over: WatchNames narrowed to the ones the relay re-serves that format
// on. An SRT viewer opened on a VP9 stream connects and receives nothing, since
// MPEG-TS has no mapping for it, so the choice is offered per format.
//
// A format no implemented codec produces narrows nothing. The relay snapshot can
// be older than the stream, and hiding transports on absent information would
// hide a choice that would have worked.
func WatchNamesFor(format string) []string {
	return narrowToFormat(WatchNames(), format)
}

// GstWatchNamesFor lists the transports a receiving GStreamer pipeline can
// decode a bitstream format from: GstWatchNames narrowed the way WatchNamesFor
// narrows its own list, including for an unknown format.
func GstWatchNamesFor(format string) []string {
	return narrowToFormat(GstWatchNames(), format)
}

func narrowToFormat(names []string, format string) []string {
	// The lists come from namesWhere, so every entry resolves in the carriage lookup
	// below.
	// A name the registry does not know carries nothing there and would drop out of
	// the narrowed list instead.
	for _, name := range names {
		_, ok := Get(name)
		assert.Assert(ok, "a narrowed list holds registered transports", name)
	}
	// An empty format is the roster before the relay reported one, which narrows
	// nothing rather than narrowing to nothing.
	if !capabilities.HasFormat(format) {
		return names
	}
	return slices.DeleteFunc(names, func(name string) bool {
		return !CanWatchFormat(name, format)
	})
}

// ValidatePublish rejects a publish leg that cannot carry the codec's bitstream:
// an unknown transport, one with no publish form, and a format the protocol has
// no mapping for. Both publish engines call it beside capabilities.Validate, so
// a command the relay would refuse to ingest is never built, and the refusal
// names the transports that would have worked.
func ValidatePublish(name, codec string) error {
	t, ok := Get(name)
	if !ok {
		return fmt.Errorf("unknown transport %q", name)
	}
	c, ok := capabilities.Get(codec)
	if !ok {
		return fmt.Errorf("unknown codec %q", codec)
	}
	if !slices.Contains(t.Formats().Publish, c.Format) {
		carried := PublishNamesFor(c.Format)
		if len(carried) == 0 {
			return fmt.Errorf("transport %s cannot carry codec %s: no registered transport publishes %s",
				name, codec, c.Format)
		}
		return fmt.Errorf("transport %s cannot carry codec %s: publish it over %s",
			name, codec, strings.Join(carried, " or "))
	}
	return nil
}

// ValidatePublishSettings checks the configured transport's own publish-leg
// settings, the fields its serializations read off s and only it knows the legal
// values of. A transport declaring none passes, and so does a name the registry
// does not know: that name is ValidatePublish's refusal, and one wrong setting
// should not produce two errors.
func ValidatePublishSettings(s settings.Stream) error {
	t, ok := Get(s.Transport)
	if !ok {
		return nil
	}
	v, ok := t.(PublishValidator)
	if !ok {
		return nil
	}
	return v.ValidatePublishSettings(s)
}
