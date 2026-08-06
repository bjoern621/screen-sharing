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
// The base Transport is engine-neutral: it names itself and states what it
// carries per leg and per engine. How a stream is serialized for a given publish
// or watch engine is a separate capability interface (FFmpegPublisher,
// GstPublisher, Watcher, GstWatcher). No engine is privileged in the base
// contract. A transport implements one capability per engine it can drive, and an
// engine asks for its own serialization through the matching package helper. A
// transport that carries several engines implements several capabilities; one
// that carries a single engine implements only that one.
package transport

import (
	"fmt"
	"slices"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// Carriage is what one engine puts through one leg of one protocol: the video
// bitstream formats and the audio codecs its serialization there can carry. The
// video names are capabilities.Codec.Format values ("h264", "hevc", "av1", "vp9",
// "vp8") and the audio names are capabilities.AudioCodec.Format values ("opus",
// "aac").
//
// A format belongs in a set when three things hold together: this engine's muxer
// or source element handles it, the protocol has a payload mapping for it, and
// the relay implements that mapping on this listener. All three are properties of
// the wire and the element, not of the encoder that produced the bitstream.
type Carriage struct {
	Video []string `json:"video"`
	Audio []string `json:"audio"`
}

// Formats is one protocol's carriage, per leg and per engine. The keys are
// capabilities.Engines values, and an engine absent from a leg has no
// serialization for it: the matching capability interface is absent with it, and
// Register holds the two to each other.
//
// Both axes exist because neither collapses without losing a fact. The two legs
// differ because a relay listener is not symmetric: HLS serves what it cannot
// ingest, and WHIP ingest is narrower than the WHEP playback of the same WebRTC
// entry. The two engines differ because they wrap different muxers: ffmpeg's whip
// muxer carries H.264 alone where whipclientsink payloads what webrtcbin
// negotiates, and ffmpeg's flv muxer writes enhanced-RTMP tags where flvmux
// writes only the legacy ones.
//
// One list per leg for both engines would have to be the narrower of the two, and
// the narrowing is invisible at the point it costs something: the engine that
// carries more is refused a format it serializes correctly, with no reason any
// form could show. So each engine states its own set, and where two engines
// genuinely carry the same one they name one shared value rather than agreeing by
// coincidence.
//
// On the publish leg the engines are the two publish engines. On the watch leg
// capabilities.EngineFfmpeg is the URL-opening players (ffplay and mpv, both on
// libavformat) and capabilities.EngineGst is a receiving GStreamer pipeline, the
// native grid's. The browser is neither: it reaches the relay through its own
// RTCPeerConnection and carries its own table (webgrid.ts).
type Formats struct {
	Publish map[string]Carriage `json:"publish"`
	Watch   map[string]Carriage `json:"watch"`
}

// Transport identifies one protocol, leg-neutral: the same registry entry can
// carry a stream to the relay and from it. The publish and watch serializations
// are the capability interfaces below.
type Transport interface {
	// Name is the registry key, shown in the UI transport dropdown.
	Name() string
	// Formats states what this protocol carries, per leg and per engine.
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
//
// A transport's format sets and its serialization capabilities are two statements
// of the same fact, so each is asserted against the other: an engine that states
// a carriage has to be able to serialize the leg, and one that can serialize it
// has to say what it carries. Either half alone is a transport that offers a leg
// it cannot build, or builds one no caller is allowed to reach.
func Register(t Transport) {
	assert.IsNotNil(t, "a registry entry is a transport")
	assert.Assert(t.Name() != "", "a transport names itself")

	f := t.Formats()
	// A protocol that carries nothing on either leg is a row no caller can reach:
	// both serialization sides read the format sets to decide what they offer.
	assert.Assert(len(f.Publish) > 0 || len(f.Watch) > 0, "a transport carries a leg on an engine", t.Name())
	for leg, carriages := range map[string]map[string]Carriage{"publish": f.Publish, "watch": f.Watch} {
		for engine, c := range carriages {
			assert.Assert(knownEngine(engine), "a carriage names a publish or watch engine", t.Name(), leg, engine)
			assert.Assert(len(c.Video) > 0, "a carriage carries a video format", t.Name(), leg, engine)
		}
	}

	_, ffmpegPublish := t.(FFmpegPublisher)
	_, gstPublish := t.(GstPublisher)
	_, watch := t.(Watcher)
	_, gstWatch := t.(GstWatcher)
	assertStated(t, "publish", capabilities.EngineFfmpeg, ffmpegPublish, f.Publish)
	assertStated(t, "publish", capabilities.EngineGst, gstPublish, f.Publish)
	assertStated(t, "watch", capabilities.EngineFfmpeg, watch, f.Watch)
	assertStated(t, "watch", capabilities.EngineGst, gstWatch, f.Watch)

	_, exists := registry[t.Name()]
	assert.Assert(!exists, "transport registered twice", t.Name())

	registry[t.Name()] = t
}

// assertStated holds one engine's serialization capability and its carriage to
// each other, in both directions.
func assertStated(t Transport, leg, engine string, serializes bool, carriages map[string]Carriage) {
	_, stated := carriages[engine]
	assert.Assert(serializes == stated,
		"an engine serializing a leg states what it carries there, and only then",
		t.Name(), leg, engine)
}

// knownEngine reports whether engine names a publish or watch engine. Every
// engine reaching this package is named by the caller rather than read off the
// settings, so one outside the set is a caller that made it up and the lookups
// assert it.
func knownEngine(engine string) bool {
	return slices.Contains(capabilities.Engines, engine)
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

// CanPublish reports whether the named engine can serialize a publish command for
// the named transport. An unknown transport reports false.
func CanPublish(name, engine string) bool {
	assert.Assert(knownEngine(engine), "a publish question names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return false
	}
	_, ok = f.Publish[engine]
	return ok
}

// CanWatch reports whether the named engine can receive over the named
// transport: a URL for the players, a source fragment for a GStreamer pipeline.
// An unknown transport reports false.
func CanWatch(name, engine string) bool {
	assert.Assert(knownEngine(engine), "a watch question names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return false
	}
	_, ok = f.Watch[engine]
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
// list is sorted for a stable order. It spans both legs and both engines: a
// caller that means one of them asks PublishNames or WatchNames instead.
func Names() []string {
	return namesWhere(func(Transport) bool { return true })
}

// PublishNames lists the transports this engine can publish a stream over,
// sorted. A watch-only protocol (HLS, which the relay serves but does not
// ingest) is in no engine's list, so it never reaches the publish dropdown to be
// greyed out with a reason no capture backend could lift.
func PublishNames(engine string) []string {
	assert.Assert(knownEngine(engine), "a publish roster names an engine", engine)

	return namesWhere(func(t Transport) bool {
		_, ok := t.Formats().Publish[engine]
		return ok
	})
}

// WatchNames lists the transports this engine can receive over, sorted. It is
// independent of the publish transport, so a stream published over one protocol
// is offered for watching over every protocol the relay re-serves it on.
//
// The two engines' lists each hold a transport the other lacks: a player needs a
// URL and WHEP is an exchange rather than an address, while nothing on the
// GStreamer side reads the relay's HLS segments.
func WatchNames(engine string) []string {
	assert.Assert(knownEngine(engine), "a watch roster names an engine", engine)

	return namesWhere(func(t Transport) bool {
		_, ok := t.Formats().Watch[engine]
		return ok
	})
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

// FormatsOf returns what the named transport carries per leg and engine, and
// false for a name the registry does not know.
func FormatsOf(name string) (Formats, bool) {
	t, ok := Get(name)
	if !ok {
		return Formats{}, false
	}
	return t.Formats(), true
}

// PublishCarriage returns what the named engine puts through the named
// transport's publish leg, and false when that engine has no publish form for it.
func PublishCarriage(name, engine string) (Carriage, bool) {
	assert.Assert(knownEngine(engine), "a carriage lookup names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return Carriage{}, false
	}
	c, ok := f.Publish[engine]
	return c, ok
}

// WatchCarriage returns what the named engine receives over the named
// transport's watch leg, and false when that engine has no watch form for it.
func WatchCarriage(name, engine string) (Carriage, bool) {
	assert.Assert(knownEngine(engine), "a carriage lookup names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return Carriage{}, false
	}
	c, ok := f.Watch[engine]
	return c, ok
}

// AllFormats maps every registered transport to its carriage, for a UI that
// needs the whole table at once. The frontend greys a codec the selected
// transport and engine cannot publish and states which watch legs carry a
// stream, both off this one table rather than a copy per rule.
func AllFormats() map[string]Formats {
	out := make(map[string]Formats, len(registry))
	for name, t := range registry {
		out[name] = t.Formats()
	}
	return out
}

// CanPublishFormat reports whether this engine carries a bitstream format to the
// relay over the named transport. An unknown transport carries nothing.
func CanPublishFormat(name, engine, format string) bool {
	c, ok := PublishCarriage(name, engine)
	return ok && slices.Contains(c.Video, format)
}

// CanWatchFormat reports whether this engine receives a bitstream format over the
// named transport. An unknown transport carries nothing.
func CanWatchFormat(name, engine, format string) bool {
	c, ok := WatchCarriage(name, engine)
	return ok && slices.Contains(c.Video, format)
}

// CanPublishAudio reports whether this engine carries an audio codec to the relay
// over the named transport. An unknown transport carries nothing.
func CanPublishAudio(name, engine, format string) bool {
	c, ok := PublishCarriage(name, engine)
	return ok && slices.Contains(c.Audio, format)
}

// PublishNamesFor lists the transports this engine carries a bitstream format
// over on the publish leg, sorted. A format the engine has no publish path for at
// all yields an empty list, which is the settings form's cue that the codec
// cannot be published from this capture backend.
func PublishNamesFor(engine, format string) []string {
	assert.Assert(knownEngine(engine), "a publish roster names an engine", engine)

	return namesWhere(func(t Transport) bool {
		return slices.Contains(t.Formats().Publish[engine].Video, format)
	})
}

// WatchNamesFor lists the transports this engine can receive a bitstream format
// over: WatchNames narrowed to the ones the relay re-serves that format on. An
// SRT viewer opened on a VP9 stream connects and receives nothing, since MPEG-TS
// has no mapping for it, so the choice is offered per format.
//
// A format no implemented codec produces narrows nothing. The relay snapshot can
// be older than the stream, and hiding transports on absent information would
// hide a choice that would have worked.
func WatchNamesFor(engine, format string) []string {
	names := WatchNames(engine)
	// An empty format is the roster before the relay reported one, which narrows
	// nothing rather than narrowing to nothing.
	if !capabilities.HasFormat(format) {
		return names
	}
	return slices.DeleteFunc(names, func(name string) bool {
		return !CanWatchFormat(name, engine, format)
	})
}

// ValidatePublish rejects a publish leg that cannot carry the codec's bitstream:
// an unknown transport, one this engine has no publish form for, and a format the
// engine's muxer or the protocol has no mapping for. Both publish engines call it
// beside capabilities.Validate under their own name, so a command the relay would
// refuse to ingest is never built, and the refusal names the transports that
// would have worked on the engine that is running.
func ValidatePublish(name, engine, codec string) error {
	assert.Assert(knownEngine(engine), "publish validation names an engine", engine)

	if _, ok := Get(name); !ok {
		return fmt.Errorf("unknown transport %q", name)
	}
	c, ok := capabilities.Get(codec)
	if !ok {
		return fmt.Errorf("unknown codec %q", codec)
	}
	carriage, ok := PublishCarriage(name, engine)
	if !ok {
		return fmt.Errorf("transport %s has no %s publish form, so it cannot carry codec %s: publish it over %s",
			name, engine, codec, orNone(PublishNamesFor(engine, c.Format)))
	}
	if !slices.Contains(carriage.Video, c.Format) {
		return fmt.Errorf("transport %s cannot carry codec %s on the %s engine: publish it over %s",
			name, codec, engine, orNone(PublishNamesFor(engine, c.Format)))
	}
	return nil
}

// ValidatePublishAudio rejects an audio track the publish leg cannot carry. A
// stream with no audio passes: there is no track to find a mapping for.
//
// It is a second refusal rather than a wider ValidatePublish because the two fail
// for different reasons and the fix differs: a video format the leg lacks is
// answered by another transport, an audio codec it lacks by another audio codec
// on the same one.
func ValidatePublishAudio(name, engine, audioCodec string) error {
	assert.Assert(knownEngine(engine), "publish validation names an engine", engine)

	if audioCodec == capabilities.AudioNone {
		return nil
	}
	a, ok := capabilities.GetAudio(audioCodec)
	if !ok {
		return fmt.Errorf("unknown audio codec %q", audioCodec)
	}
	carriage, ok := PublishCarriage(name, engine)
	if !ok {
		return fmt.Errorf("transport %s has no %s publish form, so it cannot carry audio codec %s", name, engine, audioCodec)
	}
	if !slices.Contains(carriage.Audio, a.Format) {
		return fmt.Errorf("transport %s cannot carry audio codec %s on the %s engine: use %s, or publish without audio",
			name, audioCodec, engine, orNone(capabilities.AudioNamesFor(engine, carriage.Audio)))
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

// orNone renders the alternatives a refusal points at, and says so where there
// are none rather than trailing off after "use".
func orNone(names []string) string {
	if len(names) == 0 {
		return "nothing this engine carries"
	}
	return strings.Join(names, " or ")
}
