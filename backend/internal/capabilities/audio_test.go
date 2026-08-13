package capabilities

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"slices"
	"testing"
)

// An engine either codes an audio codec or states why it cannot, never both and never neither.
// Neither is what ValidateAudio asserts on: a refusal with no gap has no missing element to name.
// Both is a row that puts the codec out of reach on an engine and hands that engine's builder an
// element anyway, which codes a track with an element the running engine does not have.
func TestEveryAudioCodecStatesAnEncoderOrAGapPerEngine(t *testing.T) {
	for _, a := range AudioCodecs {
		for _, engine := range Engines {
			_, encodes := a.EncoderOn(engine)
			_, gapped := a.EngineGap(engine)
			if encodes == gapped {
				t.Errorf("audio codec %s on the %s engine: encoder %v, gap %v, want exactly one",
					a.Name, engine, encodes, gapped)
			}
		}
	}
}

// An entry naming an engine no lookup is made with is never found, so the codec reads as uncoded on
// the engine that does code it and the refusal that follows asserts on a gap that is not there.
func TestEveryAudioEncoderNamesAKnownEngine(t *testing.T) {
	for _, a := range AudioCodecs {
		for _, e := range a.Encoders {
			if !slices.Contains(Engines, e.Engine) {
				t.Errorf("audio codec %s has an encoder on engine %q, which is not one of %v", a.Name, e.Engine, Engines)
			}
			if e.Element == "" {
				t.Errorf("audio codec %s states an encoder on the %s engine with no element", a.Name, e.Engine)
			}
		}
	}
}

// A GStreamer muxer pad negotiates framed caps and an encoder's output does not carry them, so
// every entry on that engine states the parser the audio branch puts between the two.
// The ffmpeg muxers take the encoder's packets directly, so a parser named there is an element name
// no builder reads.
func TestParsersFollowTheEngine(t *testing.T) {
	for _, a := range AudioCodecs {
		if enc, ok := a.EncoderOn(EngineGst); ok && enc.Parser == "" {
			t.Errorf("audio codec %s codes with %s on the gstreamer engine and states no parser to frame it",
				a.Name, enc.Element)
		}
		if enc, ok := a.EncoderOn(EngineFfmpeg); ok && enc.Parser != "" {
			t.Errorf("audio codec %s states parser %q on the ffmpeg engine, which mixes packets rather than framed caps",
				a.Name, enc.Parser)
		}
	}
}

// The rate and the bitrate ride into both builders' commands verbatim, so a row leaving one out
// sends "-ar 0" and "rate=0".
func TestEveryAudioCodecStatesARateAndABitrate(t *testing.T) {
	for _, a := range AudioCodecs {
		if a.Name == "" || a.Format == "" {
			t.Errorf("an audio row names itself and the format it is carried under, got %q and %q", a.Name, a.Format)
		}
		if a.Rate <= 0 {
			t.Errorf("audio codec %s states sample rate %d", a.Name, a.Rate)
		}
		if a.BitrateK <= 0 {
			t.Errorf("audio codec %s states bitrate %d", a.Name, a.BitrateK)
		}
	}
}

// AudioNone is the absent track rather than a row, and every audio refusal passes it through.
// A row under that name is a codec the builders encode a stream that has no audio in.
func TestAudioNoneIsNotARow(t *testing.T) {
	if _, ok := GetAudio(AudioNone); ok {
		t.Errorf("%q is the absent track and must not be a row", AudioNone)
	}
	if slices.Contains(AudioNames(), AudioNone) {
		t.Errorf("AudioNames() = %v, must not offer the absent track as a codec", AudioNames())
	}
	for _, engine := range Engines {
		if err := ValidateAudio(engine, AudioNone); err != nil {
			t.Errorf("a stream without audio has nothing to encode on the %s engine: %v", engine, err)
		}
	}
}

// ValidateAudio answers for the engine that is running: an unknown codec is refused, and a codec
// the table carries passes on every engine that codes it.
func TestValidateAudio(t *testing.T) {
	for _, engine := range Engines {
		if err := ValidateAudio(engine, "mp3"); err == nil {
			t.Errorf("an audio codec no row carries must be refused on the %s engine", engine)
		}
		for _, a := range AudioCodecs {
			if _, ok := a.EncoderOn(engine); !ok {
				continue
			}
			if err := ValidateAudio(engine, a.Name); err != nil {
				t.Errorf("%s is coded on the %s engine: %v", a.Name, engine, err)
			}
		}
	}
}

// A refusal points at the codecs that would have worked on the engine that is running and the leg
// it publishes over, so the roster is narrowed by both.
// A codec the engine cannot code and one the carriage does not hold are equally useless as advice.
func TestAudioNamesForNarrowsByEngineAndCarriage(t *testing.T) {
	if got := AudioNamesFor(EngineFfmpeg, []string{"opus"}); !slices.Equal(got, []string{"opus"}) {
		t.Errorf("AudioNamesFor(ffmpeg, [opus]) = %v, want opus alone", got)
	}
	if got := AudioNamesFor(EngineGst, nil); len(got) != 0 {
		t.Errorf("AudioNamesFor(gstreamer, nothing carried) = %v, want an empty roster", got)
	}
	// A carriage naming only formats outside the table narrows to nothing rather than to everything.
	if got := AudioNamesFor(EngineFfmpeg, []string{"mp3"}); len(got) != 0 {
		t.Errorf("AudioNamesFor(ffmpeg, [mp3]) = %v, want an empty roster", got)
	}
	// Table order, which is the order the settings form shows.
	all := make([]string, 0, len(AudioCodecs))
	for _, a := range AudioCodecs {
		all = append(all, a.Format)
	}
	if got := AudioNamesFor(EngineFfmpeg, all); !slices.Equal(got, AudioNames()) {
		t.Errorf("AudioNamesFor(ffmpeg, everything carried) = %v, want the table order %v", got, AudioNames())
	}
}

// A gap's reason is what a surface reads out, so an unset one leaves a sentence naming no missing
// element.
func TestEveryAudioGapStatesAReasonAndAKnownEngine(t *testing.T) {
	for _, a := range AudioCodecs {
		for _, g := range a.Gaps {
			if g.Engine != "" && !slices.Contains(Engines, g.Engine) {
				t.Errorf("audio codec %s has a gap on engine %q, which is not one of %v", a.Name, g.Engine, Engines)
			}
			if g.Reason == screensharev1.TextCode_TEXT_CODE_UNSPECIFIED {
				t.Errorf("audio codec %s has a gap on engine %q with no reason", a.Name, g.Engine)
			}
			// Every gappable option is a video one and EngineGap matches on the engine alone, so a gap
			// naming an option takes the codec off that engine while reading as if it withheld one value.
			if g.Option != "" || g.Value != "" {
				t.Errorf("audio codec %s has a gap on option %q value %q, which binds engine-wide anyway",
					a.Name, g.Option, g.Value)
			}
		}
	}
}
