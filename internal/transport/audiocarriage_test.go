package transport

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// A carriage's audio names are capabilities.AudioCodec.Format values, the vocabulary
// ValidatePublishAudio matches the configured track against.
// A name outside it matches no codec, so the transport states an audio format nothing here encodes,
// and every audio publish over that leg is refused with a list of alternatives the name is not in.
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

// An audio codec is coded on an engine and carried by a transport, and both have to hold for the
// track to reach the relay.
// A codec no transport carries on the engine that codes it is a row the settings form offers and
// every publish over every protocol refuses.
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

// WebRTC negotiates Opus and has no SDP form for AAC, on either engine: which audio a session
// carries is a property of the protocol rather than of the muxer each engine wraps.
// A publish reaching the relay with an AAC track would have it dropped there,
// so the refusal is what keeps the stream from arriving silent.
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
		// The fix for an audio codec the leg lacks is another audio codec on the same leg,
		// so the refusal names the one that would have worked.
		if !strings.Contains(err.Error(), "opus") {
			t.Errorf("refusal %q must name the audio codec webrtc does carry", err)
		}
	}
	// A stream with no audio track has nothing to find a mapping for, whatever codec the settings
	// carry.
	for _, engine := range capabilities.Engines {
		if err := ValidatePublishAudio("webrtc", engine, capabilities.AudioNone); err != nil {
			t.Errorf("a stream without audio publishes over webrtc on the %s engine: %v", engine, err)
		}
	}
}

// The other three refusals are a codec no row carries, a transport that publishes on the other
// engine only, and a transport the registry does not know.
// Each names a different fix, so each is refused here rather than passed to a muxer that would drop
// the track.
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
	// SRT's MPEG-TS registers a stream type for both codecs on both engines, which is what makes it
	// the leg an AAC refusal elsewhere can point at.
	for _, engine := range capabilities.Engines {
		for _, audioCodec := range []string{"opus", "aac"} {
			if err := ValidatePublishAudio("srt", engine, audioCodec); err != nil {
				t.Errorf("srt carries %s on the %s engine: %v", audioCodec, engine, err)
			}
		}
	}
}
