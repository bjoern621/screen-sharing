package publish

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/rules"
)

// cursorFacts is a configuration the pointer rules bind against.
// They match on the capture backend and nothing else, so that is all a caller has to answer.
func cursorFacts(capture string) rules.Facts {
	return rules.Facts{rules.AxisCapture: rules.TextValue(capture)}
}

// The rules and the table are one fact read two ways, over every backend and every mode.
func TestCursorRulesAnswerWhatTheTableServes(t *testing.T) {
	for _, capture := range Captures() {
		v := rules.EvaluateRules(cursorFacts(capture), cursorRules())
		for _, mode := range cursor.Modes {
			served := CursorServed(capture, mode)
			refused := !v.ValueEnabled(rules.AxisCursor, mode)

			// Metadata is the one mode a backend serves and the app still refuses: the portal reports a
			// position nothing here reads, which is a fact about this app rather than about that capture.
			// Every other mode, on every other backend, is the table's answer alone.
			if mode == cursor.Metadata && capture == "portal" {
				if !refused {
					t.Errorf("%s: the metadata mode is offered while nothing reads the capture's own pointer", capture)
				}
				continue
			}
			if served == refused {
				t.Errorf("%s: the table serves %s=%v and the rules refuse=%v", capture, mode, served, refused)
			}
		}
	}
}

// A pointer control with no legal value is a form the repair cannot move off,
// so every backend is held to leaving one mode reachable rather than trusted with it.
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

// The scanout backend refuses with the fact about its own path rather than with a sentence about
// backends in general.
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

// The portal carries the app's own limit alone: the capture does report a position, so a reader who
// saw a "this backend has no pointer metadata" refusal there would go looking for a setting that
// would not help.
func TestThePortalCarriesOnlyTheAppsOwnLimit(t *testing.T) {
	v := rules.EvaluateRules(cursorFacts("portal"), cursorRules())

	reasons := v.ValueReasons(rules.AxisCursor, cursor.Metadata)
	if len(reasons) != 1 {
		t.Fatalf("the portal's metadata refusal states one fact, got %d", len(reasons))
	}
	if got := reasons[0].GetCode(); got != screensharev1.TextCode_TEXT_CODE_CURSOR_METADATA_NOT_CARRIED {
		t.Errorf("the portal states %v, want the fact that nothing carries a pointer", got)
	}

	// The X11 backend serves the mode and states nothing: its display server answers any client asking
	// where the pointer is, which is what the publish child reads there.
	x11 := rules.EvaluateRules(cursorFacts("ximagesrc"), cursorRules())
	if got := len(x11.ValueReasons(rules.AxisCursor, cursor.Metadata)); got != 0 {
		t.Errorf("a backend whose display server answers states %d facts, want none", got)
	}

	// A backend with no position to report states its own fact, which is a different thing to fix than
	// the app's.
	kms := rules.EvaluateRules(cursorFacts("kmsgrab"), cursorRules())
	if got := len(kms.ValueReasons(rules.AxisCursor, cursor.Metadata)); got != 1 {
		t.Errorf("a backend with no pointer metadata states %d facts, want its own", got)
	}
}
