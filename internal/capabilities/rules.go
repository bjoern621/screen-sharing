package capabilities

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/rules"
	"bjoernblessin.de/screenshare/internal/text"
)

// The codec table as rules: every gap and every numeric ceiling this package declares,
// registered into the one evaluator (docs/development-principles.md, "A fact lives in
// one table").
//
// The rows keep being authored on the codec they belong to, because a gap reads best
// beside the encoder it is about, and this file is where that shape is turned into the
// shape every consumer reads. What it buys is that a codec fact and a fact about a
// capture backend, a platform or a pair of ends are answered by one evaluation instead of
// by a consumer written per table, which is what the gap mechanism could never do: a Gap
// names a codec, an engine, an option and a value, and a fact needing a fifth axis had
// nowhere to go.
//
// The conversion is deliberately faithful rather than tidy. Each rule below states
// exactly what the row it came from stated, so the equivalence test can hold the two
// answers together for every codec, engine, option and value at once, and a later edit
// that changes what is legal has to change it in the table rather than here.

// optionAxes is the axis a rule names each gappable option by.
//
// The two spellings differ because they answer to different things: an option is keyed
// as the settings JSON names it, and an axis as the form addresses the control. A gap
// declared against an option nobody can match on would bind nothing, so a missing entry
// fails at load rather than at the first resolve.
var optionAxes = map[string]string{
	OptionChroma:     rules.AxisChroma,
	OptionMode:       rules.AxisMode,
	OptionColorRange: rules.AxisColorRange,
}

func init() {
	for _, option := range Options {
		_, ok := optionAxes[option]
		assert.Assert(ok, "a gappable option names the axis a rule matches it on", option)
	}
	rules.Register(codecRules()...)
	rules.Register(audioRules()...)
}

// audioRules is the audio table's gaps, in table order.
//
// An audio codec has one axis and no per-option values, so every gap it declares takes
// the codec off an engine, which is written the same way the video table's engine-wide
// gaps are: a refusal of that entry of the control, binding on the engine alone so one
// evaluation answers for every entry of the dropdown rather than only for the selected
// one.
//
// The table declares none today, because both codecs reach both engines. That is the
// point of converting it anyway: the first audio codec that reaches one engine and not
// the other is a row and no new consumer, where before it would have been a second gap
// lookup written beside the first.
func audioRules() []rules.Rule {
	var out []rules.Rule
	for _, a := range AudioCodecs {
		for _, g := range a.Gaps {
			assert.Assert(g.Option == "",
				"an audio gap takes the codec off an engine rather than withholding an option", a.Name, g.Option)
			out = append(out, rules.Rule{
				When:    audioWhen(g.Engine),
				Verdict: rules.Refuse,
				Field:   rules.AxisAudioCodec,
				Values:  rules.OneOf(a.Name),
				Reason:  g.Reason,
			})
		}
	}
	return out
}

// audioWhen is the facts an audio gap binds under. An empty engine names none, which is
// a codec no engine here codes rather than one wrapper missing an element.
func audioWhen(engine string) map[string]rules.Match {
	if engine == "" {
		return nil
	}
	assert.Assert(knownEngine(engine), "an audio rule names a publish engine", engine)
	return map[string]rules.Match{rules.AxisEngine: rules.OneOf(engine)}
}

// Reaches reports whether one value of one option reaches this codec's encoder on the
// named engine.
//
// It is the rule-backed replacement for asking a row for its gaps. A caller outside this
// package wants the answer rather than the row that produced it, and routing it through
// the evaluator is what keeps a builder's idea of what an element implements and the
// form's greying from drifting apart.
func Reaches(codec, engine, option, value string) bool {
	assert.Assert(knownEngine(engine), "a capability question names a publish engine", engine)
	axis, ok := optionAxes[option]
	assert.Assert(ok, "a capability question names a gappable option", option)

	c, known := Get(codec)
	if !known {
		return false
	}
	return codecVerdicts(c, engine).ValueEnabled(axis, value)
}

// HasEncoderOn reports whether the named engine has an encoder for this codec at all.
// It is the engine-wide gap asked as a question, for a caller deciding what is worth
// probing or building rather than what to grey.
func HasEncoderOn(codec, engine string) bool {
	assert.Assert(knownEngine(engine), "a capability question names a publish engine", engine)

	c, known := Get(codec)
	if !known {
		return false
	}
	return codecVerdicts(c, engine).ValueEnabled(rules.AxisCodec, c.Name)
}

// codecVerdicts answers the codec table's rules for one codec on one engine.
//
// The axes a caller did not name arrive empty, which withholds nothing: the rules that
// read them are the ceilings, and a ceiling asked about no mode and no figure refuses
// nothing. What this answers is the question the old row lookups answered, and it answers
// it out of the same rules every other consumer reads.
func codecVerdicts(c Codec, engine string) rules.Verdicts {
	return rules.EvaluateRules(validationFacts(c, engine, nil, 0, 0), codecRules())
}

// codecRules is the whole table as rules, in table order.
func codecRules() []rules.Rule {
	var out []rules.Rule
	for _, c := range Codecs {
		out = append(out, c.gapRules()...)
		out = append(out, c.ceilingRules()...)
	}
	return out
}

// gapRules is one codec's gaps.
//
// A gap naming an option takes that value of that control away wherever this codec is
// selected. A gap naming none takes the codec itself off the engine, which is written as
// a refusal of the codec entry rather than of any control below it: no value of any
// option reaches an encoder that is not there, and greying the entry is what says so
// once instead of once per control.
func (c Codec) gapRules() []rules.Rule {
	out := make([]rules.Rule, 0, len(c.Gaps))
	for _, g := range c.Gaps {
		if g.Option == "" {
			out = append(out, rules.Rule{
				When:    c.when(g.Engine, nil),
				Verdict: rules.Refuse,
				Field:   rules.AxisCodec,
				Values:  rules.OneOf(c.Name),
				Reason:  g.Reason,
			})
			continue
		}
		axis, ok := optionAxes[g.Option]
		assert.Assert(ok, "a gap names a gappable option", g.Option, c.Name)
		out = append(out, rules.Rule{
			When:    c.when(g.Engine, map[string]rules.Match{rules.AxisCodec: rules.OneOf(c.Name)}),
			Verdict: rules.Refuse,
			Field:   axis,
			Values:  rules.OneOf(g.Value),
			Reason:  g.Reason,
		})
	}
	return out
}

// ceilingRules is one codec's numeric limits, per engine.
//
// They are rules for the reason the gaps are, and they are the half that makes the system
// one shape rather than two. A quantizer scale and a bitrate ceiling used to be columns
// that two consumers read separately: the form narrowed a control by them and the
// validator refused a value by them, so the range a slider offered and the value a publish
// accepted were two answers derived from one fact. As rules they are one answer, and the
// statement that narrows the control is the statement that refuses the value.
//
// Each is gated on the modes that read the knob. A quantizer target the encoder never sees
// must not narrow a slider the user is not looking at, and a bitrate ceiling means nothing
// to a constant-quality encode that sends no target. The mode axis is what the columns
// could not carry at all: targetsBitrate lived in the validator, so the form narrowed the
// bitrate in every mode including the two that ignore it.
//
// The limit itself rides as an argument. It is the one figure no axis carries, since it is
// the row's own fact rather than anything the configuration reads.
func (c Codec) ceilingRules() []rules.Rule {
	var out []rules.Rule
	for _, engine := range Engines {
		if scale := c.CqMaxOn(engine); scale > 0 {
			out = append(out, rules.Rule{
				When: c.when(engine, map[string]rules.Match{
					rules.AxisCodec: rules.OneOf(c.Name),
					rules.AxisMode:  rules.OneOf(ModeCrf),
				}),
				Verdict: rules.Refuse,
				Field:   rules.AxisCq,
				Values:  rules.AtLeast(scale + 1),
				Reason:  screensharev1.TextCode_TEXT_CODE_CQ_ABOVE_CODEC_SCALE,
				Args: []*screensharev1.TextArg{
					text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_CQ_MAX, int64(scale)),
				},
			})
		}
		if limit := c.BitrateLimitOn(engine); limit > 0 {
			out = append(out, rules.Rule{
				When: c.when(engine, map[string]rules.Match{
					rules.AxisCodec: rules.OneOf(c.Name),
					rules.AxisMode:  rules.OneOf(bitrateModes()...),
				}),
				Verdict: rules.Refuse,
				Field:   rules.AxisBitrateM,
				Values:  rules.AtLeast(limit + 1),
				Reason:  screensharev1.TextCode_TEXT_CODE_BITRATE_ABOVE_CODEC_LIMIT,
				Args: []*screensharev1.TextArg{
					text.Num(screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_LIMIT_MBPS, int64(limit)),
				},
			})
		}
	}
	return out
}

// bitrateModes is the rate-control modes that aim at a bitrate the user sets, in the shape
// a Match takes. It reads targetsBitrate rather than listing the three again, so the modes
// a rule narrows the control in and the modes the validator checks the ceiling in cannot
// come apart.
func bitrateModes() []string {
	out := make([]string, 0, len(Modes))
	for _, mode := range Modes {
		if targetsBitrate(mode) {
			out = append(out, mode)
		}
	}
	return out
}

// when is the facts a rule off this codec binds under: whatever it was given, plus the
// engine where the fact is one engine's.
//
// An empty engine names none, which is how a fact about the format or the library rather
// than about one wrapper is written: leaving the axis out is what makes the rule bind on
// both engines, and naming both would be the same answer written twice.
func (c Codec) when(engine string, base map[string]rules.Match) map[string]rules.Match {
	out := make(map[string]rules.Match, len(base)+1)
	for axis, match := range base {
		out[axis] = match
	}
	if engine != "" {
		assert.Assert(knownEngine(engine), "a rule off the codec table names a publish engine", engine, c.Name)
		out[rules.AxisEngine] = rules.OneOf(engine)
	}
	return out
}
