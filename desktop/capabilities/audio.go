package capabilities

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"
)

// The audio half of the codec facts: which codec the second track is encoded in,
// and how each publish engine codes it.
//
// It is a table for the same reason the video one is. An audio codec is carried
// by some transports and not others, is coded by a different element on each
// engine, and pins the sample rate the branch resamples to. Written out at the
// two call sites instead, those facts drift the moment one engine gains a codec.

// AudioNone is the audio codec of a stream with no audio track. It is not a row:
// there is nothing to encode and nothing for a transport to carry, which is why
// every audio refusal passes it through rather than looking it up.
const AudioNone = "none"

// AudioEncoder is how one publish engine codes an audio codec.
type AudioEncoder struct {
	// Engine is the publish engine, one of Engines.
	Engine string `json:"engine"`
	// Element is the ffmpeg encoder name or the GStreamer element that codes the
	// track, spelled as its engine spells it.
	Element string `json:"element"`
	// Parser is the GStreamer element that frames the coded stream for the muxer.
	// It is empty on the ffmpeg engine, whose muxers take the encoder's packets
	// directly, and a GStreamer entry states one because a muxer pad needs framed
	// caps to negotiate.
	Parser string `json:"parser"`
}

// AudioCodec is one audio codec the publish leg can carry as its second track.
type AudioCodec struct {
	// Name is the settings value and the UI key: "opus", "aac".
	Name string `json:"name"`
	// Format is the name transports carry it under (transport.Carriage.Audio). It
	// is separate from Name for the same reason a video codec's Format is: a
	// protocol carries a bitstream, not the encoder that produced it.
	Format string `json:"format"`
	// Rate is the sample rate the capture branch resamples to before the encoder,
	// in Hz. It follows the codec: Opus codes at 48 kHz and nothing else.
	Rate int `json:"rate"`
	// BitrateK is the target the track is coded at, in kbit/s. Desktop audio is
	// speech and system sounds over a stereo pair, which both codecs carry
	// transparently at this rate.
	BitrateK int `json:"bitrateK"`
	// Encoders is one entry per publish engine that codes this codec. An engine
	// absent from the list has no encoder for it and carries a Gap saying so.
	Encoders []AudioEncoder `json:"encoders"`
	// Gaps lists the engines that cannot code this codec, with the reason. Empty
	// means every engine reaches it.
	Gaps []Gap `json:"gaps"`
}

// AudioCodecs is the audio capability table. Order is the UI display order.
//
// Both codecs reach both engines, which is a fact and not a convenience: the
// entries state the element each engine uses, and the two are different elements
// coding the same bitstream rather than one name assumed to work on both.
var AudioCodecs = []AudioCodec{
	{
		// Opus: the one audio codec every hop here already handles, and the only
		// one WebRTC negotiates, so it is the codec a stream keeps whatever leg it
		// is watched over. Its bitstream carries no sample rate the decoder has to
		// match, and the encoders take 48 kHz alone.
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
		// AAC: what the RTMP leg's FLV container carried before the enhanced-RTMP
		// tags, and what the players behind an RTMP or HLS URL expect. WebRTC
		// negotiates no AAC at all, which is a carriage fact the transports state
		// rather than a gap here.
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

// AudioNames lists the audio codecs, in table order. It is the settings form's
// dropdown, greyed per engine and per transport rather than narrowed here.
func AudioNames() []string {
	out := make([]string, 0, len(AudioCodecs))
	for _, a := range AudioCodecs {
		out = append(out, a.Name)
	}
	return out
}

// GetAudio returns the audio codec registered under name, or false.
func GetAudio(name string) (AudioCodec, bool) {
	for _, a := range AudioCodecs {
		if a.Name == name {
			return a, true
		}
	}
	return AudioCodec{}, false
}

// EncoderOn returns how this engine codes the codec, and false when it has no
// encoder for it.
func (a AudioCodec) EncoderOn(engine string) (AudioEncoder, bool) {
	assert.Assert(knownEngine(engine), "an audio encoder lookup names a publish engine", engine)

	for _, e := range a.Encoders {
		if e.Engine == engine {
			return e, true
		}
	}
	return AudioEncoder{}, false
}

// EngineGap returns the gap that takes this audio codec off the named engine, and
// false when that engine codes it.
//
// It reads the gaps that name no option, as the codec table's EngineGap does. An
// audio codec has one axis and no per-option values, so a gap naming one is a row
// stating something this lookup cannot honour, and skipping it here would let it
// read as an engine-wide refusal it never declared.
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

// AudioNamesFor lists the audio codecs this engine codes whose bitstream is in
// the given carriage, in table order. It is what a refusal points at: the codecs
// that would have worked on the engine that is running and the leg it is
// publishing over.
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

// ValidateAudio rejects an audio codec the engine has no encoder for. A stream
// with no audio track passes: there is nothing to encode.
//
// Whether the publish transport carries the resulting track is the transport
// package's own refusal (transport.ValidatePublishAudio), which the same callers
// make beside this one.
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
	// A codec no engine entry codes states why, so a refusal names the missing
	// element rather than reporting that the table is short a row.
	assert.Assert(ok, "an audio codec an engine cannot code states why", audioCodec, engine)
	return fmt.Errorf("audio codec %s has no %s encoder: %s", audioCodec, engine, gap.Reason)
}
