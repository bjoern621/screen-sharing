package transport

import (
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// legs pairs each direction a carriage is stated for with the capability interface every engine
// serializes it through.
// Every per-leg check below walks this table, so a direction added here is checked like the others.
var legs = []struct {
	name       string
	carriages  func(Formats) map[string]Carriage
	serializes map[string]func(Transport) bool
}{
	{
		name:      "publish",
		carriages: func(f Formats) map[string]Carriage { return f.Publish },
		serializes: map[string]func(Transport) bool{
			capabilities.EngineFfmpeg: func(t Transport) bool { _, ok := t.(FFmpegPublisher); return ok },
			capabilities.EngineGst:    func(t Transport) bool { _, ok := t.(GstPublisher); return ok },
		},
	},
	{
		name:      "watch",
		carriages: func(f Formats) map[string]Carriage { return f.Watch },
		serializes: map[string]func(Transport) bool{
			capabilities.EngineFfmpeg: func(t Transport) bool { _, ok := t.(Watcher); return ok },
			capabilities.EngineGst:    func(t Transport) bool { _, ok := t.(GstWatcher); return ok },
		},
	},
}

// A carriage and the serialization that builds it are two halves of one statement, per leg and per
// engine.
// A transport stating formats an engine cannot serialize offers a combination nothing can build,
// and one serializing a leg it states no carriage for is offered for streams and refused for all of
// them.
// Register asserts the pair on the way in, and this reads it back off the registry, which is what
// holds a Formats method answering with more than a literal.
func TestEveryStatedCarriageHasItsSerialization(t *testing.T) {
	for _, name := range Names() {
		tr, _ := Get(name)
		f := tr.Formats()

		for _, leg := range legs {
			carriages := leg.carriages(f)
			for engine, serializes := range leg.serializes {
				_, stated := carriages[engine]
				if serializes(tr) != stated {
					t.Errorf("%s on the %s leg: %s serializes %v, states a carriage %v",
						name, leg.name, engine, serializes(tr), stated)
				}
			}
		}
	}
}

// statedNoSerialization is one half of the pair Register refuses: a publish carriage on the ffmpeg
// engine with no publisher implementing it.
type statedNoSerialization struct{}

func (statedNoSerialization) Name() string { return "stated-no-serialization" }

func (statedNoSerialization) Formats() Formats {
	return Formats{Publish: map[string]Carriage{
		capabilities.EngineFfmpeg: {Video: []string{"h264"}, Audio: []string{"opus"}},
	}}
}

// serializationNoCarriage is the other half: an ffmpeg publish serialization whose only stated
// carriage is on the watch leg.
// Its watch entry is consistent, which leaves the publish leg as the only thing the assertion can
// fire on.
type serializationNoCarriage struct{}

func (serializationNoCarriage) Name() string { return "serialization-no-carriage" }

func (serializationNoCarriage) Formats() Formats {
	return Formats{Watch: map[string]Carriage{
		capabilities.EngineGst: {Video: []string{"h264"}, Audio: []string{"opus"}},
	}}
}

func (serializationNoCarriage) PublishArgs(settings.Settings) []string {
	return []string{"-f", "mpegts", "srt://relay.example:8890"}
}

func (serializationNoCarriage) GstSource(settings.Settings, string) []string {
	return []string{"srtsrc", "uri=srt://relay.example:8890"}
}

// Register holds the two halves to each other, so a transport stating one without the other stays
// out of the registry.
// Every roster and every refusal here reads the carriage, and would otherwise answer for a leg
// nothing can build, or hide one that can.
func TestRegisterRefusesAHalfStatedLeg(t *testing.T) {
	// assert panics through the standard logger, which prints each refusal below on its way out.
	// That message is the expected outcome and not something to report, so the log goes nowhere and
	// the test reads the message off the panic value.
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	cases := []struct {
		name string
		tr   Transport
	}{
		{"a stated carriage with no serialization", statedNoSerialization{}},
		{"a serialization with no stated carriage", serializationNoCarriage{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				// The message is checked and not only the panic: every other assertion in Register aborts the
				// same way and would prove nothing about the pair this case breaks.
				switch panicked := recover().(type) {
				case nil:
					t.Errorf("Register accepted %s", tc.name)
				case string:
					if !strings.Contains(panicked, "states what it carries") {
						t.Errorf("Register refused %s over %q, want the carriage invariant", tc.name, panicked)
					}
				default:
					t.Errorf("Register refused %s with %v", tc.name, panicked)
				}
				if _, ok := Get(tc.tr.Name()); ok {
					t.Errorf("%s reached the registry", tc.tr.Name())
				}
			}()
			Register(tc.tr)
		})
	}
}

// A format needs a way out and a way back on the engine that produced it: one transport that engine
// publishes it over, and one it receives it over.
// A format missing either is a codec the settings form offers for that capture backend and no
// stream can use.
func TestEveryFormatHasBothLegsOnEveryEngine(t *testing.T) {
	for _, engine := range capabilities.Engines {
		for _, format := range capabilities.Formats() {
			if len(PublishNamesFor(engine, format)) == 0 {
				t.Errorf("format %s has no publish transport on the %s engine", format, engine)
			}
			if len(WatchNamesFor(engine, format)) == 0 {
				t.Errorf("format %s has no watch transport on the %s engine", format, engine)
			}
		}
	}
}

// A publish carriage names what an encoder here hands the muxer, so every video format on one is a
// format the codec table produces.
// A name no row produces is a typo that narrows the transport to nothing for that format, or a
// promise about a codec this app cannot encode.
//
// The watch leg is not held to it, because the relay re-serves whatever it ingested, including a
// stream this app never published, which is why WatchNamesFor narrows on nothing it cannot place.
func TestEveryPublishedVideoFormatIsOneTheCodecTableProduces(t *testing.T) {
	for _, name := range Names() {
		f, _ := FormatsOf(name)
		for engine, c := range f.Publish {
			for _, format := range c.Video {
				if !capabilities.HasFormat(format) {
					t.Errorf("%s publishes %s on the %s engine, which no implemented codec produces",
						name, format, engine)
				}
			}
		}
	}
}

func TestCarriesFormat(t *testing.T) {
	cases := []struct {
		transport, engine, format string
		publish, watchable        bool
	}{
		// MPEG-TS registers a stream type for H.264 and H.265 and for none of the others, in both
		// directions and on both engines.
		{"srt", capabilities.EngineFfmpeg, "h264", true, true},
		{"srt", capabilities.EngineGst, "h264", true, true},
		{"srt", capabilities.EngineFfmpeg, "vp9", false, false},
		{"srt", capabilities.EngineGst, "av1", false, false},
		// RTP payloads the whole codec table.
		{"rtsp", capabilities.EngineFfmpeg, "av1", true, true},
		{"rtsp", capabilities.EngineGst, "vp8", true, true},
		// The relay serves HLS and ingests none, and no GStreamer source element reads the playlist.
		{"hls", capabilities.EngineFfmpeg, "h264", false, true},
		{"hls", capabilities.EngineFfmpeg, "vp8", false, false},
		{"hls", capabilities.EngineGst, "h264", false, false},
		// RTMP is the asymmetric one: the enhanced-RTMP tags the relay ingests come out of the flv muxer
		// and out of no flvmux, and the FLV demuxers behind both readers take H.264 alone.
		{"rtmp", capabilities.EngineFfmpeg, "hevc", true, false},
		{"rtmp", capabilities.EngineGst, "hevc", false, false},
		{"rtmp", capabilities.EngineFfmpeg, "h264", true, true},
		{"rtmp", capabilities.EngineGst, "h264", false, true},
		// WHIP ingest is H.264 through ffmpeg's muxer and the wider WebRTC set through whipclientsink,
		// and WHEP playback is the receiving pipeline's alone, no player opening an exchange.
		{"webrtc", capabilities.EngineFfmpeg, "h264", true, false},
		{"webrtc", capabilities.EngineGst, "h264", true, true},
		{"webrtc", capabilities.EngineFfmpeg, "vp9", false, false},
		{"webrtc", capabilities.EngineGst, "vp9", true, true},
		{"webrtc", capabilities.EngineGst, "hevc", false, false},
		{"nope", capabilities.EngineFfmpeg, "h264", false, false},
	}
	for _, tc := range cases {
		if got := CanPublishFormat(tc.transport, tc.engine, tc.format); got != tc.publish {
			t.Errorf("CanPublishFormat(%s, %s, %s) = %v, want %v", tc.transport, tc.engine, tc.format, got, tc.publish)
		}
		if got := CanWatchFormat(tc.transport, tc.engine, tc.format); got != tc.watchable {
			t.Errorf("CanWatchFormat(%s, %s, %s) = %v, want %v", tc.transport, tc.engine, tc.format, got, tc.watchable)
		}
	}
}

// The watch lists narrow per format, and a format no codec here produces narrows nothing: the relay
// snapshot can be older than the stream, so absent information must not take a working choice away.
func TestWatchNamesForNarrowsByFormat(t *testing.T) {
	if got := WatchNamesFor(capabilities.EngineFfmpeg, "vp9"); slices.Contains(got, "srt") {
		t.Errorf("WatchNamesFor(ffmpeg, vp9) = %v, must exclude srt", got)
	}
	if got := WatchNamesFor(capabilities.EngineFfmpeg, "h264"); !slices.Contains(got, "srt") {
		t.Errorf("WatchNamesFor(ffmpeg, h264) = %v, must include srt", got)
	}
	for _, engine := range capabilities.Engines {
		all := WatchNames(engine)
		if got := WatchNamesFor(engine, "mpeg2"); !slices.Equal(got, all) {
			t.Errorf("WatchNamesFor(%s, unknown format) = %v, want every watch transport %v", engine, got, all)
		}
		if got := WatchNamesFor(engine, ""); !slices.Equal(got, all) {
			t.Errorf("WatchNamesFor(%s, empty) = %v, want every watch transport %v", engine, got, all)
		}
	}
	// For VP9 the receiving pipeline's list is the wider one, WHEP having no player URL.
	if got := WatchNamesFor(capabilities.EngineGst, "vp9"); !slices.Contains(got, "webrtc") {
		t.Errorf("WatchNamesFor(gstreamer, vp9) = %v, must include webrtc", got)
	}
}

func TestValidatePublish(t *testing.T) {
	if err := ValidatePublish("srt", capabilities.EngineFfmpeg, "hevc_nvenc"); err != nil {
		t.Errorf("srt carries hevc_nvenc: %v", err)
	}
	if err := ValidatePublish("nope", capabilities.EngineFfmpeg, "hevc_nvenc"); err == nil {
		t.Error("an unknown transport must be refused")
	}
	if err := ValidatePublish("srt", capabilities.EngineFfmpeg, "nope"); err == nil {
		t.Error("an unknown codec must be refused")
	}
	// The same codec over the same transport passes on one engine and is refused on the other, which
	// is why a refusal names the engine.
	if err := ValidatePublish("rtmp", capabilities.EngineGst, "libx264"); err == nil {
		t.Error("rtmp has no GStreamer publish form and must be refused there")
	}
	if err := ValidatePublish("rtmp", capabilities.EngineFfmpeg, "libx264"); err != nil {
		t.Errorf("rtmp carries libx264 through the flv muxer: %v", err)
	}
	// A refusal names the legs that would have carried the codec, that being the settings change it
	// asks for.
	err := ValidatePublish("srt", capabilities.EngineFfmpeg, "libvpx-vp9")
	if err == nil {
		t.Fatal("srt/MPEG-TS has no VP9 mapping and must be refused")
	}
	if !strings.Contains(err.Error(), "rtsp") {
		t.Errorf("refusal %q must name the transports that carry VP9", err)
	}
}
