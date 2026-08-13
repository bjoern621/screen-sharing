package publish

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/rules"
)

// The one engine that cannot read what it captured says so where a reader is reaching for the
// format an HDR surface rides in.
//
// It is a note and never a refusal, because a refusal needs a fact and the fact is exactly what
// that engine cannot establish: a running ffmpeg tells its caller what it is encoding and never
// what it read.
// What the note buys is that the reader is told before the stream goes out,
// rather than by a viewer whose picture is wrong.
func TestTheEngineThatCannotReadTheSurfaceSaysSo(t *testing.T) {
	for _, tc := range []struct {
		engine string
		noted  bool
	}{
		{engine: EngineFfmpeg, noted: true},
		{engine: EngineGst, noted: false},
	} {
		facts := rules.Facts{
			rules.AxisEngine: rules.TextValue(tc.engine),
			rules.AxisChroma: rules.TextValue(tenBitChroma),
		}
		notes := rules.EvaluateRules(facts, engineColourRules()).ValueNotes(rules.AxisChroma, tenBitChroma)

		if noted := len(notes) > 0; noted != tc.noted {
			t.Errorf("the %s engine notes %d things about %s, want noted=%v",
				tc.engine, len(notes), tenBitChroma, tc.noted)
		}
		if !tc.noted {
			continue
		}
		// The note names the engine that does carry the surface's colour, so it states the way out and
		// not only the limit.
		if !namesArg(notes[0], screensharev1.TextArgName_TEXT_ARG_NAME_OTHER_ENGINE, EngineGst) {
			t.Errorf("the note is %v, and it does not name the engine that carries it", notes[0])
		}
	}
}

// The note is on the format an HDR surface rides in and on no other, so a standard-range publish is
// not followed around by a statement about a case it is not in.
func TestTheNoteIsOnTheTenBitFormatAlone(t *testing.T) {
	facts := rules.Facts{
		rules.AxisEngine: rules.TextValue(EngineFfmpeg),
		rules.AxisChroma: rules.TextValue("yuv420p"),
	}
	v := rules.EvaluateRules(facts, engineColourRules())
	if notes := v.ValueNotes(rules.AxisChroma, "yuv420p"); len(notes) > 0 {
		t.Errorf("an 8-bit format carries %v, and no 8-bit format can hold an HDR surface", notes)
	}
}

// namesArg reports whether a statement carries one identifier under one argument name.
func namesArg(t *screensharev1.Text, name screensharev1.TextArgName, id string) bool {
	for _, a := range t.GetArgs() {
		if a.GetName() == name && a.GetId() == id {
			return true
		}
	}
	return false
}
