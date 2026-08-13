// Package transport states how encoded video leaves the machine and how a viewer reaches a stream.
//
// A stream crosses two independent legs, each naming its own protocol: publish (publisher to relay)
// and watch (relay to viewer).
// The relay re-serves every ingested stream on all its listeners, so the legs need not agree and a
// stream published over SRT can be watched over RTSP.
// The publish leg is settings.Settings.Transport; the watch leg is passed by name to WatchURL and
// GstSource, never read off the settings.
// An identifier here that does not name a leg belongs to whichever leg its caller is on.
//
// Implementations register in init() (srt.go), so a protocol is added as one file here and no
// caller changes.
//
// The base Transport is engine-neutral, naming itself and stating what it carries per leg and per
// engine.
// Serialization for one engine is a separate capability interface (FFmpegPublisher, GstPublisher,
// Watcher, BrowserWatcher, GstWatcher), one implemented per engine a transport drives.
package transport

import (
	"fmt"
	"slices"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// Carriage is one engine's set for one leg of one protocol: the video bitstream formats and the
// audio codecs its serialization there carries.
// Video names are capabilities.Codec.Format values ("h264", "vp9"), audio names are
// capabilities.AudioCodec.Format values ("opus", "aac").
//
// A format belongs in a set when all three hold: this engine's muxer or source element handles it,
// the protocol has a payload mapping for it, and the relay implements that mapping on the listener.
// All three are properties of the wire and the element, never of the encoder that produced the
// bitstream.
type Carriage struct {
	Video []string `json:"video"`
	Audio []string `json:"audio"`
}

// Formats holds one protocol's carriages, keyed by leg and engine.
// An engine missing from a leg has no serialization for it, the matching capability interface being
// missing with it, and Register holds the two to each other.
//
// Neither axis collapses without losing a fact.
// A relay listener is not symmetric, so the legs differ: HLS serves what it cannot ingest, and WHIP
// ingest is narrower than the WHEP playback of the same WebRTC entry.
// The engines wrap different muxers, so they differ too: ffmpeg's whip muxer carries H.264 alone
// where whipclientsink payloads what webrtcbin negotiates, and ffmpeg's flv muxer writes
// enhanced-RTMP tags where flvmux writes the legacy ones alone.
// One list per leg would have to be the narrower of the two engines, and the narrowing costs where
// it cannot be seen: the engine carrying more is refused a format it serializes correctly, with no
// reason any form could show.
//
// Publish keys are capabilities.Engines, the two publish engines.
// Watch keys are the three readers: capabilities.EngineFfmpeg the URL-opening players (ffplay and
// mpv, both on libavformat), capabilities.EngineGst the tile grid's receiving GStreamer pipeline,
// and EngineBrowser the machine's default browser on the player page the relay serves.
type Formats struct {
	Publish map[string]Carriage `json:"publish"`
	Watch   map[string]Carriage `json:"watch"`
}

// Transport is one protocol, leg-neutral: the same registry entry carries a stream to the relay and
// from it.
// The serializations are the capability interfaces below.
type Transport interface {
	// Name is the registry key, and the value the transport dropdown shows.
	Name() string
	Formats() Formats
}

// FFmpegPublisher serializes to ffmpeg output args, ["-f","mpegts","srt://..."], appended to the
// encoder command.
type FFmpegPublisher interface {
	PublishArgs(s settings.Settings) []string
}

// GstMuxName is the name every GstPublisher gives its muxer element, so a publish pipeline attaches
// further branches (an audio track) with "mux.".
const GstMuxName = "mux"

// GstPublisher serializes to the muxer and sink elements terminating a GStreamer pipeline.
// The muxer element carries name=GstMuxName.
type GstPublisher interface {
	GstSink(s settings.Settings) []string
}

// PublishValidator is a transport carrying publish-leg settings whose legal values only it knows.
// Both publish engines call ValidatePublishSettings beside ValidatePublish, so a value the protocol
// does not take stops here rather than inside the process that was handed it.
type PublishValidator interface {
	ValidatePublishSettings(s settings.Settings) error
}

// Watcher yields the input URL a player opens.
type Watcher interface {
	WatchURL(s settings.Settings, streamName string) string
}

// EngineBrowser is the third watch engine, the machine's default browser on the player page the
// relay serves for a stream.
//
// Named here and not in capabilities.Engines, which is the publish engines, what a Gap may name and
// what an encoder probe runs against, none of which a reader that encodes nothing belongs in.
// Nothing here publishes through a browser, so the constant is watch-side only.
const EngineBrowser = "browser"

// WatchEngines are the readers a watch carriage can be stated for, in the order a table walks them.
// Counterpart of capabilities.Engines, and a second list rather than that one: the browser reads
// and never publishes, so a walk over the publish engines would drop every row it states.
var WatchEngines = []string{capabilities.EngineFfmpeg, capabilities.EngineGst, EngineBrowser}

// BrowserWatcher is a transport the relay serves a player page for, yielding that page's address.
// Separate from Watcher because the two readers open different things: a player takes the media
// address, a browser takes an HTML page that fetches the media itself.
//
// The page is the relay's own and not anything this app serves, so what it plays is a property of
// the relay's listener and of the browser running it, which is what the browser carriage states.
type BrowserWatcher interface {
	BrowserURL(s settings.Settings, streamName string) string
}

// GstWatcher serializes to the source elements a receiving GStreamer pipeline decodes from.
// The fragment ends at the encoded stream, and the receiver appends the decode and sink elements.
type GstWatcher interface {
	GstSource(s settings.Settings, streamName string) []string
}

var registry = map[string]Transport{}

// Register adds a transport to the registry, and a name registered twice is an Entwicklungsfehler.
//
// A transport's format sets and its serialization capabilities state one fact twice, so each is
// asserted against the other: an engine stating a carriage serializes the leg, and an engine that
// serializes a leg says what it carries.
// Either half alone is a transport offering a leg it cannot build, or building one no caller may
// reach.
func Register(t Transport) {
	assert.IsNotNil(t, "a registry entry is a transport")
	assert.Assert(t.Name() != "", "a transport names itself")

	f := t.Formats()
	// Both serialization sides read the format sets to decide what they offer, so a protocol carrying
	// nothing on either leg is a row no caller reaches.
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
	_, browserWatch := t.(BrowserWatcher)
	assertStated(t, "publish", capabilities.EngineFfmpeg, ffmpegPublish, f.Publish)
	assertStated(t, "publish", capabilities.EngineGst, gstPublish, f.Publish)
	assertStated(t, "watch", capabilities.EngineFfmpeg, watch, f.Watch)
	assertStated(t, "watch", capabilities.EngineGst, gstWatch, f.Watch)
	assertStated(t, "watch", EngineBrowser, browserWatch, f.Watch)

	_, exists := registry[t.Name()]
	assert.Assert(!exists, "transport registered twice", t.Name())

	registry[t.Name()] = t
}

// assertStated fails unless one engine serializes a leg exactly when it states a carriage there.
func assertStated(t Transport, leg, engine string, serializes bool, carriages map[string]Carriage) {
	_, stated := carriages[engine]
	assert.Assert(serializes == stated,
		"an engine serializing a leg states what it carries there, and only then",
		t.Name(), leg, engine)
}

// knownEngine covers the publish engines and the watch readers together.
// Every engine reaching this package is named by the caller rather than read off the settings, so
// one outside the set is a caller that made it up, and the lookups assert it.
//
// The browser passes on both legs, and a browser publish carriage is Register's refusal rather than
// this predicate's: holding a stated carriage and a serialization capability to each other is where
// that combination has no serialization to be held to.
func knownEngine(engine string) bool {
	return slices.Contains(capabilities.Engines, engine) || slices.Contains(WatchEngines, engine)
}

func Get(name string) (t Transport, ok bool) {
	t, ok = registry[name]
	return t, ok
}

// PublishArgs is the ffmpeg output args for the configured publish transport.
// false where that transport has no ffmpeg publish form.
func PublishArgs(s settings.Settings) ([]string, bool) {
	t, ok := Get(s.Publish.Transport)
	if !ok {
		return nil, false
	}
	p, ok := t.(FFmpegPublisher)
	if !ok {
		return nil, false
	}
	args := p.PublishArgs(s)
	assert.Assert(len(args) > 0, "a publishing transport yields ffmpeg output args", s.Publish.Transport)
	return args, true
}

// GstSink is the muxer and sink elements for the configured publish transport.
// false where that transport terminates no GStreamer pipeline.
func GstSink(s settings.Settings) ([]string, bool) {
	t, ok := Get(s.Publish.Transport)
	if !ok {
		return nil, false
	}
	g, ok := t.(GstPublisher)
	if !ok {
		return nil, false
	}
	sink := g.GstSink(s)
	assert.Assert(len(sink) > 0, "a publishing transport yields muxer and sink elements", s.Publish.Transport)
	return sink, true
}

// CanPublish reports whether the named engine serializes a publish command for the named transport,
// and false for a name the registry does not know.
func CanPublish(name, engine string) bool {
	assert.Assert(knownEngine(engine), "a publish question names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return false
	}
	_, ok = f.Publish[engine]
	return ok
}

// CanWatch reports whether the named engine receives over the named transport: a URL for the
// players, a source fragment for a GStreamer pipeline, a page address for the browser.
// false for a name the registry does not know.
func CanWatch(name, engine string) bool {
	assert.Assert(knownEngine(engine), "a watch question names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return false
	}
	_, ok = f.Watch[engine]
	return ok
}

// GstSource is the source elements a receiving pipeline decodes a stream from over the named
// transport.
// false where that transport has no GStreamer watch form.
// The transport is named explicitly and never read off s.Transport, for WatchURL's reason.
func GstSource(name string, s settings.Settings, streamName string) ([]string, bool) {
	t, ok := Get(name)
	if !ok {
		return nil, false
	}
	g, ok := t.(GstWatcher)
	if !ok {
		return nil, false
	}
	// The receiving side asserts a stream has a source fragment, so an implementation yielding none
	// fails here instead of in the grid process.
	src := g.GstSource(s, streamName)
	assert.Assert(len(src) > 0, "a watching transport yields source elements", name, streamName)
	return src, true
}

// WatchURL is the viewer input URL for the named transport.
// false where that transport has no player watch form.
// The transport is named explicitly and never read off s.Transport: the relay re-serves every
// ingested stream on all its listeners, so a viewer picks its leg independently of the publisher.
func WatchURL(name string, s settings.Settings, streamName string) (string, bool) {
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

// BrowserURL is the address of the relay's player page for the named transport.
// false where the relay serves no page on that leg.
// The transport is named explicitly, for WatchURL's reason.
func BrowserURL(name string, s settings.Settings, streamName string) (string, bool) {
	t, ok := Get(name)
	if !ok {
		return "", false
	}
	w, ok := t.(BrowserWatcher)
	if !ok {
		return "", false
	}
	url := w.BrowserURL(s, streamName)
	assert.Assert(url != "", "a transport with a player page yields its address", name, streamName)
	return url, true
}

// Names is every registered transport, sorted because the registry is a map and iteration order is
// otherwise arbitrary.
// It spans both legs and every engine: a caller meaning one asks PublishNames or WatchNames.
func Names() []string {
	return namesWhere(func(Transport) bool { return true })
}

// PublishNames is the transports this engine publishes a stream over, sorted.
// A watch-only protocol, HLS, which the relay serves and does not ingest, is on no engine's list,
// so it never reaches the publish dropdown to be greyed with a reason no capture backend can lift.
func PublishNames(engine string) []string {
	assert.Assert(knownEngine(engine), "a publish roster names an engine", engine)

	return namesWhere(func(t Transport) bool {
		_, ok := t.Formats().Publish[engine]
		return ok
	})
}

// WatchNames is the transports this engine receives over, sorted.
// Independent of the publish transport, so a stream is offered for watching over every protocol the
// relay re-serves it on.
//
// No two reader lists agree, and each difference is a property of the reader: a player needs a URL
// and WHEP is an exchange rather than an address, nothing on the GStreamer side reads the relay's
// HLS segments, and the browser reaches the legs the relay serves a page for.
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

// FormatsOf is what the named transport carries per leg and engine.
// false for a name the registry does not know.
func FormatsOf(name string) (Formats, bool) {
	t, ok := Get(name)
	if !ok {
		return Formats{}, false
	}
	return t.Formats(), true
}

// PublishCarriage is what the named engine puts through the named transport's publish leg.
// false where that engine has no publish form for it.
func PublishCarriage(name, engine string) (Carriage, bool) {
	assert.Assert(knownEngine(engine), "a carriage lookup names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return Carriage{}, false
	}
	c, ok := f.Publish[engine]
	return c, ok
}

// WatchCarriage is what the named engine receives over the named transport's watch leg.
// false where that engine has no watch form for it.
func WatchCarriage(name, engine string) (Carriage, bool) {
	assert.Assert(knownEngine(engine), "a carriage lookup names an engine", engine)

	f, ok := FormatsOf(name)
	if !ok {
		return Carriage{}, false
	}
	c, ok := f.Watch[engine]
	return c, ok
}

// AllFormats is every registered transport's carriage, for a UI needing the whole table at once.
// Greying a codec the selected transport and engine cannot publish and naming the watch legs that
// carry a stream both read this table rather than a copy per rule.
func AllFormats() map[string]Formats {
	out := make(map[string]Formats, len(registry))
	for name, t := range registry {
		out[name] = t.Formats()
	}
	return out
}

// CanPublishFormat asks the publish carriage, so an unknown transport carries nothing.
func CanPublishFormat(name, engine, format string) bool {
	c, ok := PublishCarriage(name, engine)
	return ok && slices.Contains(c.Video, format)
}

// CanWatchFormat asks the watch carriage, so an unknown transport carries nothing.
func CanWatchFormat(name, engine, format string) bool {
	c, ok := WatchCarriage(name, engine)
	return ok && slices.Contains(c.Video, format)
}

// CanPublishAudio asks the publish carriage's audio set, so an unknown transport carries nothing.
func CanPublishAudio(name, engine, format string) bool {
	c, ok := PublishCarriage(name, engine)
	return ok && slices.Contains(c.Audio, format)
}

// PublishNamesFor is the transports this engine publishes a bitstream format over, sorted.
// A format with no publish path on the engine yields an empty list, the settings form's cue that
// the codec cannot leave this capture backend.
func PublishNamesFor(engine, format string) []string {
	assert.Assert(knownEngine(engine), "a publish roster names an engine", engine)

	return namesWhere(func(t Transport) bool {
		return slices.Contains(t.Formats().Publish[engine].Video, format)
	})
}

// WatchNamesFor is WatchNames narrowed to the transports the relay re-serves this format on.
// The narrowing is per format because a leg that carries a stream carries it as a bitstream: an SRT
// viewer opened on a VP9 stream connects and receives nothing, MPEG-TS having no mapping for it.
//
// A format no implemented codec produces narrows nothing, the relay snapshot being able to lag the
// stream: hiding transports on absent information would hide a choice that would have worked.
func WatchNamesFor(engine, format string) []string {
	names := WatchNames(engine)
	// An empty format is the roster before the relay reported one: it narrows nothing rather than
	// narrowing to nothing.
	if !capabilities.HasFormat(format) {
		return names
	}
	return slices.DeleteFunc(names, func(name string) bool {
		return !CanWatchFormat(name, engine, format)
	})
}

// ValidatePublish refuses a publish leg that cannot carry the codec's bitstream: an unknown
// transport, one this engine has no publish form for, and a format the engine's muxer or the
// protocol has no mapping for.
// Both publish engines call it beside capabilities.Validate under their own name, so a command the
// relay would refuse to ingest is never built, and the refusal names the transports that would have
// worked on the engine that is running.
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

// ValidatePublishAudio refuses an audio track the publish leg cannot carry.
// A stream with no audio passes: no track, no mapping to find.
//
// A second refusal rather than a wider ValidatePublish, because the fix differs: a video format the
// leg lacks is answered by another transport, an audio codec it lacks by another audio codec on the
// same one.
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

// ValidatePublishSettings checks the configured transport's own publish-leg settings, the fields
// its serializations read off s and whose legal values only it knows.
// A transport declaring none passes, and so does a name the registry does not know: that name is
// ValidatePublish's refusal, and one wrong setting owes the user one error.
func ValidatePublishSettings(s settings.Settings) error {
	t, ok := Get(s.Publish.Transport)
	if !ok {
		return nil
	}
	v, ok := t.(PublishValidator)
	if !ok {
		return nil
	}
	return v.ValidatePublishSettings(s)
}

// orNone renders the alternatives a refusal points at, and names the empty case rather than letting
// a sentence trail off after "use".
func orNone(names []string) string {
	if len(names) == 0 {
		return "nothing this engine carries"
	}
	return strings.Join(names, " or ")
}
