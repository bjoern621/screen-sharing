package publish

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/rules"
	"bjoernblessin.de/screenshare/internal/text"
)

// What each engine can say about the colour of the surface it captures.
//
// One of them can say nothing.
// The GStreamer engine's child reports the caps the capture negotiated, which is how an HDR surface
// is recognised at all and how the encoder input is narrowed to the colour it has
// (gstrun/surface.go).
// The ffmpeg engine has no such report, a running ffmpeg telling its caller what it is encoding and
// never what it read, and it tags every encode BT.709 through a setparams filter, a partial colour
// description being no description (ffmpeg/args.go, colourFilter).
//
// An HDR desktop captured through an ffmpeg backend therefore publishes as a standard-range stream
// carrying HDR samples: every viewer trusts the tag, and the picture is wrong on all of them.
// Nothing there can detect it, so it is stated rather than refused: a refusal needs a fact, and
// that fact is unavailable on that engine.
//
// The statement lands on the one control a reader reaches HDR through.
// Ten bits per component is the only format an HDR surface rides in, so the note appears where
// somebody is reaching for it and nowhere else.
// A note on the whole engine would follow every standard-range publish around for a case it is not
// in.

func init() {
	rules.Register(engineColourRules()...)
}

// engineColourRules is what each engine states about the colour it carries.
func engineColourRules() []rules.Rule {
	return []rules.Rule{{
		When: map[string]rules.Match{
			rules.AxisEngine: rules.OneOf(EngineFfmpeg),
		},
		Verdict: rules.Note,
		Field:   rules.AxisChroma,
		Values:  rules.OneOf(tenBitChroma),
		Reason:  screensharev1.TextCode_TEXT_CODE_ENGINE_TAGS_STANDARD_RANGE,
		// The engine that does carry it, so the note names the way out rather than only the limit.
		// An argument rather than an axis reading, the axis carrying the selected engine where a reader
		// needs the other one.
		Args: []*screensharev1.TextArg{
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OTHER_ENGINE, EngineGst),
		},
	}}
}
