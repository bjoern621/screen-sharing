package capabilities

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The audio half of the codec facts: which codec the second track is coded in, and how each publish
// engine codes it.
//
// A table for the reason the video one is.
// An audio codec is carried by some transports and not others, is coded by a different element on
// each engine, and pins the sample rate the capture branch resamples to.
// Written out at the two call sites, those facts drift the moment one engine gains a codec.

// AudioNone is the audio codec of a stream with no audio track.
// It is not a row: nothing to encode and nothing for a transport to carry, which is why every audio
// refusal passes it through rather than looking it up.
const AudioNone = "none"

// AudioEncoder is how one publish engine codes an audio codec.
type AudioEncoder struct {
	// Engine is one of Engines.
	Engine string `json:"engine"`
	// Element is the ffmpeg encoder name or the GStreamer element, spelled as its engine spells it.
	Element string `json:"element"`
	// Parser frames the coded stream for the muxer, because a GStreamer muxer pad negotiates framed
	// caps and an encoder's output does not carry them.
	// Empty on the ffmpeg engine, whose muxers take the encoder's packets directly.
	Parser string `json:"parser"`
}

// AudioCodec is one audio codec the publish leg can carry as its second track.
type AudioCodec struct {
	// Name is the settings value and the UI key: "opus", "aac".
	Name string `json:"name"`
	// Format is the name transports carry it under (transport.Carriage.Audio).
	// Separate from Name for the reason a video codec's Format is: a protocol carries a bitstream, not
	// the encoder that produced it.
	Format string `json:"format"`
	// Rate is the sample rate the capture branch resamples to before the encoder, in Hz.
	// It follows the codec: Opus codes at 48000 and nothing else.
	Rate int `json:"rate"`
	// BitrateK is what the track is coded at, in kbit/s.
	// Desktop audio is speech and system sounds over a stereo pair, which both codecs carry
	// transparently at this rate.
	BitrateK int `json:"bitrateK"`
	// Encoders is one entry per publish engine that codes this codec.
	// An engine absent from the list has no encoder for it and carries a Gap saying so.
	Encoders []AudioEncoder `json:"encoders"`
	// Gaps names the engines that cannot code this codec, with the reason.
	// Empty means every engine reaches it.
	Gaps []Gap `json:"gaps"`
}

// AudioCodecs is the audio capability table, in display order.
//
// A row states the element each engine codes it with rather than one name assumed to work on both,
// since two engines coding one bitstream are two different elements.
var AudioCodecs = []AudioCodec{
	{
		// The only codec WebRTC negotiates, so a stream keeps it whatever leg it is watched over.
		// Its encoders take 48 kHz alone, and its bitstream names no rate a decoder has to match.
		Name:     "opus",
		Format:   "opus",
		Rate:     48000,
		BitrateK: 128,
		Encoders: []AudioEncoder{
			{Engine: EngineFfmpeg, Element: "libopus"},
			{Engine: EngineGst, Element: "opusenc", Parser: "opusparse"},
		},
	},
	{
		// What the RTMP leg's FLV container carries, and what the players behind an RTMP or HLS URL
		// expect.
		// WebRTC negotiates no AAC, which is carriage and is stated by the transport rather than gapped
		// here.
		Name:     "aac",
		Format:   "aac",
		Rate:     48000,
		BitrateK: 128,
		Encoders: []AudioEncoder{
			{Engine: EngineFfmpeg, Element: "aac"},
			{Engine: EngineGst, Element: "avenc_aac", Parser: "aacparse"},
		},
	},
}

// AudioNames lists the audio codecs, in table order.
// It is the settings form's dropdown, greyed per engine and transport rather than narrowed here.
func AudioNames() []string {
	out := make([]string, 0, len(AudioCodecs))
	for _, a := range AudioCodecs {
		out = append(out, a.Name)
	}

	assert.Assert(len(out) == len(AudioCodecs), "every audio codec is named", len(out), len(AudioCodecs))
	return out
}

// GetAudio is the row under name, and false where no row carries it, AudioNone included.
func GetAudio(name string) (AudioCodec, bool) {
	for _, a := range AudioCodecs {
		if a.Name == name {
			return a, true
		}
	}
	return AudioCodec{}, false
}

// EncoderOn is how this engine codes the codec, and false where it has no encoder for it, which the
// row then states as a Gap.
func (a AudioCodec) EncoderOn(engine string) (AudioEncoder, bool) {
	assert.Assert(knownEngine(engine), "an audio encoder lookup names a publish engine", engine)

	for _, e := range a.Encoders {
		if e.Engine == engine {
			return e, true
		}
	}
	return AudioEncoder{}, false
}

// EngineGap is the gap that takes this audio codec off the named engine, and false where that
// engine codes it.
//
// It matches on the engine alone, as the codec table's EngineGap does.
// An audio codec has no per-option values, so a gap naming an option is a row this lookup cannot
// honour: skipping it would leave it reading as the engine-wide refusal it never declared, which is
// why it asserts instead.
func (a AudioCodec) EngineGap(engine string) (Gap, bool) {
	assert.Assert(knownEngine(engine), "an audio gap lookup names a publish engine", engine)

	for _, g := range a.Gaps {
		assert.Assert(g.Option == "", "an audio gap takes the codec off an engine rather than withholding an option", a.Name, g.Option)
		if g.covers(engine) {
			return g, true
		}
	}
	return Gap{}, false
}

// AudioNamesFor lists the audio codecs this engine codes whose bitstream is in the given carriage,
// in table order.
// It is what a refusal points at, so it narrows by both: a codec the engine cannot code and one the
// leg does not carry are equally useless as advice.
func AudioNamesFor(engine string, carried []string) []string {
	assert.Assert(knownEngine(engine), "an audio roster names a publish engine", engine)

	var out []string
	for _, a := range AudioCodecs {
		if _, ok := a.EncoderOn(engine); !ok {
			continue
		}
		if contains(carried, a.Format) {
			out = append(out, a.Name)
		}
	}
	return out
}

// ValidateAudio rejects an audio codec the engine has no encoder for.
// A stream with no audio track passes, having nothing to encode.
//
// Whether the publish transport carries the resulting track is the transport package's own refusal
// (transport.ValidatePublishAudio), which the same callers make beside this one.
// The codec is the user's and leaves as an error; the engine is the caller's own and asserts.
func ValidateAudio(engine, audioCodec string) error {
	assert.Assert(knownEngine(engine), "audio validation names a publish engine", engine)

	if audioCodec == AudioNone {
		return nil
	}
	a, ok := GetAudio(audioCodec)
	if !ok {
		return fmt.Errorf("unknown audio codec %q", audioCodec)
	}
	if _, ok := a.EncoderOn(engine); ok {
		return nil
	}
	gap, ok := a.EngineGap(engine)
	// A row an engine cannot code states why, so a surface greying it names the missing element rather
	// than reporting that the table is short a row.
	// The refusal here names identifiers alone, for the reason Validate's do.
	assert.Assert(ok, "an audio codec an engine cannot code states why", audioCodec, engine)
	assert.Assert(gap.Reason != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"an audio codec gap names which fact it is", audioCodec, engine)
	return fmt.Errorf("audio codec %s has no %s encoder", audioCodec, engine)
}
