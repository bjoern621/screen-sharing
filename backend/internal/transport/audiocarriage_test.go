package transport

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// ValidatePublishAudio matches the configured track against a carriage's audio names, so those
// names are capabilities.AudioCodec.Format values.
// A name outside that vocabulary matches no codec, and every audio publish over the leg stating it
// is refused with a list of alternatives the name is not on.
func TestEveryCarriedAudioFormatIsAnAudioCodecFormat(t *testing.T) {
	formats := make([]string, 0, len(capabilities.AudioCodecs))
	for _, a := range capabilities.AudioCodecs {
		formats = append(formats, a.Format)
	}

	for _, name := range Names() {
		f, _ := FormatsOf(name)
		for _, leg := range legs {
			for engine, c := range leg.carriages(f) {
				for _, format := range c.Audio {
					if !slices.Contains(formats, format) {
						t.Errorf("%s carries audio %s on the %s leg of the %s engine, which is not one of %v",
							name, format, leg.name, engine, formats)
					}
				}
			}
		}
	}
}

// A track reaches the relay only where an engine codes it and a transport carries it, so a codec no
// transport carries on the engine that codes it is a row the settings form offers and every publish
// over every protocol refuses.
func TestEveryAudioCodecIsCarriedWhereItIsCoded(t *testing.T) {
	for _, a := range capabilities.AudioCodecs {
		for _, engine := range capabilities.Engines {
			if _, ok := a.EncoderOn(engine); !ok {
				continue
			}
			carried := slices.ContainsFunc(Names(), func(name string) bool {
				return CanPublishAudio(name, engine, a.Format)
			})
			if !carried {
				t.Errorf("audio codec %s is coded on the %s engine and no transport publishes it", a.Name, engine)
			}
		}
	}
}

// WebRTC negotiates Opus and has no SDP form for AAC on either engine, since what a session carries
// is the protocol's fact rather than the wrapped muxer's.
// The relay drops an AAC track that arrives anyway, so the refusal is what keeps a stream from
// publishing silent.
func TestValidatePublishAudioHoldsWebRTCToOpus(t *testing.T) {
	for _, engine := range capabilities.Engines {
		if err := ValidatePublishAudio("webrtc", engine, "opus"); err != nil {
			t.Errorf("webrtc carries opus on the %s engine: %v", engine, err)
		}
		err := ValidatePublishAudio("webrtc", engine, "aac")
		if err == nil {
			t.Errorf("webrtc has no AAC form and must refuse it on the %s engine", engine)
			continue
		}
		// The fix is another audio codec on the same leg, so the refusal names the one that carries.
		if !strings.Contains(err.Error(), "opus") {
			t.Errorf("refusal %q must name the audio codec webrtc does carry", err)
		}
	}
	// No track, no mapping to find, whatever the settings carry.
	for _, engine := range capabilities.Engines {
		if err := ValidatePublishAudio("webrtc", engine, capabilities.AudioNone); err != nil {
			t.Errorf("a stream without audio publishes over webrtc on the %s engine: %v", engine, err)
		}
	}
}

// The other three refusals: a codec no row carries, a transport publishing on the other engine
// alone, and a name the registry does not know.
// Each has its own fix, so each is refused here rather than handed to a muxer that would drop the
// track.
func TestValidatePublishAudioRefusesWhatNoLegCarries(t *testing.T) {
	if err := ValidatePublishAudio("srt", capabilities.EngineFfmpeg, "mp3"); err == nil {
		t.Error("an audio codec no row carries must be refused")
	}
	if err := ValidatePublishAudio("rtmp", capabilities.EngineGst, "aac"); err == nil {
		t.Error("rtmp has no GStreamer publish form and must refuse an audio track there")
	}
	if err := ValidatePublishAudio("nope", capabilities.EngineFfmpeg, "opus"); err == nil {
		t.Error("an unknown transport carries no audio track")
	}
	// SRT's MPEG-TS registers a stream type for both codecs on both engines, which is what lets an AAC
	// refusal elsewhere point at it.
	for _, engine := range capabilities.Engines {
		for _, audioCodec := range []string{"opus", "aac"} {
			if err := ValidatePublishAudio("srt", engine, audioCodec); err != nil {
				t.Errorf("srt carries %s on the %s engine: %v", audioCodec, engine, err)
			}
		}
	}
}
