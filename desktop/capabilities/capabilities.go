// Package capabilities is the single source of truth for the fixed facts about
// each video codec: the encoder family it runs on, which pixel formats it may
// encode, which rate-control modes its encoder implements, and the scale its
// constant-quality knob counts on.
//
// The two publish engines wrap different encoder implementations, so a codec or one
// value of one settings option can be one engine's and not the other's. Each such
// difference is a Gap naming the engine, the option, the value and the reason,
// rather than a fact narrowed to what both engines do: an option value one engine
// reaches stays offered there, and the engine that lacks it says why. Which options
// a gap may name is the Options list, so an axis is added by naming it there rather
// than by growing the Gap type.
//
// Which protocol carries a codec is not a fact about the encoder and is not
// modeled here. A protocol carries a bitstream format, so the transport package
// declares its own format set per leg and answers both directions from it.
//
// These facts are consumed on both sides of the wire. The ffmpeg argument builder
// reads them to branch and to reject an impossible combination, and the frontend
// fetches them (App.Capabilities) to grey out options the user cannot pick. One
// definition keeps the two in agreement: a codec's constraints cannot say one
// thing to the encoder and another to the UI.
//
// Presentation (labels, tooltips) and bitrate heuristics are the frontend's
// concern and are not modeled here.
package capabilities

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"
)

// The publish engines a capability can differ between. This package depends on
// nothing, which is what lets both engines and the frontend binding read it, so the
// names are declared here and the publish package's own constants take them from
// here rather than the other way round.
const (
	EngineFfmpeg = "ffmpeg"
	EngineGst    = "gstreamer"
)

// Engines lists every publish engine a Gap may name and a lookup may be asked about.
var Engines = []string{EngineFfmpeg, EngineGst}

// The encoder families a codec's backend belongs to. A family is the axis every
// per-backend fact keys off: the UI offers the family and the format as two separate
// choices, and each builder dispatches its family-wide behaviour off a table keyed
// this way rather than off a per-family flag or a name suffix.
const (
	FamilySoftware = "software"
	FamilyNvenc    = "nvenc"
	FamilyVaapi    = "vaapi"
	FamilyQsv      = "qsv"
	FamilyAmf      = "amf"
	FamilyV4l2     = "v4l2"
	FamilyRkmpp    = "rkmpp"
	FamilyVulkan   = "vulkan"
)

// Families lists every encoder family a codec row may declare.
var Families = []string{
	FamilySoftware, FamilyNvenc, FamilyVaapi, FamilyQsv,
	FamilyAmf, FamilyV4l2, FamilyRkmpp, FamilyVulkan,
}

// The rate-control modes an encode runs under. Three aim at a bitrate and differ in
// the ceiling, one aims at a quality, and one codes bit-exact.
const (
	ModeCbr      = "cbr"
	ModeVbr      = "vbr"
	ModeAbr      = "abr"
	ModeCrf      = "crf"
	ModeLossless = "lossless"
)

// Modes lists every rate-control mode a Gap may name and Validate may be given.
var Modes = []string{ModeCbr, ModeVbr, ModeAbr, ModeCrf, ModeLossless}

// The settings options a Gap may take a value away from. The names are the JSON
// field names settings.Stream carries, so a gap the table declares names the form
// control it greys without a translation step in between.
const (
	OptionChroma     = "chroma"
	OptionMode       = "mode"
	OptionColorRange = "colorRange"
)

// Options lists every option a Gap may name and Validate is given a value for.
// Adding one here, a refusal phrase below and a row on the codec that lacks it is
// the whole of declaring a new kind of gap: the lookup, the validator and the
// frontend read this list rather than a field per axis.
var Options = []string{OptionChroma, OptionMode, OptionColorRange}

// optionRefusals is how a gap on each option reads when it refuses a publish, with
// the refused value substituted. A phrase per option because "has no cbr
// rate-control mode" and "cannot encode pixel format yuv444p" are what the message
// has to say, and an option with no phrase is one Validate cannot report on, which
// it asserts rather than reports generically.
var optionRefusals = map[string]string{
	OptionChroma:     "cannot encode pixel format %s",
	OptionMode:       "has no %s rate-control mode",
	OptionColorRange: "cannot encode at colour range %s",
}

// knownOption reports whether option names a gappable settings option.
func knownOption(option string) bool {
	return contains(Options, option)
}

// knownEngine reports whether engine names a publish engine.
//
// Every lookup below asks about an engine the caller names itself, never one read
// off the settings or the frontend, so an engine outside this set is a caller that
// made one up and the lookups assert it.
// The codec, pixel format and mode they are asked about are the user's, and stay
// error-returning.
func knownEngine(engine string) bool {
	return contains(Engines, engine)
}

// EveryEngine states one numeric limit that every publish engine enforces, for
// the encoders whose knob is the library's rather than one engine's wrapper. It is
// how a shared fact is declared once without collapsing the per-engine axis: the
// row still answers per engine, and a row that later diverges names the engines
// separately without any consumer changing.
func EveryEngine(limit int) map[string]int {
	out := make(map[string]int, len(Engines))
	for _, engine := range Engines {
		out[engine] = limit
	}
	return out
}

// Gap is one thing a codec cannot do, with the reason the UI shows in place of the
// option. Engine names the publish engine the gap applies to; empty means every
// engine, i.e. the format or the library has no such capability rather than one
// builder failing to reach it.
//
// A gap takes one value of one option away: Option names which option, Value which
// of its values. A gap naming no option takes the codec off that engine altogether,
// for a format that engine has no encoder for at all.
//
// Which options exist is the Options list and nothing else. An axis is added by
// naming it there rather than by growing a field here, so a codec that cannot encode
// at a colour range is declared the same way as one that cannot encode a pixel
// format.
type Gap struct {
	// Engine is "ffmpeg", "gstreamer", or empty for both.
	Engine string `json:"engine"`
	// Option is the settings option this gap takes a value away from, one of
	// Options. Empty exactly when the gap takes the codec off the engine.
	Option string `json:"option"`
	// Value is the option value the engine's encoder will not take, empty exactly
	// when Option is.
	Value string `json:"value"`
	// Reason states which library or element lacks the capability. Where the other
	// engine has it, the reason says so, since switching capture backend is the
	// user's way to reach it.
	Reason string `json:"reason"`
}

// covers reports whether the gap binds on the named engine.
func (g Gap) covers(engine string) bool {
	return g.Engine == "" || g.Engine == engine
}

// Codec describes one video codec's fixed capabilities.
type Codec struct {
	// Name is the ffmpeg encoder name, e.g. "hevc_nvenc".
	Name string `json:"name"`
	// Family is the encoder family: the backend codecs in it share, so the UI can
	// offer the family and the format as two separate choices, and each builder
	// binds what a backend does once per family rather than once per codec. One of
	// Families.
	Family string `json:"family"`
	// Format is the video coding format independent of the backend: "h264",
	// "hevc", "av1", "vp9", "vp8". Facts that follow the format rather than the
	// backend (coding efficiency, browser decodability) key off this on the
	// frontend.
	Format string `json:"format"`
	// Implemented is true once the argument builders (encoderArgs, gstEncoder)
	// actually map this codec to a working command. A false entry appears in the
	// UI greyed out so the roadmap is visible, but BuildPublishArgs rejects it.
	Implemented bool `json:"implemented"`
	// Chromas lists every pixel format this codec may encode, on whichever publish
	// engine reaches the most of them. A format the other engine's encoder will not
	// take stays in the list and is declared as a Gap, so the UI can offer it where
	// it works and say who lacks it where it does not.
	Chromas []string `json:"chromas"`
	// CqMax is the highest value the constant-quality knob accepts per publish
	// engine, i.e. the scale the crf mode's quantizer target runs on. It follows the
	// encoder and not the format: the H.26x encoders reach 51, libvpx and the
	// software AV1 ones 63, and an encoder whose knob is a raw quantizer index
	// counts to 127 or 255.
	//
	// It is keyed by engine because the scale belongs to the property each engine
	// sets, not to the silicon underneath: ffmpeg's QSV encoders state a CQP
	// quantizer on the H.26x scale for every codec but AV1, where the qsv elements
	// pass the format's own index through. An engine with no entry has no scale
	// declared, which is the case for every family the argument builders do not map
	// yet, and the quantizer is then not bounded here.
	CqMax map[string]int `json:"cqMax"`
	// BitrateLimitM is the highest bitrate target the encoder accepts per publish
	// engine, in Mbit/s, for the modes that aim at one. An engine with no entry
	// takes any rate the machine can produce, which is the usual case. This is a
	// ceiling on the target, not the VBR burst ceiling the user sets above it.
	BitrateLimitM map[string]int `json:"bitrateLimitM"`
	// Gaps lists what this codec cannot do, per axis and per publish engine. Empty
	// means every chroma above and all five rate-control modes reach the encoder on
	// both engines.
	Gaps []Gap `json:"gaps"`
}

// OptionGap returns the gap that keeps this codec from encoding with value for the
// named option on the named engine, and false when that value reaches its encoder
// there. It is the one gap lookup: chroma, rate-control mode and colour range are
// asked about the same way, and so is any option Options gains.
//
// A chroma the codec cannot encode on either engine is absent from Chromas instead,
// which Validate rejects on its own.
func (c Codec) OptionGap(engine, option, value string) (Gap, bool) {
	assert.Assert(knownEngine(engine), "a gap lookup names a publish engine", engine)
	assert.Assert(knownOption(option), "a gap lookup names a gappable option", option)

	for _, g := range c.Gaps {
		if g.Option == option && g.Value == value && g.covers(engine) {
			return g, true
		}
	}
	return Gap{}, false
}

// EngineGap returns the gap that takes this codec off the named engine altogether,
// and false when that engine has an encoder for it. It is the gap that names no
// option: no value of any of them reaches an encoder that is not there.
func (c Codec) EngineGap(engine string) (Gap, bool) {
	assert.Assert(knownEngine(engine), "a gap lookup names a publish engine", engine)

	for _, g := range c.Gaps {
		if g.Option == "" && g.covers(engine) {
			return g, true
		}
	}
	return Gap{}, false
}

// CqMaxOn returns the top of this codec's constant-quality scale on the named
// engine, and 0 where the engine declares none.
func (c Codec) CqMaxOn(engine string) int {
	assert.Assert(knownEngine(engine), "a quantizer scale lookup names a publish engine", engine)

	return c.CqMax[engine]
}

// BitrateLimitOn returns the highest bitrate target this codec accepts on the
// named engine in Mbit/s, and 0 where the engine imposes no ceiling.
func (c Codec) BitrateLimitOn(engine string) int {
	assert.Assert(knownEngine(engine), "a bitrate ceiling lookup names a publish engine", engine)

	return c.BitrateLimitM[engine]
}

// EngineChromas returns the pixel formats this codec encodes on the named engine, in
// table order: Chromas minus the formats gapped there. An engine with no encoder for
// the codec at all encodes none of them, so an engine-wide gap answers with nothing
// rather than with the formats the other engine reaches.
func (c Codec) EngineChromas(engine string) []string {
	assert.Assert(knownEngine(engine), "a chroma list names a publish engine", engine)

	if _, gap := c.EngineGap(engine); gap {
		return nil
	}
	out := make([]string, 0, len(c.Chromas))
	for _, chroma := range c.Chromas {
		if _, gap := c.OptionGap(engine, OptionChroma, chroma); !gap {
			out = append(out, chroma)
		}
	}
	return out
}

// Validate rejects a codec and option-value combination this table forbids, so a
// settings object that no frontend normalized cannot reach an encoder.
// engine is the caller's own publish engine ("ffmpeg" or "gstreamer"), which
// decides which codecs and option values are available: both engines call this, so
// neither path can accept what the other rejects, and a capability only one of them
// reaches is refused for the other rather than silently approximated.
//
// options carries one value per entry in Options, keyed as Options names them. The
// caller takes them out of its own settings, which keeps this package free of
// dependencies, and a value left out is asserted rather than skipped: an option
// gained here and not supplied there would reach an encoder with its gaps unread.
//
// Whether the publish transport carries the resulting bitstream is the
// transport package's own refusal (transport.ValidatePublish), which the same
// callers make beside this one.
func Validate(engine, codec string, options map[string]string, cq, bitrateM int) error {
	// The engine and the option set are the caller's own; the values it is
	// validating are the user's, which is why only these two are asserts.
	assert.Assert(knownEngine(engine), "validation names a publish engine", engine)
	for _, option := range Options {
		_, ok := options[option]
		assert.Assert(ok, "validation carries a value for every gappable option", option)
	}
	chroma, mode := options[OptionChroma], options[OptionMode]

	c, ok := Get(codec)
	if !ok {
		return fmt.Errorf("unknown codec %q", codec)
	}
	if !c.Implemented {
		return fmt.Errorf("codec %s is listed but not implemented yet", c.Name)
	}
	// A mode outside the set matches no gap and would reach the builders, where the
	// rate-control switches end in the cbr branch: the encode would run at a rate
	// control the settings never asked for.
	if !contains(Modes, mode) {
		return fmt.Errorf("unknown rate-control mode %q", mode)
	}
	if gap, ok := c.EngineGap(engine); ok {
		return fmt.Errorf("codec %s has no %s encoder: %s", c.Name, engine, gap.Reason)
	}
	if !contains(c.Chromas, chroma) {
		return fmt.Errorf("codec %s cannot encode pixel format %s", c.Name, chroma)
	}
	// Every option is read the same way and in table order, so a codec gapped on two
	// of them names the first rather than the one an axis-by-axis validator happened
	// to check first.
	for _, option := range Options {
		gap, ok := c.OptionGap(engine, option, options[option])
		if !ok {
			continue
		}
		refusal, ok := optionRefusals[option]
		assert.Assert(ok, "a gappable option states how its refusal reads", option)
		return fmt.Errorf("codec %s %s on the %s engine: %s",
			c.Name, fmt.Sprintf(refusal, options[option]), engine, gap.Reason)
	}
	// The quantizer target reaches the encoder in crf mode only, and each
	// encoder's knob has its own scale: 60 is a valid libvpx CQ and an error on
	// x264. The scale is the running engine's, since the two set different
	// properties and one may pass a wider index through than the other clamps to.
	if cqMax := c.CqMaxOn(engine); mode == ModeCrf && cqMax > 0 && (cq < 0 || cq > cqMax) {
		return fmt.Errorf("quantizer target %d is outside codec %s's 0-%d range on the %s engine", cq, c.Name, cqMax, engine)
	}
	// The bitrate target reaches the encoder in the three bitrate modes only, so a
	// value left over from a lossless preset must not block a constant-quality
	// encode that never sends it.
	if limit := c.BitrateLimitOn(engine); targetsBitrate(mode) && limit > 0 && bitrateM > limit {
		return fmt.Errorf("bitrate target %d Mbit/s is above codec %s's %d Mbit/s ceiling on the %s engine", bitrateM, c.Name, limit, engine)
	}
	return nil
}

// targetsBitrate reports whether a rate-control mode aims at a bitrate the user
// sets. Constant quality and lossless spend whatever the picture costs, so the
// bitrate field means nothing to them.
func targetsBitrate(mode string) bool {
	return mode == ModeCbr || mode == ModeVbr || mode == ModeAbr
}

// Get returns the capabilities for name, or false if the codec is unknown.
func Get(name string) (Codec, bool) {
	for _, c := range Codecs {
		if c.Name == name {
			return c, true
		}
	}
	return Codec{}, false
}

// SupportsChroma reports whether codec name may encode the given pixel format on the
// named publish engine. A format the codec codes on the other engine only reports
// false here, matching what Validate accepts for that engine.
func SupportsChroma(name, engine, chroma string) bool {
	assert.Assert(knownEngine(engine), "a chroma question names a publish engine", engine)

	c, ok := Get(name)
	if !ok {
		return false
	}
	if !contains(c.Chromas, chroma) {
		return false
	}
	_, gap := c.OptionGap(engine, OptionChroma, chroma)
	return !gap
}

// HasFormat reports whether an implemented codec here produces this bitstream
// format. The transport package asks before narrowing a watch choice by format,
// so a relay path in a format this app never encodes narrows nothing instead of
// narrowing to nothing.
func HasFormat(format string) bool {
	return contains(Formats(), format)
}

// Formats lists the bitstream formats implemented codecs produce, in table order.
func Formats() []string {
	var out []string
	for _, c := range Codecs {
		if c.Implemented && !contains(out, c.Format) {
			out = append(out, c.Format)
		}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
