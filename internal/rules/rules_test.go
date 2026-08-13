package rules

import (
	"strings"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Codes with no meaning beyond being valid.
// What a rule states is checked by which identifiers ride with it, never by the wording,
// which lives on the surface.
const (
	someReason  = screensharev1.TextCode_TEXT_CODE_CODEC_CANNOT_ENCODE_CHROMA
	otherReason = screensharev1.TextCode_TEXT_CODE_CODEC_CODES_NO_RGB
)

// facts is a whole configuration, so a test says what it changes rather than what it carries.
// Every declared axis is present, which is what the evaluator requires of a caller and what a
// partial fixture would hide.
func facts(over map[string]Value) Facts {
	f := Facts{
		AxisCapture:    TextValue("portal"),
		AxisEngine:     TextValue("gstreamer"),
		AxisCodec:      TextValue("libvpx"),
		AxisFamily:     TextValue("software"),
		AxisFormat:     TextValue("vp8"),
		AxisChroma:     TextValue("yuv420p"),
		AxisColorRange: TextValue("pc"),
		AxisMode:       TextValue("cbr"),
		AxisMemory:     TextValue("auto"),
		AxisTransport:  TextValue("srt"),
		AxisAudioCodec: TextValue("opus"),
		AxisOS:         TextValue("linux"),
		AxisDisplay:    TextValue("wayland"),
		AxisBitrateM:   NumberValue(150),
		AxisCq:         NumberValue(19),
	}
	for k, v := range over {
		f[k] = v
	}
	return f
}

// A rule naming one axis binds wherever that axis reads what it asks for, which is the broad end of
// the range the old Gap could not reach past.
func TestOneAxisBindsAcrossEverythingElse(t *testing.T) {
	r := Rule{
		When:    map[string]Match{AxisFormat: OneOf("vp8")},
		Verdict: Refuse,
		Field:   AxisColorRange,
		Values:  OneOf("pc"),
		Reason:  someReason,
	}

	v := EvaluateRules(facts(nil), []Rule{r})
	if v.ValueEnabled(AxisColorRange, "pc") {
		t.Error("a format-wide refusal left the value selectable")
	}
	if !v.ValueEnabled(AxisColorRange, "tv") {
		t.Error("a refusal of one value took another with it")
	}

	// Same rule, another codec of another format: nothing is taken.
	other := EvaluateRules(facts(map[string]Value{AxisFormat: TextValue("av1")}), []Rule{r})
	if !other.ValueEnabled(AxisColorRange, "pc") {
		t.Error("a rule bound on a format it does not name")
	}
}

// The surgical end: a rule may name as many axes as the fact needs, and it binds on that exact
// combination alone.
// This is what "only this codec on this capture backend on this engine" is written as.
func TestEveryNamedAxisHasToBind(t *testing.T) {
	r := Rule{
		When: map[string]Match{
			AxisCodec:   OneOf("libvpx"),
			AxisEngine:  OneOf("gstreamer"),
			AxisCapture: OneOf("portal"),
			AxisMode:    OneOf("cbr"),
			AxisChroma:  OneOf("yuv420p"),
		},
		Verdict: Refuse,
		Field:   "publish.effort",
		Reason:  someReason,
	}

	if v := EvaluateRules(facts(nil), []Rule{r}); v.Enabled("publish.effort") {
		t.Error("the exact combination did not bind")
	}
	for _, off := range []struct {
		axis  string
		value Value
	}{
		{AxisCodec, TextValue("libx264")},
		{AxisEngine, TextValue("ffmpeg")},
		{AxisCapture, TextValue("ximagesrc")},
		{AxisMode, TextValue("crf")},
		{AxisChroma, TextValue("gbrp")},
	} {
		v := EvaluateRules(facts(map[string]Value{off.axis: off.value}), []Rule{r})
		if !v.Enabled("publish.effort") {
			t.Errorf("the rule bound with %s reading %s", off.axis, off.value.Text())
		}
	}
}

// A refusal naming no value takes the whole control, which is how a field is greyed.
func TestRefusingNoValueTakesTheControl(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{{
		When:    map[string]Match{AxisEngine: OneOf("gstreamer")},
		Verdict: Refuse,
		Field:   "publish.effort",
		Reason:  someReason,
	}})

	if v.Enabled("publish.effort") {
		t.Error("a control refused entirely stayed editable")
	}
	if len(v.Reasons("publish.effort")) != 1 {
		t.Errorf("a disabled control says why, got %d reasons", len(v.Reasons("publish.effort")))
	}
	if !v.Visible("publish.effort") {
		t.Error("greying a control took it off the screen")
	}
}

// Every reason crosses.
// A control two facts block is blocked by two facts, and which one a reader is shown is the shell's
// judgement rather than a ranking made here.
func TestEveryMatchingReasonIsKept(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{
		{
			When:    map[string]Match{AxisEngine: OneOf("gstreamer")},
			Verdict: Refuse,
			Field:   "publish.effort",
			Reason:  someReason,
		},
		{
			When:    map[string]Match{AxisFamily: OneOf("software")},
			Verdict: Refuse,
			Field:   "publish.effort",
			Reason:  otherReason,
		},
	})

	got := v.Reasons("publish.effort")
	if len(got) != 2 {
		t.Fatalf("two facts blocked the control, %d reasons crossed", len(got))
	}
	if got[0].GetCode() != someReason || got[1].GetCode() != otherReason {
		t.Errorf("the reasons arrived as %v and %v", got[0].GetCode(), got[1].GetCode())
	}
}

// A numeric refusal narrows the ends the control is offered between, so the slider and the publish
// cannot disagree about what the encoder takes.
func TestNumericRefusalNarrowsTheOfferedRange(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{{
		When:    map[string]Match{AxisCodec: OneOf("libvpx")},
		Verdict: Refuse,
		Field:   AxisBitrateM,
		Values:  AtLeast(101),
		Reason:  someReason,
	}})

	low, high := v.Bounds(AxisBitrateM, 0, 10000)
	if low != 0 || high != 100 {
		t.Errorf("the offered range narrowed to %d-%d, want 0-100", low, high)
	}
	if len(v.BoundReasons(AxisBitrateM)) != 1 {
		t.Error("a narrowed range says why")
	}
	// A control nothing narrowed is offered what it was given.
	if l, h := v.Bounds(AxisCq, 0, 63); l != 0 || h != 63 {
		t.Errorf("an unconstrained range came back as %d-%d", l, h)
	}
}

// Two bands narrow from both ends, which is the case a single ceiling column could not express at
// all.
func TestBandsNarrowFromBothEnds(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{
		{
			When: map[string]Match{AxisCodec: OneOf("libvpx")}, Verdict: Refuse,
			Field: AxisCq, Values: AtLeast(52), Reason: someReason,
		},
		{
			When: map[string]Match{AxisCodec: OneOf("libvpx")}, Verdict: Refuse,
			Field: AxisCq, Values: AtMost(9), Reason: otherReason,
		},
	})

	if low, high := v.Bounds(AxisCq, 0, 63); low != 10 || high != 51 {
		t.Errorf("the offered range narrowed to %d-%d, want 10-51", low, high)
	}
}

// Notes go either way: about the control, or about one of its entries.
func TestNotesRideOnControlsAndOnEntries(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{
		{
			When: map[string]Match{AxisMode: OneOf("cbr")}, Verdict: Note,
			Field: AxisBitrateM, Reason: someReason,
		},
		{
			When: map[string]Match{AxisCodec: OneOf("libvpx")}, Verdict: Note,
			Field: AxisChroma, Values: OneOf("yuv420p"), Reason: otherReason,
		},
	})

	if len(v.Notes(AxisBitrateM)) != 1 {
		t.Error("a control-wide note did not arrive")
	}
	if len(v.ValueNotes(AxisChroma, "yuv420p")) != 1 {
		t.Error("an entry's note did not arrive")
	}
	if len(v.ValueNotes(AxisChroma, "gbrp")) != 0 {
		t.Error("a note about one entry landed on another")
	}
	// A note never greys what it describes.
	if !v.Enabled(AxisBitrateM) || !v.ValueEnabled(AxisChroma, "yuv420p") {
		t.Error("a note made a control inert")
	}
}

func TestHidingTakesTheControlOffTheScreen(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{{
		When:    map[string]Match{AxisCapture: OneOf("portal")},
		Verdict: Hide,
		Field:   "publish.drm_map",
		Reason:  someReason,
	}})

	if v.Visible("publish.drm_map") {
		t.Error("a hidden control stayed on the screen")
	}
}

// The reason carries the axes the rule matched on, in vocabulary order, plus the control and the
// single value the verdict took.
// That is what lets a row state a code alone.
func TestReasonCarriesTheIdentifiersItIsAbout(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{{
		When: map[string]Match{
			AxisCodec:    OneOf("libvpx"),
			AxisEngine:   OneOf("gstreamer"),
			AxisBitrateM: AtLeast(100),
		},
		Verdict: Refuse,
		Field:   AxisColorRange,
		Values:  OneOf("pc"),
		Reason:  someReason,
	}})

	reasons := v.ValueReasons(AxisColorRange, "pc")
	if len(reasons) != 1 {
		t.Fatalf("the entry carries one reason, got %d", len(reasons))
	}
	var got []string
	for _, a := range reasons[0].GetArgs() {
		got = append(got, a.GetName().String())
	}
	want := []string{
		"TEXT_ARG_NAME_ENGINE", "TEXT_ARG_NAME_CODEC", "TEXT_ARG_NAME_BITRATE_MBPS",
		"TEXT_ARG_NAME_OPTION", "TEXT_ARG_NAME_VALUE",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the statement carried %v, want %v", got, want)
	}
	for _, a := range reasons[0].GetArgs() {
		if a.GetName() == screensharev1.TextArgName_TEXT_ARG_NAME_CODEC && a.GetId() != "libvpx" {
			t.Errorf("the codec argument carried %q", a.GetId())
		}
		if a.GetName() == screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_MBPS && a.GetNumber() != 150 {
			t.Errorf("the bitrate argument carried %d, want the fact's own reading", a.GetNumber())
		}
	}
}

// A rule refusing several entries names none of them in its statement: there is no one value to
// point at, and which entries went is the control's own answer.
func TestSeveralValuesCarryNoSingleValueArgument(t *testing.T) {
	v := EvaluateRules(facts(nil), []Rule{{
		When:    map[string]Match{AxisEngine: OneOf("gstreamer")},
		Verdict: Refuse,
		Field:   AxisChroma,
		Values:  OneOf("gbrp", "yuv444p"),
		Reason:  someReason,
	}})

	for _, value := range []string{"gbrp", "yuv444p"} {
		if v.ValueEnabled(AxisChroma, value) {
			t.Errorf("%s stayed selectable", value)
		}
		for _, a := range v.ValueReasons(AxisChroma, value)[0].GetArgs() {
			if a.GetName() == screensharev1.TextArgName_TEXT_ARG_NAME_VALUE {
				t.Errorf("a refusal of two entries named %q as the value", a.GetId())
			}
		}
	}
}

// The facts have to carry every axis a rule names.
// A rule that quietly bound nothing would read on screen as a combination the app allows.
func TestAnAbsentFactIsABug(t *testing.T) {
	defer func() {
		switch panicked := recover().(type) {
		case nil:
			t.Error("an evaluation ran with an axis nobody answered")
		case string:
			if !strings.Contains(panicked, "every axis a rule matches on") {
				t.Errorf("the evaluation aborted over %q", panicked)
			}
		default:
			t.Errorf("the evaluation aborted with %v", panicked)
		}
	}()

	partial := facts(nil)
	delete(partial, AxisCodec)
	EvaluateRules(partial, []Rule{{
		When: map[string]Match{AxisCodec: OneOf("libvpx")}, Verdict: Refuse,
		Field: AxisChroma, Values: OneOf("gbrp"), Reason: someReason,
	}})
}

func TestRegistrationRefusesAMalformedRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule Rule
		want string
	}{
		{
			name: "an axis the vocabulary does not carry",
			rule: Rule{
				When: map[string]Match{"publish.invented": OneOf("x")}, Verdict: Refuse,
				Field: AxisChroma, Reason: someReason,
			},
			want: "matches on a declared axis",
		},
		{
			name: "a numeric match against a text axis",
			rule: Rule{
				When: map[string]Match{AxisCodec: AtLeast(3)}, Verdict: Refuse,
				Field: AxisChroma, Reason: someReason,
			},
			want: "written in the kind its axis reads",
		},
		{
			name: "a text match against a numeric axis",
			rule: Rule{
				When: map[string]Match{AxisBitrateM: OneOf("100")}, Verdict: Refuse,
				Field: AxisChroma, Reason: someReason,
			},
			want: "written in the kind its axis reads",
		},
		{
			name: "a rule stating no fact",
			rule: Rule{
				When: map[string]Match{AxisCodec: OneOf("libvpx")}, Verdict: Refuse,
				Field: AxisChroma,
			},
			want: "states the fact behind it",
		},
		{
			name: "a rule landing on no control",
			rule: Rule{
				When: map[string]Match{AxisCodec: OneOf("libvpx")}, Verdict: Refuse,
				Reason: someReason,
			},
			want: "names the control it lands on",
		},
		{
			name: "hiding one value of a control",
			rule: Rule{
				When: map[string]Match{AxisCapture: OneOf("portal")}, Verdict: Hide,
				Field: "publish.drm_map", Values: OneOf("vaapi"), Reason: someReason,
			},
			want: "hiding names no value",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held := len(registered)
			defer func() {
				switch panicked := recover().(type) {
				case nil:
					t.Errorf("Register accepted %s", tc.name)
				case string:
					if !strings.Contains(panicked, tc.want) {
						t.Errorf("Register refused %s over %q, want %q", tc.name, panicked, tc.want)
					}
				default:
					t.Errorf("Register refused %s with %v", tc.name, panicked)
				}
				if len(registered) != held {
					t.Errorf("%s reached the registry", tc.name)
				}
			}()
			Register(tc.rule)
		})
	}
}

// Registering after the registry has been read would change what is legal underneath a form that
// was already answered.
func TestRegistrationClosesOnceEvaluated(t *testing.T) {
	held := evaluated
	defer func() {
		evaluated = held
		switch panicked := recover().(type) {
		case nil:
			t.Error("a rule was registered after an evaluation")
		case string:
			if !strings.Contains(panicked, "registered before anything is evaluated") {
				t.Errorf("Register aborted over %q", panicked)
			}
		default:
			t.Errorf("Register aborted with %v", panicked)
		}
	}()

	Evaluate(facts(nil))
	Register(Rule{
		When: map[string]Match{AxisCodec: OneOf("libvpx")}, Verdict: Refuse,
		Field: AxisChroma, Values: OneOf("gbrp"), Reason: someReason,
	})
}
