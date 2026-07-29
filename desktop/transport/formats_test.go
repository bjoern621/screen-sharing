package transport

import (
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// legs are the two directions a carriage is stated for, each paired with the capability
// interface every engine serializes that direction through.
// One table drives every per-leg check below, so a leg gained here is checked the same way
// the two present ones are.
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

// A carriage and the serialization that builds it are two halves of one statement,
// per leg and per engine.
// A transport stating formats one engine cannot serialize offers that engine a combination
// nothing can build, and one serializing a leg it states no carriage for is offered for
// streams and refused for every one of them.
// Register asserts the pair on the way in, and this reads it back off the registry,
// which is what holds a transport whose Formats method answers with more than a literal.
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

// statedNoSerialization states a publish carriage on the ffmpeg engine and implements
// no publisher for it, one half of the pair Register refuses.
type statedNoSerialization struct{}

func (statedNoSerialization) Name() string { return "stated-no-serialization" }

func (statedNoSerialization) Formats() Formats {
	return Formats{Publish: map[string]Carriage{
		capabilities.EngineFfmpeg: {Video: []string{"h264"}, Audio: []string{"opus"}},
	}}
}

// serializationNoCarriage is the other half: it serializes an ffmpeg publish and states
// a carriage for the watch leg alone.
// Its watch entry is consistent, so the publish leg is the only thing the assertion can fire on.
type serializationNoCarriage struct{}

func (serializationNoCarriage) Name() string { return "serialization-no-carriage" }

func (serializationNoCarriage) Formats() Formats {
	return Formats{Watch: map[string]Carriage{
		capabilities.EngineGst: {Video: []string{"h264"}, Audio: []string{"opus"}},
	}}
}

func (serializationNoCarriage) PublishArgs(settings.Stream) []string {
	return []string{"-f", "mpegts", "srt://relay.example:8890"}
}

func (serializationNoCarriage) GstSource(settings.Stream, string) []string {
	return []string{"srtsrc", "uri=srt://relay.example:8890"}
}

// Register is where the two halves are held to each other, so a transport stating one
// without the other must not reach the registry.
// Every roster and every refusal here reads the carriage, and would answer for a leg that
// cannot be built, or hide one that can.
func TestRegisterRefusesAHalfStatedLeg(t *testing.T) {
	// assert panics through the standard logger, so each refusal below prints its message
	// on the way out.
	// That message is the expected outcome rather than something to report, and the test
	// reads it off the panic value instead.
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
				// The message is checked, not just the panic.
				// Every other assertion in Register aborts the same way and would prove
				// nothing about the pair this case breaks.
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

// Every format a codec here produces needs a way out and a way back on the engine that
// produced it: one transport that engine publishes it over and one it receives it over.
// A format with neither on an engine is a codec the settings form offers for that capture
// backend and no stream can use.
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

// A publish carriage names what an encoder here will hand the muxer, so every video format
// in one has to be a format the codec table produces.
// A name no row produces is either a typo, which narrows the transport to nothing for that
// format, or a promise about a codec this app cannot encode.
//
// The watch leg is not held to it.
// The relay re-serves whatever it ingested, including a stream this app never published,
// which is why WatchNamesFor narrows on nothing it does not recognize.
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
		// MPEG-TS registers a stream type for H.264 and H.265 and for neither of the others,
		// in both directions and on both engines.
		{"srt", capabilities.EngineFfmpeg, "h264", true, true},
		{"srt", capabilities.EngineGst, "h264", true, true},
		{"srt", capabilities.EngineFfmpeg, "vp9", false, false},
		{"srt", capabilities.EngineGst, "av1", false, false},
		// RTP has a payload format for the whole table.
		{"rtsp", capabilities.EngineFfmpeg, "av1", true, true},
		{"rtsp", capabilities.EngineGst, "vp8", true, true},
		// The relay serves HLS and ingests none of it, and the playlist is the players' leg alone.
		{"hls", capabilities.EngineFfmpeg, "h264", false, true},
		{"hls", capabilities.EngineFfmpeg, "vp8", false, false},
		{"hls", capabilities.EngineGst, "h264", false, false},
		// RTMP is the asymmetric one: the flv muxer writes the enhanced-RTMP tags the relay
		// ingests, flvmux writes none of them, and the FLV demuxers behind both viewers
		// read H.264 alone.
		{"rtmp", capabilities.EngineFfmpeg, "hevc", true, false},
		{"rtmp", capabilities.EngineGst, "hevc", false, false},
		{"rtmp", capabilities.EngineFfmpeg, "h264", true, true},
		{"rtmp", capabilities.EngineGst, "h264", false, true},
		// WHIP ingest is H.264 through ffmpeg's muxer and the WebRTC video set through
		// whipclientsink, and WHEP playback is the receiving pipeline's alone.
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

// The watch lists narrow per format, and a format no codec here produces narrows nothing.
// The relay snapshot can be older than the stream, so absent information must not take a
// working choice away.
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
	// The receiving pipeline's list is the wider one for VP9, since WHEP has no player URL.
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
	// A transport with no publish form on the running engine is refused there and carries
	// the same codec on the other one, which is the whole reason the refusal names the engine.
	if err := ValidatePublish("rtmp", capabilities.EngineGst, "libx264"); err == nil {
		t.Error("rtmp has no GStreamer publish form and must be refused there")
	}
	if err := ValidatePublish("rtmp", capabilities.EngineFfmpeg, "libx264"); err != nil {
		t.Errorf("rtmp carries libx264 through the flv muxer: %v", err)
	}
	// A refusal names where the codec would have worked, since that is the change the
	// settings have to make.
	err := ValidatePublish("srt", capabilities.EngineFfmpeg, "libvpx-vp9")
	if err == nil {
		t.Fatal("srt/MPEG-TS has no VP9 mapping and must be refused")
	}
	if !strings.Contains(err.Error(), "rtsp") {
		t.Errorf("refusal %q must name the transports that carry VP9", err)
	}
}
