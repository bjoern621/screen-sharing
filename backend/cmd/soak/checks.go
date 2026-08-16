package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// What a control owes the widget bound to it, and what a form owes the screen as a whole.
//
// Every check here is one a reader meets as a blank or as a figure that is wrong rather than
// missing: a slider whose top a drag never lands on, a control carrying one number while the draft
// holds another, an entry listed twice, a sentence anchored to a field the form does not draw.
// None of them needs silicon, so they run on every resolve the walk makes.

// encoderRateCeiling is the largest rate an encoder takes, every one of them reading it as a signed
// 32-bit count of bits per second.
// A command carrying more than this is a pipeline that dies at launch on a value the form offered.
const encoderRateCeiling = math.MaxInt32

// longNumber finds the figures in a rendered command large enough to be worth checking against that
// ceiling. Ten digits is the width at which a bits-per-second value first reaches it.
var longNumber = regexp.MustCompile(`[0-9]{10,}`)

// checkControl holds one control to what a widget bound to it needs.
func checkControl(run *session, field *v1.Field, settings *v1.Settings) {
	key := field.GetKey()

	// A widget draws Field.value and a start sends the draft, so a disagreement between them is a
	// figure on screen that no stream will ever run.
	if shown, held := shownValue(field.GetValue()), readField(settings, key); shown != "" && held != "" && !sameValue(shown, held) {
		run.report.report("form.value_disagrees_with_settings", "form/value-mismatch/"+key,
			fmt.Sprintf("the control carries %q and the draft holds %q", shown, held),
			map[string]string{"key": key, "shown": shown, "held": held}, settings)
	}

	// A read-back figure lists nothing and edits nothing, so its cause is what the screen puts in
	// place of an interaction (form.proto, CONTROL_KIND_READONLY).
	if field.GetControl() == v1.ControlKind_CONTROL_KIND_READONLY && field.GetEnabled() {
		run.report.report("form.readonly_enabled", "form/readonly-enabled/"+key,
			"a figure carrying no input arrives editable", map[string]string{"key": key}, settings)
	}

	listed := map[string]bool{}
	for _, option := range field.GetOptions() {
		value := option.GetValue()
		if listed[value] {
			run.report.report("form.duplicate_option", "form/duplicate/"+key+"/"+value,
				"an entry is listed twice", map[string]string{"key": key, "option": value}, settings)
		}
		listed[value] = true

		if option.GetRecommended() && !option.GetEnabled() {
			run.report.report("form.recommended_option_disabled", "form/recommended-greyed/"+key+"/"+value,
				"an entry is emphasised and cannot be chosen",
				map[string]string{"key": key, "option": value}, settings)
		}
	}

	checkRangeShape(run, field, settings)
}

// checkRangeShape holds a numeric control to the band it states.
//
// The stops a slider offers are the round figures inside the band plus both ends, so a 20 ms floor
// stepping by 50 stops on 20, 50, 100 and reaches 8000
// (avalonia/.../Fields/ViewModel/FieldViewModel.cs, Ticks).
// A held value off that ladder is one no drag lands on, and a drag past it is a figure changing
// under the reader's hands.
//
// An entry outside the band is legal on a number-select and on nothing else, the burst ceiling's
// zero being the case that exists (form.proto, CONTROL_KIND_NUMBER_SELECT).
func checkRangeShape(run *session, field *v1.Field, settings *v1.Settings) {
	key := field.GetKey()
	r := field.GetRange()
	if r == nil {
		return
	}

	step := r.GetStep()
	if step == 0 {
		step = 1
	}
	if step < 0 {
		run.report.report("form.range_step_negative", "form/negative-step/"+key,
			fmt.Sprintf("a control steps by %d", step), map[string]string{"key": key}, settings)
		return
	}

	value, ok := heldNumber(field.GetValue())
	if !ok || offered(field, value) || value == r.GetMin() || value == r.GetMax() {
		return
	}
	switch {
	case value < r.GetMin() || value > r.GetMax():
		run.report.report("form.value_out_of_range", "form/out-of-range/"+key,
			fmt.Sprintf("the control holds %d and is offered %d..%d", value, r.GetMin(), r.GetMax()),
			map[string]string{"key": key, "value": fmt.Sprint(value)}, settings)
	case value%step != 0:
		run.report.report("form.value_off_step", "form/off-step/"+key,
			fmt.Sprintf("the control holds %d, which is no multiple of the %d it steps by and neither end of %d..%d",
				value, step, r.GetMin(), r.GetMax()),
			map[string]string{"key": key, "value": fmt.Sprint(value), "step": fmt.Sprint(step)}, settings)
	}
}

// checkWhole states what a form owes as a whole rather than per control.
func checkWhole(run *session, form *v1.Form, settings *v1.Settings) {
	drawn := map[string]bool{}
	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			drawn[field.GetKey()] = true
		}
	}

	blocking := ""
	for _, d := range form.GetDiagnostics() {
		if d.GetSeverity() == v1.Severity_SEVERITY_UNSPECIFIED {
			run.report.report("form.diagnostic_without_severity", "form/no-severity/"+d.GetFieldKey(),
				"a diagnostic is ranked by nothing", map[string]string{"key": d.GetFieldKey()}, settings)
		}
		if d.GetText().GetCode() == v1.TextCode_TEXT_CODE_UNSPECIFIED {
			run.report.report("form.diagnostic_without_text", "form/no-text/"+d.GetFieldKey(),
				"a diagnostic carries no statement to word",
				map[string]string{"key": d.GetFieldKey()}, settings)
		}
		// A diagnostic is drawn beside the control it names, so one naming a field this form does
		// not carry is a sentence with nowhere to land.
		if key := d.GetFieldKey(); key != "" && !drawn[key] {
			run.report.report("form.diagnostic_off_form", "form/orphan-diagnostic/"+key,
				fmt.Sprintf("a diagnostic names %s and no group carries that field", key),
				map[string]string{"key": key}, settings)
		}
		if d.GetSeverity() == v1.Severity_SEVERITY_ERROR {
			blocking = fmt.Sprint(d.GetText().GetCode())
		}
	}

	// Publishable is given rather than derived so that a start button disables without a shell
	// ranking diagnostics, which holds only while the two agree (form.proto, Form.publishable).
	switch {
	case form.GetPublishable() && blocking != "":
		run.report.report("form.publishable_with_error", "form/publishable-error/"+blocking,
			"the form says the draft can publish and carries a blocking diagnostic",
			map[string]string{"diagnostic": blocking}, settings)
	case !form.GetPublishable() && blocking == "":
		run.report.report("form.unpublishable_without_error", "form/unpublishable-silent",
			"the form refuses the draft and names no blocking diagnostic", nil, settings)
	}

	checkPresetShape(run, form, settings)
	checkSummary(run, form, settings)
}

// checkPresetShape holds the built-in presets to what each one owes: an outcome or a reason, never
// both and never neither, and at most one of them already delivered.
func checkPresetShape(run *session, form *v1.Form, settings *v1.Settings) {
	selected := []string{}
	for _, preset := range form.GetPresets() {
		key := preset.GetKey()
		if key == "" {
			run.report.report("form.preset_without_key", "form/preset-unnamed",
				"a preset arrives under no key", nil, settings)
			continue
		}
		if preset.GetSelected() {
			selected = append(selected, key)
		}

		reached, refused := preset.GetSettings() != nil, preset.GetReason() != nil
		if reached == refused {
			run.report.report("form.preset_answer_ambiguous", "form/preset-ambiguous/"+key,
				fmt.Sprintf("%s carries settings %t and a reason %t", key, reached, refused),
				map[string]string{"preset": key}, settings)
		}
		if refused && preset.GetReason().GetCode() == v1.TextCode_TEXT_CODE_UNSPECIFIED {
			run.report.report("form.preset_refused_without_reason", "form/preset-no-reason/"+key,
				fmt.Sprintf("%s is out of reach and names no reason", key),
				map[string]string{"preset": key}, settings)
		}
	}
	// The promises are written pairwise disjoint, so a draft delivering two of them is a table that
	// stopped holding them apart (form.proto, BuiltinPreset.selected).
	if len(selected) > 1 {
		run.report.report("form.presets_both_selected", "form/presets-selected/"+strings.Join(selected, "+"),
			fmt.Sprintf("the draft is reported as delivering %s at once", strings.Join(selected, " and ")),
			map[string]string{"presets": strings.Join(selected, ",")}, settings)
	}
}

// checkSummary holds the figures under the diagnostics to what a screen can print.
func checkSummary(run *session, form *v1.Form, settings *v1.Settings) {
	if !form.GetPublishable() {
		return
	}
	summary := form.GetSummary()

	for _, figure := range []struct {
		name  string
		value float64
		above float64
	}{
		{"bitrate_mbps", summary.GetEstimate().GetBitrateMbps(), 0},
		{"raw_mbps", summary.GetEstimate().GetRawMbps(), 0},
	} {
		switch {
		case math.IsNaN(figure.value) || math.IsInf(figure.value, 0):
			run.report.report("form.estimate_not_finite", "form/estimate-nan/"+figure.name,
				fmt.Sprintf("the estimate states %s as %v", figure.name, figure.value),
				map[string]string{"figure": figure.name}, settings)
		case figure.value <= figure.above:
			run.report.report("form.estimate_missing", "form/estimate-zero/"+figure.name,
				fmt.Sprintf("a publishable draft predicts %s at %.2f", figure.name, figure.value),
				map[string]string{"figure": figure.name}, settings)
		}
	}
	if headroom := summary.GetEstimate().GetHeadroomMbps(); math.IsNaN(headroom) || math.IsInf(headroom, 0) {
		run.report.report("form.estimate_not_finite", "form/estimate-nan/headroom_mbps",
			fmt.Sprintf("the estimate states headroom_mbps as %v", headroom),
			map[string]string{"figure": "headroom_mbps"}, settings)
	}

	// Every encoder reads its rate as a signed 32-bit count of bits per second and refuses the
	// encode rather than clamping, so a figure above that ceiling is a launch that fails on a value
	// the form offered as legal.
	for _, token := range longNumber.FindAllString(summary.GetCommand(), -1) {
		value, err := strconv.ParseInt(token, 10, 64)
		if err == nil && value > encoderRateCeiling {
			run.report.report("form.command_value_overflows", "form/command-overflow/"+run.codecOf(settings),
				fmt.Sprintf("the rendered command carries %s, above the %d an encoder takes", token, encoderRateCeiling),
				map[string]string{"value": token, "codec": run.codecOf(settings)}, settings)
			break
		}
	}
}

// checkRepairsNamed holds a resolve to the moves it reported making.
//
// A shell reports the move instead of quietly rewriting what somebody typed (form.proto,
// Form.repaired_field_keys), which it can do only for the keys the resolve names.
// A value that moved unannounced is a control that changes under a reader's hands.
func checkRepairsNamed(run *session, form *v1.Form, draft *v1.Settings, moved, chosen string) {
	named := map[string]bool{}
	for _, key := range form.GetRepairedFieldKeys() {
		named[key] = true
	}

	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			key := field.GetKey()
			// A field the draft never set is one the backend fills, which is a default and not a
			// repair: gain carries presence exactly so that a silent source and an unset one differ,
			// and a read answers both with zero (backend/internal/wire/settings.go, audioGain).
			if named[key] || shifted(draft, form.GetSettings(), key) || !stated(draft, key) {
				continue
			}
			was, is := readField(draft, key), readField(form.GetSettings(), key)
			if was == "" || is == "" || sameValue(was, is) {
				continue
			}
			run.report.report("form.repair_not_named", "form/silent-repair/"+key,
				fmt.Sprintf("%s went from %q to %q and the resolve named no repair of it", key, was, is),
				map[string]string{"key": key, "was": was, "became": is, "moved": moved, "value": chosen},
				draft)
		}
	}
}

// shifted says whether a key names an entry of a list whose length moved between the two drafts.
//
// A removal takes the entries after it down a row, so every key past the removed one names a
// different entry in the two drafts and comparing them by key compares two different sources.
// The removal itself is what the resolve names as the repair.
func shifted(before, after *v1.Settings, key string) bool {
	open := strings.Index(key, "[")
	if open < 0 {
		return false
	}
	was, ok := listLength(before, key[:open])
	if !ok {
		return false
	}
	is, ok := listLength(after, key[:open])
	return ok && was != is
}

// stated says whether a draft carries a value of its own for a key, as against leaving the field
// for the backend to fill.
func stated(settings *v1.Settings, key string) bool {
	message, desc, err := locate(settings, key, false)
	return err == nil && message.Has(desc)
}

func listLength(settings *v1.Settings, key string) (int, bool) {
	message, desc, err := locate(settings, key, false)
	if err != nil || !desc.IsList() {
		return 0, false
	}
	return message.Get(desc).List().Len(), true
}

// coverage is what the walk reached, so a run states what it never asked for rather than leaving
// that to be guessed from a seed.
//
// An entry offered on every form and held on none is a corner of the settings space no finding can
// come out of, which is a gap in the run and not a defect in the product.
type coverage struct {
	mu      sync.Mutex
	offered map[string]map[string]bool
	held    map[string]map[string]bool
	bands   map[string]*band
}

// band is one numeric control's ends and whether the walk stood on them.
type band struct {
	low, high     int64
	sawLow, sawHi bool
}

func newCoverage() *coverage {
	return &coverage{
		offered: map[string]map[string]bool{},
		held:    map[string]map[string]bool{},
		bands:   map[string]*band{},
	}
}

// see records what one form offered and what it held.
//
// Only what the walk may move: a field it is barred from writing was never going to be covered, and
// reporting the fact would be the run describing its own rules back to itself.
// Only the entries of a select, too, a number-select's being shortcuts recomputed against whatever
// the target holds rather than a list to work through (form.proto, CONTROL_KIND_NUMBER_SELECT).
func (c *coverage) see(form *v1.Form) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			key := field.GetKey()
			if !field.GetVisible() || !field.GetEnabled() || frozen[key] || strings.HasPrefix(key, "relay.") {
				continue
			}
			lists := field.GetControl() == v1.ControlKind_CONTROL_KIND_SELECT ||
				field.GetControl() == v1.ControlKind_CONTROL_KIND_RADIO
			if lists {
				for _, option := range field.GetOptions() {
					if !option.GetEnabled() {
						continue
					}
					if c.offered[key] == nil {
						c.offered[key] = map[string]bool{}
					}
					c.offered[key][option.GetValue()] = true
				}
				if c.held[key] == nil {
					c.held[key] = map[string]bool{}
				}
				c.held[key][shownValue(field.GetValue())] = true
			}
			if r := field.GetRange(); r != nil {
				reach, ok := c.bands[key]
				if !ok || reach.low != r.GetMin() || reach.high != r.GetMax() {
					reach = &band{low: r.GetMin(), high: r.GetMax()}
					c.bands[key] = reach
				}
				if held, ok := heldNumber(field.GetValue()); ok {
					reach.sawLow = reach.sawLow || held == r.GetMin()
					reach.sawHi = reach.sawHi || held == r.GetMax()
				}
			}
		}
	}
}

// report states every entry the walk was offered and never held, and every band it never stood at
// an end of.
func (c *coverage) report(run *session) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range sorted(c.offered) {
		var missed []string
		for value := range c.offered[key] {
			if !c.held[key][value] {
				missed = append(missed, value)
			}
		}
		if len(missed) == 0 {
			continue
		}
		sort.Strings(missed)
		run.report.report("form.coverage_gap", "form/never-held/"+key,
			fmt.Sprintf("%s offered %s and the walk held none of them", key, strings.Join(missed, ", ")),
			map[string]string{"key": key, "missed": strings.Join(missed, ",")}, nil)
	}

	for key, reach := range c.bands {
		if reach.sawLow && reach.sawHi {
			continue
		}
		var ends []string
		if !reach.sawLow {
			ends = append(ends, fmt.Sprintf("%d", reach.low))
		}
		if !reach.sawHi {
			ends = append(ends, fmt.Sprintf("%d", reach.high))
		}
		run.report.report("form.coverage_gap", "form/never-at-end/"+key,
			fmt.Sprintf("%s was never held at %s", key, strings.Join(ends, " or ")),
			map[string]string{"key": key, "ends": strings.Join(ends, ",")}, nil)
	}
}

func sorted(m map[string]map[string]bool) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// shownValue is the figure a widget draws, in the spelling a settings read answers in.
func shownValue(value *v1.FieldValue) string {
	switch kind := value.GetKind().(type) {
	case *v1.FieldValue_Text:
		return kind.Text
	case *v1.FieldValue_Number:
		return strconv.FormatInt(kind.Number, 10)
	case *v1.FieldValue_Decimal:
		return strconv.FormatFloat(kind.Decimal, 'g', -1, 64)
	case *v1.FieldValue_Flag:
		return strconv.FormatBool(kind.Flag)
	}
	return ""
}

// heldNumber is a control's value where it carries one, and false where the field is no quantity.
func heldNumber(value *v1.FieldValue) (int64, bool) {
	switch kind := value.GetKind().(type) {
	case *v1.FieldValue_Number:
		return kind.Number, true
	case *v1.FieldValue_Decimal:
		return int64(kind.Decimal), true
	}
	return 0, false
}

// offered says whether a number is one of the entries listed beside a band, which is the one thing
// a number-select can say that a range alone cannot.
func offered(field *v1.Field, value int64) bool {
	for _, option := range field.GetOptions() {
		if n, err := strconv.ParseInt(option.GetValue(), 10, 64); err == nil && n == value {
			return true
		}
	}
	return false
}

// sameValue compares two spellings of one value, as numbers where both are numbers.
// 6 and 6.0 are one figure, and a check reading them as two would report every double on the form.
func sameValue(a, b string) bool {
	if a == b {
		return true
	}
	x, errA := strconv.ParseFloat(a, 64)
	y, errB := strconv.ParseFloat(b, 64)
	return errA == nil && errB == nil && x == y
}
