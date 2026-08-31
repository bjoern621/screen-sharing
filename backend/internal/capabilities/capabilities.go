// Package capabilities is the single source of truth for the fixed facts about each video codec:
// the encoder family it runs on, which pixel formats it may encode,
// which rate-control modes its encoder implements,
// and the scale its constant-quality knob counts on.
//
// The two publish engines wrap different encoder implementations,
// so a codec, or one value of one settings option, can be one engine's and not the other's.
// Each difference is a Gap naming the engine, the option, the value and the reason,
// rather than a fact narrowed to what both engines do:
// an option value one engine reaches stays offered there, and the engine that lacks it says why.
// Which options a gap may name is the Options list,
// so an axis is added by naming it there rather than by growing the Gap type.
//
// A Gap and a numeric ceiling are how those facts are written, on the codec row they belong to.
// They are not what anything reads:
// rules.go turns each into a rule in the one evaluator (internal/rules),
// and the greying, the offered range and the refusal in Validate read one evaluation.
// The split exists because a fact about a codec and a capture backend together,
// or a codec and a platform, has no row to sit on,
// and a fact with no row grows a table of its own with a consumer written against it.
//
// Which protocol carries a codec is not a fact about the encoder and is not modeled here.
// A protocol carries a bitstream format,
// so the transport package declares its own format set per leg and answers both directions from it.
//
// These facts are consumed on both sides of the wire:
// the argument builders branch on them and reject an impossible combination,
// and a shell reads them (App.Capabilities) to grey out what cannot be picked.
// One definition keeps a codec's constraints from differing between the encoder and a form.
//
// The words are not here.
// Labels, tooltips and refusal sentences are written where the layout is,
// keyed by the identifiers these rows use (docs/ipc-api.md).
//
// Every row is a compiled-in fact,
// so a malformed one is an Entwicklungsfehler and the lookups assert on it.
// What a user picked arrives from a settings file and stays error-returning.
package capabilities

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/rules"
)

// The publish engines a capability can differ between.
// This package depends on the rule vocabulary and on nothing else in the domain,
// which is what lets every consumer read it,
// so the names are declared here and the publish package's own constants take them from here.
const (
	EngineFfmpeg = "ffmpeg"
	EngineGst    = "gstreamer"
)

// Engines lists every publish engine a Gap may name and a lookup may be asked about.
var Engines = []string{EngineFfmpeg, EngineGst}

// The encoder families a codec's backend belongs to.
// A family is the axis every per-backend fact keys off:
// a surface names the family and the format to tell one catalog row from another,
// and each builder dispatches its family-wide behaviour off a table keyed this way,
// rather than off a per-family flag or a name suffix.
const (
	FamilySoftware     = "software"
	FamilyNvenc        = "nvenc"
	FamilyVaapi        = "vaapi"
	FamilyQsv          = "qsv"
	FamilyAmf          = "amf"
	FamilyV4l2         = "v4l2"
	FamilyRkmpp        = "rkmpp"
	FamilyVulkan       = "vulkan"
	FamilyVideoToolbox = "videotoolbox"
)

// Families lists every encoder family a codec row may declare.
var Families = []string{
	FamilySoftware, FamilyNvenc, FamilyVaapi, FamilyQsv,
	FamilyAmf, FamilyV4l2, FamilyRkmpp, FamilyVulkan, FamilyVideoToolbox,
}

// The rate-control modes an encode runs under.
// Some aim at a bitrate and differ in the ceiling they hold it under,
// one aims at a quality, and one codes bit-exact.
// Which aim at a bitrate is targetsBitrate.
const (
	ModeCbr      = "cbr"
	ModeVbr      = "vbr"
	ModeAbr      = "abr"
	ModeCrf      = "crf"
	ModeLossless = "lossless"
)

// Modes lists every rate-control mode a Gap may name and Validate may be given.
var Modes = []string{ModeCbr, ModeVbr, ModeAbr, ModeCrf, ModeLossless}

// The quantization ranges a picture may be coded at:
// every code value carrying image data, or the 16-235 studio swing a broadcast chain expects.
//
// Declared here for the reason the modes are.
// The colour range is an option a Gap may take a value away from,
// and a table naming the axis without naming its values leaves every consumer to type them,
// where the one that types them differently greys the wrong option.
const (
	ColorRangeFull    = "pc"
	ColorRangeLimited = "tv"
)

// ColorRanges lists every colour range a Gap may name and Validate may be given.
var ColorRanges = []string{ColorRangeFull, ColorRangeLimited}

// The settings options a Gap may take a value away from.
// Each is the JSON field name settings.Settings carries,
// so a gap and the form control it greys are the same identifier on both sides of the wire.
const (
	OptionChroma     = "chroma"
	OptionMode       = "mode"
	OptionColorRange = "colorRange"
	// OptionTune is a step of the row's tune ladder.
	// Gappable because a tune knob is an encoder wrapper's and not the library's:
	// libaom takes a tune on ffmpeg where av1enc exposes no such property,
	// and oneVPL's scenario reaches the qsv encoders through ffmpeg alone.
	// Gapping the steps leaves TuneNone standing, which is what an engine without the knob spends.
	OptionTune = "tune"
)

// Options lists every option a Gap may name and Validate is given a value for.
// Naming one here, a refusal phrase below and a row on the codec that lacks it,
// is the whole of declaring another kind of gap:
// the lookup, the validator and every surface read this list rather than a field per axis.
var Options = []string{OptionChroma, OptionMode, OptionColorRange, OptionTune}

// optionRefusals is how a gap on each option reads when it refuses a publish,
// with the refused value substituted.
// A phrase per option, "has no cbr rate-control mode" and "cannot encode pixel format yuv444p"
// being what the message says.
// An option with no phrase is one Validate asserts on rather than refuses generically.
var optionRefusals = map[string]string{
	OptionChroma:     "cannot encode pixel format %s",
	OptionMode:       "has no %s rate-control mode",
	OptionColorRange: "cannot encode at colour range %s",
	OptionTune:       "does not tune for %s",
}

func knownOption(option string) bool {
	return contains(Options, option)
}

// knownEngine is the line between the two failure kinds here.
// Every lookup below asks about an engine the caller names itself, never one read off the settings,
// so an engine outside this set is an Entwicklungsfehler and asserts.
// The codec, pixel format and mode they are asked about are the user's and stay error-returning.
func knownEngine(engine string) bool {
	return contains(Engines, engine)
}

// EveryEngine states one numeric limit under every publish engine,
// for the encoders whose knob is the library's rather than one wrapper's.
// It declares a shared fact once without collapsing the per-engine axis:
// the row still answers per engine,
// and a row that diverges names the engines separately with no consumer changing.
func EveryEngine(limit int) map[string]int {
	out := make(map[string]int, len(Engines))
	for _, engine := range Engines {
		out[engine] = limit
	}

	assert.Assert(len(out) == len(Engines), "a shared limit is stated for every engine", len(out))
	return out
}

// Gap is one thing a codec cannot do, with the reason a surface shows in place of the option.
//
// A gap takes one value of one option away: Option names which option, Value which of its values.
// A gap naming no option takes the codec off that engine altogether,
// for a format that engine has no encoder for at all.
//
// Which options exist is the Options list and nothing else,
// so a codec that cannot encode at a colour range is declared like one that cannot encode a chroma.
type Gap struct {
	// Engine is "ffmpeg", "gstreamer", or empty for both.
	// Empty says the format or the library has no such capability,
	// rather than one builder failing to reach it.
	Engine string `json:"engine"`
	// Option is one of Options, and empty exactly when the gap takes the codec off the engine.
	Option string `json:"option"`
	// Value is the option value the engine's encoder will not take, empty exactly when Option is.
	Value string `json:"value"`
	// Reason names which library or element lacks the capability,
	// as a code rather than a sentence about it (api/proto/screenshare/v1/text.proto).
	// One code per row and no arguments:
	// the three fields above already say which codec, which engine and which value,
	// so a surface rendering the code has everything the sentence names.
	Reason screensharev1.TextCode `json:"reason"`
}

func (g Gap) covers(engine string) bool {
	return g.Engine == "" || g.Engine == engine
}

// Ladder is one encoder knob's steps, and where each rate-control mode starts on it.
//
// The steps are the encoder's own identifiers rather than a normalized scale,
// a normalized one lying:
// x264's "slow" and SVT-AV1's preset 8 are not the same trade,
// and a number carried across a codec change would land on a different real setting.
// A shell names each step and renders one it has no name for as the identifier,
// which lets a codec gain a ladder with nothing edited there (docs/domain-model.md).
//
// Steps run from the most effort to the least, whatever direction the encoder's own numbering takes,
// so a control is drawn without knowing which way each ladder counts.
type Ladder struct {
	// Steps are the values the encoder takes, most effort first.
	// Empty is an encoder without the knob, which greys the control with the reason.
	Steps []string `json:"steps"`
	// Defaults is the step each rate-control mode starts on, keyed as Modes names them.
	// A mode with no entry leaves the knob unset,
	// which is the encoder's own default rather than a step of this ladder.
	Defaults map[string]string `json:"defaults"`
	// Pins are the modes that fix the step rather than starting on it,
	// so the control is greyed there and the statement names the step in force.
	//
	// A fact about the encoder and not about the mode:
	// NVENC pins its preset in CBR, a low-latency preset being what lets it hold a constant rate,
	// and x264 in the same mode takes whatever step it is given.
	// A flag on the mode would grey every codec's control wherever one of them pins.
	Pins []string `json:"pins"`
}

func (l Ladder) PinsIn(mode string) bool {
	return contains(l.Pins, mode)
}

// StepFor is where a mode starts on this ladder,
// and false where the ladder leaves the knob to the encoder in that mode.
func (l Ladder) StepFor(mode string) (string, bool) {
	step, ok := l.Defaults[mode]
	return step, ok
}

func (l Ladder) Has(step string) bool {
	return contains(l.Steps, step)
}

// Resolve is what one encode spends on this ladder, and false for a step the encoder does not take.
//
// An empty answer means spend nothing and leave the knob at the encoder's own default.
// A ladder with no steps is an encoder without the knob,
// and a mode the Defaults leave out is one that default is right for.
//
// A held step wins wherever the ladder carries it,
// which is what makes the control a choice rather than a display of the row.
// It loses in a mode that pins the knob, and a pinned mode refuses nothing:
// the control is greyed there, so the value behind the greying is one nothing spends,
// and refusing a publish over it would name a field nobody can reach.
func (l Ladder) Resolve(mode, held string) (string, bool) {
	if len(l.Steps) == 0 {
		return "", true
	}
	if l.PinsIn(mode) || held == "" {
		step, _ := l.StepFor(mode)
		return step, true
	}
	if l.Has(held) {
		return held, true
	}
	return "", false
}

// Steps are the two ladder steps one encode spends, in the encoder's own identifiers.
// Either is empty where its ladder leaves the knob alone.
type Steps struct {
	Effort, Tune string
}

// ResolveSteps is what an encode of this codec in this mode spends on both ladders,
// and a refusal for a step the encoder does not take.
//
// Both builders read it, which is the point.
// The step is a fact about the encoder, not of the engine that builds the command.
// An encode that followed the engine would tie a stream's picture to its capture backend.
// Each engine still owns the spelling: ffmpeg's -preset, the x264 element's speed-preset,
// and the empty step, which every builder answers by passing no such option at all.
//
// A refusal is an Umgebungsfehler,
// keeping a hand-edited settings file off an encoder's own error path:
// the field is free-form text,
// the repair walks a step off the ladder back onto it,
// and nothing between the two bounds a value that reached the settings by another route.
func (c Codec) ResolveSteps(mode, effort, tune string) (Steps, error) {
	e, ok := c.Effort.Resolve(mode, effort)
	if !ok {
		return Steps{}, fmt.Errorf("codec %s takes no effort step %q, only %s",
			c.Name, effort, strings.Join(c.Effort.Steps, ", "))
	}
	t, ok := c.Tune.Resolve(mode, tune)
	if !ok {
		return Steps{}, fmt.Errorf("codec %s takes no tune step %q, only %s",
			c.Name, tune, strings.Join(c.Tune.Steps, ", "))
	}
	return Steps{Effort: e, Tune: t}, nil
}

// everyMode spreads one step across every rate-control mode,
// for a ladder whose step does not follow what the encoder is aiming at.
func everyMode(step string) map[string]string {
	assert.Assert(step != "", "a ladder default names the step it spends")

	out := make(map[string]string, len(Modes))
	for _, mode := range Modes {
		out[mode] = step
	}

	assert.Assert(len(out) == len(Modes), "a spread step covers every rate control", len(out))
	return out
}

// Codec describes one video codec's fixed capabilities.
type Codec struct {
	// Name is the ffmpeg encoder name: "hevc_nvenc".
	Name string `json:"name"`
	// Family is the backend the codecs in it share, one of Families.
	// A surface names the family and the format to tell one row from another,
	// and each builder binds what a backend does once per family rather than once per codec.
	Family string `json:"family"`
	// Library is what this row's encoder is called where its family holds more than one,
	// spelled as the project names itself: "x264", "svt-av1".
	// Empty on a family that is one encoder, whose own name is what names it (Encoder).
	Library string `json:"library"`
	// Format is the coding format independent of the backend: "h264", "hevc", "av1", "vp9", "vp8".
	// Facts that follow the format rather than the backend key off this,
	// coding efficiency and decodability among them.
	Format string `json:"format"`
	// Implemented is true where the argument builders map this codec to a working command
	// (encoderArgs, gstEncoder).
	// A false row is offered greyed, so a codec no builder reaches is visible,
	// and BuildPublishArgs rejects it.
	Implemented bool `json:"implemented"`
	// Chromas is every pixel format this codec may encode,
	// on whichever publish engine reaches the most of them.
	// A format the other engine's encoder will not take stays in the list and is declared as a Gap,
	// so it is offered where it works and greyed with the missing element where it does not.
	Chromas []string `json:"chromas"`
	// CqMax is the top of the constant-quality scale per publish engine,
	// which is what the crf mode's quantizer target counts on.
	// It follows the encoder rather than the format:
	// the H.26x encoders reach 51, libvpx and the software AV1 ones 63,
	// and an encoder whose knob is a raw quantizer index counts to 127 or 255.
	//
	// Keyed by engine, the scale belonging to the property each engine sets and not to the silicon:
	// ffmpeg's QSV encoders state a CQP quantizer on the H.26x scale for every codec but AV1,
	// where the qsv elements pass the format's own index through.
	// An engine with no entry declares no scale, which leaves the quantizer unbounded here.
	CqMax map[string]int `json:"cqMax"`
	// BitrateLimitM is the highest bitrate target the encoder accepts per publish engine, in Mbit/s,
	// for the modes that aim at one.
	// An engine with no entry takes any rate the machine can produce.
	// It bounds the target and not the VBR burst ceiling set above it.
	BitrateLimitM map[string]int `json:"bitrateLimitM"`
	// BufferLimitKb is the largest rate buffer the encoder's own field holds per publish engine,
	// in kilobits.
	// The buffer reaches the encoder as the rate times the window,
	// so this bounds the pair rather than either half,
	// and the window is the half a form narrows, the rate being the figure somebody chose
	// (form.fieldVbvBounds).
	// An engine with no entry holds whatever a 32-bit field does.
	BufferLimitKb map[string]int `json:"bufferLimitKb"`
	// GopLimit is the longest keyframe interval the encoder's own field holds per publish engine,
	// in frames.
	// An engine with no entry takes any interval the control offers.
	// The encoder refuses an interval above it rather than coding a shorter one,
	// so it bounds what the control offers and what a publish is refused for.
	GopLimit map[string]int `json:"gopLimit"`
	// Effort is the speed-against-quality ladder this encoder takes, and where each mode starts on it.
	Effort Ladder `json:"effort"`
	// Tune is what the encoder optimizes for, a different question from how hard it works:
	// a live encode drops the lookahead and the frame reordering a quality one keeps,
	// whatever effort it is spending.
	// An encoder with both takes the tune step beside the effort step rather than instead of it.
	Tune Ladder `json:"tune"`
	// Gaps is what this codec cannot do, per axis and per publish engine.
	// Empty means every chroma above and every rate-control mode reaches the encoder on both engines.
	Gaps []Gap `json:"gaps"`
	// DriverDefects is what this codec's encoder does reach and one installed driver miscodes.
	DriverDefects []DriverDefect `json:"driverDefects"`
}

// DriverDefect is one value this codec's encoder declares, other drivers run,
// and the driver named here gets wrong badly enough to withhold it.
//
// Separate from Gap, the two answering different questions.
// A gap is what an encoder cannot do and holds wherever this app runs,
// so the engine-scoped lookups read Gaps alone,
// and a builder enumerating what an element implements gets the same answer on every machine.
// A defect is what one driver gets wrong and holds only while that driver sits under the encoder,
// so it reaches a form and a publish through the evaluator,
// the reader that carries the machine's own facts.
//
// A defect is written down rather than probed for where trying it is the damage:
// an encode that hangs the graphics device cannot be looked up the way a missing element is,
// so the driver is identified (internal/gpu) and matched.
type DriverDefect struct {
	// Driver is the implementation carrying the defect, spelled as it names itself: "radeonsi".
	Driver string `json:"driver"`
	// Models narrows the defect to the adapters that carry it,
	// each matched whole against the name the driver reports.
	// Empty covers every adapter the driver drives,
	// which is what a defect in the driver's own code rather than in one generation of silicon takes.
	Models []string `json:"models"`
	// Option and Value are what the defect withholds, named as a Gap names them.
	Option string `json:"option"`
	Value  string `json:"value"`
	// FixedIn is the first driver release without the defect, packed as gpu.Version packs one.
	// Zero withholds the value on every release, for a defect no version is known to have fixed.
	FixedIn int `json:"fixedIn"`
	// Reason names the defect, as Gap.Reason names a gap.
	Reason screensharev1.TextCode `json:"reason"`
}

// Device is the video driver a capability question is asked about.
//
// The reading is internal/gpu's and the shape is declared here,
// so this package depends on the rule vocabulary and on nothing else in the domain.
// The zero Device is a driver nothing identified, which matches no defect:
// a machine that could not name its driver keeps every option the encoder declares.
type Device struct {
	// Driver is the implementation, spelled as it names itself: "radeonsi".
	Driver string `json:"driver"`
	// Model is the adapter it drives.
	// Example: "AMD Radeon 780M Graphics".
	Model string `json:"model"`
	// Version is the driver's release packed into one comparable figure: 26.1.6 reads 26001006.
	Version int `json:"version"`
}

// OptionGap is the gap keeping this codec from encoding with value for the named option,
// on the named engine, and false where that value reaches its encoder there.
// The one gap lookup:
// chroma, rate-control mode, colour range and any option Options gains are asked about the same way.
//
// A chroma the codec encodes on neither engine is absent from Chromas instead,
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

// WithheldByDriver reports whether the driver on device takes this option value away,
// where the encoder itself implements it.
//
// The question OptionGap does not answer:
// a gap is the encoder's and holds everywhere,
// a defect is one driver's and holds where that driver is installed.
// Routed through the evaluator rather than walking DriverDefects,
// so this and the refusal Validate returns cannot come apart.
func (c Codec) WithheldByDriver(device Device, engine, option, value string) bool {
	assert.Assert(knownEngine(engine), "a driver question names a publish engine", engine)
	axis, ok := optionAxes[option]
	assert.Assert(ok, "a driver question names a gappable option", option)

	v := rules.EvaluateRules(
		validationFacts(c, engine, map[string]string{option: value}, 0, 0, 0, device), codecRules())
	return refusedByDriver(v, axis, value)
}

// EngineGap is the gap that takes this codec off the named engine altogether,
// and false where that engine has an encoder for it.
// The gap naming no option, no value of any reaching an encoder that is not there.
func (c Codec) EngineGap(engine string) (Gap, bool) {
	assert.Assert(knownEngine(engine), "a gap lookup names a publish engine", engine)

	for _, g := range c.Gaps {
		if g.Option == "" && g.covers(engine) {
			return g, true
		}
	}
	return Gap{}, false
}

// CqMaxOn is the top of this codec's constant-quality scale on the named engine,
// and zero where the row declares none for it.
func (c Codec) CqMaxOn(engine string) int {
	assert.Assert(knownEngine(engine), "a quantizer scale lookup names a publish engine", engine)

	return c.CqMax[engine]
}

// BitrateLimitOn is the highest bitrate target this codec accepts on the named engine, in Mbit/s,
// and zero where that engine imposes no ceiling.
func (c Codec) BitrateLimitOn(engine string) int {
	assert.Assert(knownEngine(engine), "a bitrate ceiling lookup names a publish engine", engine)

	return c.BitrateLimitM[engine]
}

// BufferLimitOn is the largest rate buffer this codec's encoder field holds on the named engine,
// in kilobits, and zero where that engine imposes none of its own.
func (c Codec) BufferLimitOn(engine string) int {
	assert.Assert(knownEngine(engine), "a rate buffer ceiling lookup names a publish engine", engine)

	return c.BufferLimitKb[engine]
}

// GopLimitOn is the longest keyframe interval this codec's encoder holds on the named engine,
// in frames, and zero where that engine imposes no ceiling.
func (c Codec) GopLimitOn(engine string) int {
	assert.Assert(knownEngine(engine), "a keyframe interval ceiling lookup names a publish engine", engine)

	return c.GopLimit[engine]
}

// EngineChromas is the pixel formats this codec encodes on the named engine, in table order:
// Chromas minus the formats gapped there.
// An engine-wide gap answers with nothing rather than with the formats the other engine reaches,
// since an engine with no encoder for the codec encodes none of them.
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

// Validate rejects a codec and option-value combination this table forbids,
// so a settings object nothing repaired cannot reach an encoder.
//
// engine is the caller's own publish engine,
// and decides which codecs and option values are available.
// Both engines call this, so neither path accepts what the other rejects,
// and a capability only one of them reaches is refused for the other rather than approximated.
//
// options carries one value per entry in Options, keyed as Options names them.
// The caller takes them out of its own settings, which keeps this package free of dependencies.
// A value left out asserts rather than being skipped:
// an option gained here and not supplied there would reach an encoder with its gaps unread.
//
// device is the driver the encode would run on (gpu.Device), and decides the DriverDefects rows.
// The zero Device is a machine that named no driver, carrying no defect and refusing nothing.
//
// Whether the publish transport carries the resulting bitstream is the transport package's own
// refusal (transport.ValidatePublish), which the same callers make beside this one.
func Validate(engine, codec string, options map[string]string, cq, bitrateM, gop int, device Device) error {
	// The engine and the option set are the caller's own, so both are Entwicklungsfehler and assert.
	// The values being validated are the user's, and every one of them leaves as an error.
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
		return fmt.Errorf("codec %s is listed without an implementation", c.Name)
	}
	// A mode outside the set matches no gap and would reach the builders,
	// whose rate-control switches end in the cbr branch,
	// so the encode would run at a rate control the settings never asked for.
	if !contains(Modes, mode) {
		return fmt.Errorf("unknown rate-control mode %q", mode)
	}
	// Every refusal below is the rules' answer rather than a second reading of the columns they were
	// built from, which is what keeps a greyed control and a refused publish in step:
	// the offered range, the greyed entry and the error here read one evaluation.
	//
	// The refusals name identifiers and nothing else.
	// They are Umgebungsfehler and the same text crosses as a gRPC status,
	// so why a limit exists is a statement a surface makes from the rule,
	// rather than a sentence quoted into an error string (docs/ipc-api.md).
	v := rules.EvaluateRules(validationFacts(c, engine, options, cq, bitrateM, gop, device), codecRules())

	if !v.ValueEnabled(rules.AxisCodec, c.Name) {
		return fmt.Errorf("codec %s has no %s encoder", c.Name, engine)
	}
	if !contains(c.Chromas, chroma) {
		return fmt.Errorf("codec %s cannot encode pixel format %s", c.Name, chroma)
	}
	// Every option is read the same way and in table order,
	// so a codec refused on two of them names the first,
	// rather than the one an axis-by-axis validator happened to reach first.
	for _, option := range Options {
		axis, ok := optionAxes[option]
		assert.Assert(ok, "a gappable option names the axis a rule matches it on", option)
		if v.ValueEnabled(axis, options[option]) {
			continue
		}
		// A driver defect is worded as one.
		// The encoder does implement the value,
		// so the phrase for a capability it lacks would name the wrong culprit,
		// and the other engine drives the same driver and is no way out of it.
		if refusedByDriver(v, axis, options[option]) {
			return fmt.Errorf("codec %s cannot be published with %s=%s on this machine's %s driver, which miscodes it",
				c.Name, option, options[option], device.Driver)
		}
		refusal, ok := optionRefusals[option]
		assert.Assert(ok, "a gappable option states how its refusal reads", option)
		return fmt.Errorf("codec %s %s on the %s engine%s",
			c.Name, fmt.Sprintf(refusal, options[option]), engine,
			reachedElsewhere(c.Name, engine, option, options[option]))
	}
	// The quantizer target reaches the encoder in crf mode alone,
	// and each encoder's knob has its own scale: 60 is a valid libvpx CQ and an error on x264.
	// Which modes read the knob is the rule's own gate, so this asks about the value and nothing else.
	// A negative target is refused separately,
	// no rule stating a floor and a scale starting at zero being one no band below it can express.
	if cq < 0 || !v.NumberAllowed(rules.AxisCq, cq) {
		return fmt.Errorf("quantizer target %d is outside codec %s's 0-%d range on the %s engine",
			cq, c.Name, c.CqMaxOn(engine), engine)
	}
	if !v.NumberAllowed(rules.AxisBitrateM, bitrateM) {
		return fmt.Errorf("bitrate target %d Mbit/s is above codec %s's %d Mbit/s ceiling on the %s engine",
			bitrateM, c.Name, c.BitrateLimitOn(engine), engine)
	}
	// Every mode sends the keyframe interval, so this asks about the value alone.
	if !v.NumberAllowed(rules.AxisGop, gop) {
		return fmt.Errorf("keyframe interval %d frames is above codec %s's %d frame ceiling on the %s engine",
			gop, c.Name, c.GopLimitOn(engine), engine)
	}
	return nil
}

// refusedByDriver reports whether the installed driver took this value away rather than the encoder.
//
// It reads the reasons the evaluation already attached,
// so what decides the verdict and what words it are one pass:
// a second walk of the DriverDefects rows could refuse under one reading and explain under another.
func refusedByDriver(v rules.Verdicts, axis, value string) bool {
	for _, reason := range v.ValueReasons(axis, value) {
		if reason.GetCode() == screensharev1.TextCode_TEXT_CODE_DRIVER_DEFECT_WITHHOLDS_OPTION {
			return true
		}
	}
	return false
}

// reachedElsewhere names the engines that do reach one value of one option,
// as a clause to hang off the refusal naming the engine that does not.
// Empty where no engine reaches it, which is a fact about the format rather than about one wrapper.
//
// The clause exists because the engine is a setting.
// A refusal naming only the engine the publish was attempted on states half the fact,
// and the half it withholds is the one that can be acted on:
// a chroma one builder cannot reach is often one the other does,
// and a settings file that skipped the repair is the case where nothing greyed the option to say so.
func reachedElsewhere(codec, engine, option, value string) string {
	var others []string
	for _, e := range Engines {
		if e != engine && Reaches(codec, e, option, value) {
			others = append(others, e)
		}
	}
	if len(others) == 0 {
		return ""
	}
	return ", only on " + strings.Join(others, " and ")
}

// validationFacts is the configuration a validation is about, as the axes read it.
//
// It answers the axes this call was given and leaves the rest empty, which is honest rather than
// lossy: a fact nobody stated matches no rule that names a value, so an empty axis withholds nothing.
// What it must never do is guess,
// which would refuse a publish over a combination the caller never described.
//
// The axes it fills are the ones the codec table's own rules match on,
// rather than every axis the vocabulary declares:
// the cursor mode is a capture fact and no rule registered here names it.
// Rule.binds keeps that honest,
// a rule naming an axis the facts do not carry asserting rather than binding nothing.
func validationFacts(c Codec, engine string, options map[string]string, cq, bitrateM, gop int, device Device) rules.Facts {
	return rules.Facts{
		rules.AxisCodec:            rules.TextValue(c.Name),
		rules.AxisFamily:           rules.TextValue(c.Family),
		rules.AxisFormat:           rules.TextValue(c.Format),
		rules.AxisEncoder:          rules.TextValue(c.Encoder()),
		rules.AxisEngine:           rules.TextValue(engine),
		rules.AxisChroma:           rules.TextValue(options[OptionChroma]),
		rules.AxisMode:             rules.TextValue(options[OptionMode]),
		rules.AxisColorRange:       rules.TextValue(options[OptionColorRange]),
		rules.AxisTune:             rules.TextValue(options[OptionTune]),
		rules.AxisCq:               rules.NumberValue(cq),
		rules.AxisBitrateM:         rules.NumberValue(bitrateM),
		rules.AxisGop:              rules.NumberValue(gop),
		rules.AxisGpuDriver:        rules.TextValue(device.Driver),
		rules.AxisGpuModel:         rules.TextValue(device.Model),
		rules.AxisGpuDriverVersion: rules.NumberValue(device.Version),
		rules.AxisCapture:          rules.TextValue(""),
		rules.AxisMemory:           rules.TextValue(""),
		rules.AxisTransport:        rules.TextValue(""),
		rules.AxisAudioCodec:       rules.TextValue(""),
		rules.AxisOS:               rules.TextValue(""),
		rules.AxisDisplay:          rules.TextValue(""),
	}
}

// TargetsBitrate is targetsBitrate for the publish engines,
// which ask the same question of a running pipeline:
// a mode that sends the encoder no rate has none to send it again.
func TargetsBitrate(mode string) bool {
	return targetsBitrate(mode)
}

// targetsBitrate is whether a rate-control mode aims at a bitrate somebody sets.
// Constant quality and lossless spend whatever the picture costs,
// so the bitrate field means nothing to them.
func targetsBitrate(mode string) bool {
	return mode == ModeCbr || mode == ModeVbr || mode == ModeAbr
}

// WidestCqScale is the top of the widest quantizer scale any row declares, on any engine.
//
// What a quantizer control is offered within,
// before the rules narrow it to the selected codec's own scale.
// Derived rather than written down:
// a constant would either clamp a codec counting past it or offer numbers no encoder here reaches,
// and a row declaring a wider scale would leave the constant the one place that did not know.
func WidestCqScale() int {
	widest := 0
	for _, c := range Codecs {
		for _, engine := range Engines {
			if scale := c.CqMaxOn(engine); scale > widest {
				widest = scale
			}
		}
	}
	assert.Assert(widest > 0, "the codec table declares a quantizer scale somewhere")
	return widest
}

// Get is the row under name, and false for a codec no row carries.
func Get(name string) (Codec, bool) {
	for _, c := range Codecs {
		if c.Name == name {
			return c, true
		}
	}
	return Codec{}, false
}

// Encoder is which encode backend this row runs on, at the grain somebody picks one at:
// a family wherever that family is one encoder, and the library where several share a family.
//
// The software family is the one that splits.
// Three encoders code AV1 there and two code the H.26x formats,
// so a picker offered the family would name four rows with one entry,
// and a settings file naming it could not say which.
//
// The second axis of the encode selection, format being the first (Row).
func (c Codec) Encoder() string {
	if c.Library != "" {
		return c.Library
	}
	return c.Family
}

// Row is the encode a format and an encoder name between them, and false for a pair no row carries.
//
// The pair is what the settings hold and this is where it becomes a codec,
// so the two fields a user picks stay independent,
// and neither is a copy of the row they address.
// The grid is sparse: AMD's runtime codes no VP9,
// so that pair names nothing and the form greys it (docs/domain-model.md).
func Row(format, encoder string) (Codec, bool) {
	for _, c := range Codecs {
		if c.Format == format && c.Encoder() == encoder {
			return c, true
		}
	}
	return Codec{}, false
}

// FormatsOn lists the bitstreams one encoder produces, in table order.
// Empty for an encoder no row runs on,
// which is what a statement about a pair the table does not carry names as the way out of it.
func FormatsOn(encoder string) []string {
	var out []string
	for _, c := range Codecs {
		if c.Encoder() == encoder && !contains(out, c.Format) {
			out = append(out, c.Format)
		}
	}
	return out
}

// Encoders lists every encoder the table carries, in table order and once each.
// Order is the codec table's,
// so the picker offers them the way the rows are authored and a row brings its encoder with it.
func Encoders() []string {
	var out []string
	for _, c := range Codecs {
		if !contains(out, c.Encoder()) {
			out = append(out, c.Encoder())
		}
	}
	return out
}

// SupportsChroma answers per engine,
// so a format the codec encodes on the other engine alone reports false here,
// matching what Validate accepts on the engine asked about.
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

// HasFormat is asked by the transport package before it narrows a watch choice by format,
// so a relay path in a format this app never encodes narrows nothing rather than to nothing.
func HasFormat(format string) bool {
	return contains(Formats(), format)
}

// Formats lists the bitstream formats implemented codecs produce, in table order and once each.
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
