package publish

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/rules"
)

// cursorFacts is a configuration the pointer rules bind against. They name the capture
// backend and nothing else, so that is what a caller has to answer.
func cursorFacts(capture string) rules.Facts {
	return rules.Facts{rules.AxisCapture: rules.TextValue(capture)}
}

// The rules and the table are one fact read two ways, for every backend and every mode.
func TestCursorRulesAnswerWhatTheTableServes(t *testing.T) {
	for _, capture := range Captures() {
		v := rules.EvaluateRules(cursorFacts(capture), cursorRules())
		for _, mode := range cursor.Modes {
			served := CursorServed(capture, mode)
			refused := !v.ValueEnabled(rules.AxisCursor, mode)

			// Metadata is the one mode a backend can serve and the app still refuse,
			// because nothing carries the pointer to a viewer yet. Every other mode is
			// the table's answer alone.
			if mode == cursor.Metadata {
				if !refused {
					t.Errorf("%s: the metadata mode is offered while nothing carries a pointer", capture)
				}
				continue
			}
			if served == refused {
				t.Errorf("%s: the table serves %s=%v and the rules refuse=%v", capture, mode, served, refused)
			}
		}
	}
}

// Every backend leaves a way to publish. A combination where the pointer control has no
// legal value would be a form the repair cannot move off, so the table is held to it
// rather than trusted.
func TestEveryBackendServesAPointerMode(t *testing.T) {
	for _, capture := range Captures() {
		v := rules.EvaluateRules(cursorFacts(capture), cursorRules())
		reachable := 0
		for _, mode := range cursor.Modes {
			if v.ValueEnabled(rules.AxisCursor, mode) {
				reachable++
			}
		}
		if reachable == 0 {
			t.Errorf("%s offers no pointer mode at all", capture)
		}
	}
}

// The scanout backend is the one that cannot draw the pointer, and it says so with the
// fact about its own path rather than with a sentence about backends in general.
func TestTheScanoutBackendCannotDrawThePointer(t *testing.T) {
	v := rules.EvaluateRules(cursorFacts("kmsgrab"), cursorRules())

	if v.ValueEnabled(rules.AxisCursor, cursor.Embedded) {
		t.Fatal("kmsgrab offers to draw a pointer it cannot reach")
	}
	reasons := v.ValueReasons(rules.AxisCursor, cursor.Embedded)
	if len(reasons) != 1 {
		t.Fatalf("the refusal states one fact, got %d", len(reasons))
	}
	if got := reasons[0].GetCode(); got != screensharev1.TextCode_TEXT_CODE_KMSGRAB_HAS_NO_CURSOR_PLANE {
		t.Errorf("the refusal states %v, want the scanout plane fact", got)
	}
	if !v.ValueEnabled(rules.AxisCursor, cursor.Hidden) {
		t.Error("kmsgrab refuses the mode that describes what it actually does")
	}
}

// On the portal both facts bind and both cross: the capture does report a pointer
// position, and this app carries none. A reader who saw only the first would go looking
// for a setting that would not help.
func TestThePortalCarriesOnlyTheAppsOwnLimit(t *testing.T) {
	v := rules.EvaluateRules(cursorFacts("portal"), cursorRules())

	reasons := v.ValueReasons(rules.AxisCursor, cursor.Metadata)
	if len(reasons) != 1 {
		t.Fatalf("the portal's metadata refusal states one fact, got %d", len(reasons))
	}
	if got := reasons[0].GetCode(); got != screensharev1.TextCode_TEXT_CODE_CURSOR_METADATA_NOT_CARRIED {
		t.Errorf("the portal states %v, want the fact that nothing carries a pointer", got)
	}

	// A backend that has no pointer position to send states both, since the two are
	// different things to fix.
	x11 := rules.EvaluateRules(cursorFacts("ximagesrc"), cursorRules())
	if got := len(x11.ValueReasons(rules.AxisCursor, cursor.Metadata)); got != 2 {
		t.Errorf("a backend with no pointer metadata states %d facts, want both", got)
	}
}
