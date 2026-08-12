package form

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/rules"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The facts one resolve is evaluated against: every axis the rule vocabulary declares,
// read off the draft and off what this machine answered.
//
// It lives here because this is the layer that holds both, and the rules package holds
// neither on purpose - it is what every domain package registers into, so it may import
// none of them. The division is the same one the whole contract makes: the domain states
// what is true about a combination, and the layer that knows the combination assembles it.
//
// Every declared axis is filled on every pass, including the ones no rule reads yet. A
// rule that named an axis nobody answered would bind nothing and read on screen as a
// combination the app allows, which is indistinguishable from one nobody constrained, so
// the evaluator asserts rather than defaulting and this is the function that has to
// satisfy it.

// factsOf reads one draft into the vocabulary.
//
// A fact this machine cannot establish arrives empty rather than guessed. An empty
// reading matches no rule that names a value, so an unstated fact withholds nothing,
// which is the same answer availability gives for an engine it could not derive.
func factsOf(d Deps, s settings.Settings) rules.Facts {
	codec, known := capabilities.Get(s.Publish.Codec)
	family, format := "", ""
	if known {
		family, format = codec.Family, codec.Format
	}

	f := rules.Facts{
		rules.AxisCapture:    rules.TextValue(s.Publish.Capture),
		rules.AxisEngine:     rules.TextValue(factsEngineOf(s)),
		rules.AxisCodec:      rules.TextValue(s.Publish.Codec),
		rules.AxisFamily:     rules.TextValue(family),
		rules.AxisFormat:     rules.TextValue(format),
		rules.AxisChroma:     rules.TextValue(s.Publish.Chroma),
		rules.AxisColorRange: rules.TextValue(s.Publish.ColorRange),
		rules.AxisMode:       rules.TextValue(s.Publish.Mode),
		rules.AxisMemory:     rules.TextValue(s.Publish.CaptureMemory),
		rules.AxisCursor:     rules.TextValue(s.Publish.Cursor),
		rules.AxisTransport:  rules.TextValue(s.Publish.Transport),
		rules.AxisAudioCodec: rules.TextValue(s.Publish.AudioCodec),
		rules.AxisOS:         rules.TextValue(d.Platform.OS),
		rules.AxisDisplay:    rules.TextValue(d.Platform.Display),
		rules.AxisBitrateM:   rules.NumberValue(s.Publish.BitrateM),
		rules.AxisCq:         rules.NumberValue(s.Publish.Cq),
	}

	// The vocabulary is the contract between this function and every rule there is, so a
	// declared axis nobody filled fails here rather than at whichever resolve first
	// evaluates a rule that names it.
	for _, axis := range rules.Axes() {
		_, ok := f[axis.Name]
		assert.Assert(ok, "the facts answer every declared axis", axis.Name)
	}
	return f
}

// factsEngineOf is the publish engine the selected capture backend runs, and the empty
// string for a backend this app has no publisher for.
//
// It differs from optionEngineOf, which answers ffmpeg for an unknown backend so an
// option list has something to be built against. A fact may not be invented that way: an
// engine nobody established would bind every rule that names ffmpeg and grey controls on
// a machine whose backend runs neither engine.
func factsEngineOf(s settings.Settings) string {
	engine, err := publish.EngineFor(s.Publish.Capture)
	if err != nil {
		return ""
	}
	return engine
}

// verdictsOf is what the rules say about one draft. It is the one entry point the form
// reads availability through, so a control's greying, its notes and the ends it is
// offered between are three readings of one evaluation.
func verdictsOf(d Deps, s settings.Settings) rules.Verdicts {
	return rules.Evaluate(factsOf(d, s))
}
