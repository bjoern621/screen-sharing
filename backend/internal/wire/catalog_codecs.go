package wire

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/text"
)

// The codec half of the catalog: the capability tables and the gaps that narrow them per engine.
//
// They sit beside the machine-side conversions rather than inside them because they project one
// page, docs/domain-model.md, and reading them together is how a reader checks that the projection
// lost nothing.
// Every row crosses whole: a row narrowed on the way over would be a second, quieter definition of
// the same codec.

// catalogCodecs converts the video capability table.
//
// Gaps cross alongside the row rather than being applied to it.
// Narrowing Chromas to what every engine encodes would take a pixel format away from the engine
// that has it, with no reason shown anywhere; carrying the gap lets a form offer the format on that
// engine's capture backends and name who lacks it on the others.
func catalogCodecs() []*screensharev1.VideoCodec {
	out := make([]*screensharev1.VideoCodec, 0, len(capabilities.Codecs))
	for _, c := range capabilities.Codecs {
		out = append(out, &screensharev1.VideoCodec{
			Name:        c.Name,
			Family:      c.Family,
			Format:      c.Format,
			Implemented: c.Implemented,
			Chromas:     slices.Clone(c.Chromas),
			Limits:      engineLimits(c.CqMax, c.BitrateLimitM),
			Gaps:        catalogGaps(c.Gaps),
		})
	}
	return out
}

// catalogDecoders converts the decode table.
//
// Nothing here is probed and nothing here forbids a choice: the table describes the viewers'
// hardware rather than this machine's, a stream being published once and watched on whatever the
// watchers have.
// A shell reads it to say what a publish choice costs a viewer, which is a note on a control and
// never a greying.
func catalogDecoders() []*screensharev1.Decoder {
	out := make([]*screensharev1.Decoder, 0, len(capabilities.Decoders))
	for _, d := range capabilities.Decoders {
		out = append(out, &screensharev1.Decoder{
			Element: d.Element,
			Family:  d.Family,
			Format:  d.Format,
			Chromas: slices.Clone(d.Chromas),
		})
	}
	return out
}

// catalogAudioCodecs converts the audio table: what the second track can be coded in, and the
// element each engine codes it with.
//
// The sample rate and the bitrate are the row's rather than the user's.
// They cross so a shell can state what a track costs without a table of its own, and so the figure
// it shows is the one both engines build their branch from.
func catalogAudioCodecs() []*screensharev1.AudioCodec {
	out := make([]*screensharev1.AudioCodec, 0, len(capabilities.AudioCodecs))
	for _, a := range capabilities.AudioCodecs {
		encoders := make([]*screensharev1.AudioEncoder, 0, len(a.Encoders))
		for _, e := range a.Encoders {
			encoders = append(encoders, &screensharev1.AudioEncoder{
				Engine:  engineEnum(e.Engine),
				Element: e.Element,
				Parser:  e.Parser,
			})
		}
		out = append(out, &screensharev1.AudioCodec{
			Name:        a.Name,
			Format:      a.Format,
			RateHz:      int32(a.Rate),
			BitrateKbps: int32(a.BitrateK),
			Encoders:    encoders,
			Gaps:        catalogGaps(a.Gaps),
		})
	}
	return out
}

// catalogGaps converts a row's gaps, normalizing the option each names to the spelling a form
// control binds by (gapOptions).
//
// The reason crosses verbatim.
// It names the limit and which side has it, so a shell shows it where the option would have been
// and the reader learns what to change rather than only that the option is gone.
func catalogGaps(gaps []capabilities.Gap) []*screensharev1.Gap {
	if len(gaps) == 0 {
		return nil
	}
	out := make([]*screensharev1.Gap, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, &screensharev1.Gap{
			Engine: gapEngine(g.Engine),
			Option: gapOptionName(g.Option),
			Value:  g.Value,
			Reason: text.Of(g.Reason),
		})
	}
	return out
}

// gapOptions is the single place the two spellings of a settings option meet.
//
// capabilities.Options names an option by the Go JSON tag settings.Settings carries ("colorRange");
// a form control names it by the proto field name StreamSettings spells and form/keys.go binds a
// widget by ("color_range").
// docs/ipc-api.md promises a shell that receives a gap can grey the matching control with no
// mapping of its own, so the normalization happens here, once, and the gaps and the
// capability_options list both leave through it rather than agreeing by coincidence.
var gapOptions = map[string]string{
	capabilities.OptionChroma:     "chroma",
	capabilities.OptionMode:       "mode",
	capabilities.OptionColorRange: "color_range",
	capabilities.OptionTune:       "tune",
}

// gapOptionName normalizes one option name.
// The empty string passes through: a gap naming no option is the one that takes the codec off an
// engine altogether, and it has no control to point at.
func gapOptionName(option string) string {
	if option == "" {
		return ""
	}
	name, ok := gapOptions[option]
	if !ok {
		assert.Never("a gap names a settings option the capability table declares", option)
	}
	return name
}

// catalogOptions lists the settings fields a Gap may name, in table order and spelled as the gaps
// themselves arrive.
func catalogOptions() []string {
	out := make([]string, 0, len(capabilities.Options))
	for _, option := range capabilities.Options {
		out = append(out, gapOptionName(option))
	}
	return out
}

// engineLimits converts the per-engine numeric columns into one row per engine that either of them
// bounds the codec by.
//
// One typed row rather than a map keyed by engine name, because the contract carries the engine as
// an enum everywhere else and a map takes no enum key.
// Merging the columns makes the row a fact about an engine rather than a column heading: a reader
// asks what GStreamer bounds this codec by and gets one answer.
//
// An engine that declares neither bound contributes no row, and a bound it does not declare stays
// absent on the row rather than crossing as nought.
// Absence is how the table says there is no quantizer scale and no bitrate ceiling; zero is a
// number the encoder would have accepted.
func engineLimits(cqMax, bitrateLimit map[string]int) []*screensharev1.EngineLimit {
	var out []*screensharev1.EngineLimit
	for _, engine := range capabilities.Engines {
		cq, hasCq := cqMax[engine]
		bitrate, hasBitrate := bitrateLimit[engine]
		if !hasCq && !hasBitrate {
			continue
		}

		limit := &screensharev1.EngineLimit{Engine: engineEnum(engine)}
		if hasCq {
			v := int32(cq)
			limit.CqMax = &v
		}
		if hasBitrate {
			v := int32(bitrate)
			limit.BitrateLimitMbps = &v
		}
		out = append(out, limit)
	}
	return out
}
