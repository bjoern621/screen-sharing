package publish

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/framestamp"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/rules"
)

// cursorFacts is a configuration the pointer rules bind against.
//
// The format is one a stamp is written into, so what the capture backend rows say is what these
// answer: a format carrying no stamp refuses the mode whatever the capture does.
func cursorFacts(capture string) rules.Facts {
	return rules.Facts{
		rules.AxisCapture: rules.TextValue(capture),
		rules.AxisFormat:  rules.TextValue("h264"),
	}
}

// The rules and the table are one fact read two ways, over every backend and every mode.
func TestCursorRulesAnswerWhatTheTableServes(t *testing.T) {
	for _, capture := range Captures() {
		v := rules.EvaluateRules(cursorFacts(capture), cursorRules())
		for _, mode := range cursor.Modes {
			served := CursorServed(capture, mode)
			refused := !v.ValueEnabled(rules.AxisCursor, mode)

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

// A backend that reads a position states nothing against the mode.
// The portal's cursor metadata and the X11 server's answer are two ways to the same reading, so
// neither carries a refusal a reader would go looking for a setting over.
func TestTheBackendsThatReadAPositionStateNothing(t *testing.T) {
	for _, capture := range []string{"portal", "ximagesrc"} {
		v := rules.EvaluateRules(cursorFacts(capture), cursorRules())

		if !v.ValueEnabled(rules.AxisCursor, cursor.Metadata) {
			t.Errorf("%s refuses the mode its capture reads a position for", capture)
		}
		if got := len(v.ValueReasons(rules.AxisCursor, cursor.Metadata)); got != 0 {
			t.Errorf("%s states %d facts against the mode, want none", capture, got)
		}
	}

	// A backend with no position to report states its own fact, which is a different thing to fix than
	// a format that cannot carry one.
	kms := rules.EvaluateRules(cursorFacts("kmsgrab"), cursorRules())
	if got := len(kms.ValueReasons(rules.AxisCursor, cursor.Metadata)); got != 1 {
		t.Errorf("a backend with no pointer metadata states %d facts, want its own", got)
	}
}

// The portal's rows state what the ScreenCast call can express, and the compositor behind it states
// what it answers, so the machine is asked before a mode is offered.
func TestThePortalsPointerModesFollowTheCompositor(t *testing.T) {
	served := portal.Capabilities{CursorModes: portal.CursorHidden | portal.CursorEmbedded}

	if CursorServedHere(served, "portal", cursor.Metadata) {
		t.Error("a mode this compositor refuses is offered on the portal backend")
	}
	for _, mode := range []string{cursor.Embedded, cursor.Hidden} {
		if !CursorServedHere(served, "portal", mode) {
			t.Errorf("%s is withheld on a compositor serving it", mode)
		}
	}

	// The other backends read their own property and answer for themselves,
	// so a portal mask says nothing about them.
	if !CursorServedHere(served, "ximagesrc", cursor.Metadata) {
		t.Error("the X11 backend is greyed by what the desktop portal serves")
	}
}

// A machine nothing asked withholds nothing, so the option stays on offer
// and the portal's own refusal is what the publish meets.
func TestAnUnreadPortalMaskWithholdsNoPointerMode(t *testing.T) {
	for _, mode := range cursor.Modes {
		if !CursorServedHere(portal.Capabilities{}, "portal", mode) {
			t.Errorf("%s is greyed on a machine nothing asked", mode)
		}
	}
}

// The position reaches a viewer inside the encoded frames, so a format with no unit to carry one
// refuses the mode however well the capture reads it.
// Stated against the format rather than the capture:
// what a reader changes to get a pointer is the format.
func TestAFormatCarryingNoStampRefusesTheMode(t *testing.T) {
	for _, format := range capabilities.Formats() {
		facts := rules.Facts{
			rules.AxisCapture: rules.TextValue("ximagesrc"),
			rules.AxisFormat:  rules.TextValue(format),
		}
		v := rules.EvaluateRules(facts, cursorRules())

		carried := framestamp.Carries(format)
		if got := v.ValueEnabled(rules.AxisCursor, cursor.Metadata); got != carried {
			t.Errorf("%s carries a stamp=%v and offers the metadata mode=%v", format, carried, got)
		}
		if carried {
			continue
		}
		reasons := v.ValueReasons(rules.AxisCursor, cursor.Metadata)
		if len(reasons) != 1 {
			t.Fatalf("%s: the refusal states one fact, got %d", format, len(reasons))
		}
		if got := reasons[0].GetCode(); got != screensharev1.TextCode_TEXT_CODE_FORMAT_CARRIES_NO_CURSOR_METADATA {
			t.Errorf("%s states %v, want the fact that its bitstream carries no position", format, got)
		}
	}
}
