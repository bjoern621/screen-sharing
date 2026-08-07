package wire

import (
	"slices"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/text"
)

// The codec half of the catalog: the three capability tables and the gaps that narrow
// them per engine.
//
// They sit beside the machine-side conversions rather than inside them because they
// are a projection of one page - docs/domain-model.md - and reading them together is
// how a reader checks that the projection lost nothing. Every row crosses whole: a row
// narrowed on the way over would be a second, quieter definition of the same codec.

// catalogCodecs converts the video capability table.
//
// Gaps cross alongside the row rather than being applied to it. Narrowing Chromas to
// what both engines encode would take a pixel format away from the engine that has it,
// with no reason shown anywhere; carrying the gap instead is what lets a form offer the
// format on that engine's capture backends and say who lacks it on the other.
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
// Nothing here is probed and nothing here forbids a choice: the table describes the
// viewers' hardware rather than this machine's, because a stream is published once and
// watched on whatever the watchers have. A shell reads it to say what a publish choice
// costs a viewer, which is a note on a control and never a greying.
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

// catalogAudioCodecs converts the audio table: what the second track can be coded in,
// and the element each engine codes it with.
//
// The sample rate and the bitrate are the row's rather than the user's. They cross so
// that a shell can state what a track will cost without a table of its own, and so
// that the figure it shows is the one both engines build their branch from.
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

// catalogGaps converts a row's gaps, normalizing the option each one names to the
// spelling a form control binds by (gapOptions).
//
// The reason crosses verbatim. It is written to name the limit and which side has it,
// so a shell shows it where the option would have been and the reader learns what to
// change rather than only that the option is gone.
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
// capabilities.Options names an option by the Go JSON tag settings.Stream carries
// ("chroma", "mode", "colorRange"); a form control names the same option by its proto
// field name, which is what StreamSettings spells it and what form/keys.go binds a
// widget by ("chroma", "mode", "color_range"). docs/ipc-api.md promises a shell that
// receives a gap can grey the matching control with no mapping of its own, so the
// normalization happens here, once, and both the gaps and the capability_options list
// are emitted through it, which is what keeps the list and the gaps from disagreeing.
var gapOptions = map[string]string{
	capabilities.OptionChroma:     "chroma",
	capabilities.OptionMode:       "mode",
	capabilities.OptionColorRange: "color_range",
}

// gapOptionName normalizes one option name. The empty string passes through, because a
// gap naming no option is the gap that takes the codec off an engine altogether and has
// no control to point at.
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

// catalogOptions lists the settings fields a Gap may name, in table order and in the
// spelling the gaps themselves arrive in.
func catalogOptions() []string {
	out := make([]string, 0, len(capabilities.Options))
	for _, option := range capabilities.Options {
		out = append(out, gapOptionName(option))
	}
	return out
}

// engineLimits converts the two per-engine numeric columns into one row per engine that
// bounds the codec by either of them.
//
// They were two maps keyed by engine name and are one typed row, because the contract
// carries the engine as an enum everywhere else and a map takes no enum key. Merging
// them here is what makes the row a fact about an engine rather than a column heading:
// a reader asks what GStreamer bounds this codec by and gets one answer.
//
// An engine that declares neither bound contributes no row, and a bound it does not
// declare stays absent on the row rather than crossing as nought. Absence is how the
// table says there is no quantizer scale or no bitrate ceiling, and zero is a number
// the encoder would have accepted.
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
