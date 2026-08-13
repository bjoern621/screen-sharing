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
// Assembled in this layer because it is the one holding both.
// The rules package holds neither on purpose: it is what every domain package registers into,
// so it may import none of them.
//
// Every declared axis is filled on every pass, including the ones no rule reads.
// A rule naming an axis nobody answered would bind nothing,
// and read on screen as a combination the app allows,
// indistinguishable from one nobody constrained,
// so the evaluator asserts rather than defaulting.

// factsOf reads one draft into the rule vocabulary.
//
// A fact this machine cannot establish arrives empty rather than guessed.
// An empty reading matches no rule that names a value, so an unstated fact greys nothing,
// which is the answer availability gives for an engine it could not derive.
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

	// A declared axis nobody filled fails here,
	// rather than at whichever resolve first evaluates a rule that names it.
	for _, axis := range rules.Axes() {
		_, ok := f[axis.Name]
		assert.Assert(ok, "the facts answer every declared axis", axis.Name)
	}
	return f
}

// factsEngineOf is the publish engine behind the selected capture backend,
// empty for a backend this app has no publisher for.
//
// A fact may not be invented the way optionEngineOf invents one,
// answering ffmpeg for an unknown backend so an option list has something to be built against.
// An engine nobody established would bind every rule that names ffmpeg,
// and grey controls on a machine whose backend runs neither engine.
func factsEngineOf(s settings.Settings) string {
	engine, err := publish.EngineFor(s.Publish.Capture)
	if err != nil {
		return ""
	}
	return engine
}

// verdictsOf is what the rules say about one draft, evaluated per read and never held.
// It is the one entry point the form reads availability through,
// so a greying, a note and a numeric control's ends are three readings of one evaluation.
func verdictsOf(d Deps, s settings.Settings) rules.Verdicts {
	return rules.Evaluate(factsOf(d, s))
}
