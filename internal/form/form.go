// Package form turns a settings draft into the complete description of the screen.
//
// It is where the rules that used to live in the Wails frontend's util/ now live:
// the greyings and their reasons, the dropdown construction, the repair of a stranded
// value, the bitrate prediction and the diagnostics. They sit here rather than in a
// shell because a rule written twice drifts, and the app has three shells
// (docs/ipc-api.md).
//
// What does not live here is the wording. Every reason, note and diagnostic below is a
// code and the identifiers it is about, and every field and group is a key: what any of
// that reads as on screen is written where the screen is
// (api/proto/screenshare/v1/text.proto). The division is the same one the package
// already made between the domain tables and itself - which values exist is theirs,
// which are legal is this package's, and how they read is the surface's.
//
// Nothing here decides what a value means; the tables do that. capabilities holds
// the codec facts, transport the carriage, gpupath the frame-memory pairs and
// publish the capture backends. This package reads them and says what a screen
// should show about them, which is the one thing none of them says.
//
// Resolve is idempotent by construction: the same draft resolves to the same form,
// so a shell may call it on every keystroke and a shell that re-renders from an
// unchanged form changes nothing on screen.
package form

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Deps is everything a resolve reads that is not a fixed table.
//
// The fixed tables are package globals of capabilities, transport, gpupath and
// publish, and are read through rather than copied in: a table is the same on every
// call and a copy of one is a second definition waiting to go stale. What travels
// here is the part that is this machine's - its monitors, its platform, its probe
// result - because a test resolves a form for a machine it is not running on.
type Deps struct {
	Monitors []display.Monitor
	Platform platform.Info
	// Encoders is the probe result. A zero value is an unprobed machine rather than
	// one with no encoders, and availability treats it as such: an engine with no
	// verdicts and no reason greys nothing.
	Encoders encoders.Availability
}

// state is what availability decided about one field.
//
// The four treatments of docs/field-availability.md are the four ways this can
// read: hidden, disabled with a reason, live with a note, or plain.
type state struct {
	visible bool
	enabled bool
	// reason is the statement shown in place of a disabled field, nil on an enabled
	// one.
	reason *screensharev1.Text
	// note is carried by a field that stays editable and means something its label
	// does not describe here. It is not a third form of unavailability.
	note *screensharev1.Text
}

// field is one control's fixed description: what it edits, how it is drawn, what its
// number means, and how its value, options and range are read out of a draft.
//
// What it is called and the paragraph teaching what it does are deliberately absent.
// Both are looked up by key on the surface that draws it, which is where the column
// width, the tone and the reading level are known.
//
// The availability is absent for a different reason. A field's fixed facts do not
// change between resolves and its availability changes on every one, so they are two
// tables and not two halves of one row.
type field struct {
	key     string
	group   string
	control screensharev1.ControlKind
	// unit is what the number means, and UNIT_UNSPECIFIED on a field that is not a
	// quantity.
	unit screensharev1.Unit

	// value reads what the field holds now out of the draft. Every field has one: a
	// control with no value is a control a shell cannot render.
	value func(settings.Settings) *screensharev1.FieldValue
	// options lists the entries a select or radio offers, with value, note and
	// recommended filled. The enabled flag and its reason are left to availability,
	// which is what keeps one place deciding what is greyed. nil on the controls that
	// take no options.
	options func(Deps, settings.Settings) []*screensharev1.FieldOption
	// bounds sizes a number or a slider, nil on the controls that take no range.
	bounds func(Deps, settings.Settings) *screensharev1.NumericRange
}

// group is one heading's key and the order its fields render in, which is the order
// they appear in the fields table.
type group struct {
	key string
	// applied marks a group whose writes are settings rather than a proposal, which the
	// contract states in full (form.proto, FieldGroup.applied). It is a fact about what
	// the fields mean, so it is one column of this table rather than a rule each
	// consumer restates.
	applied bool
}

// Resolve is the whole answer to "what may the user do with these settings, and what
// should the screen say about it".
//
// The order is the one thing about it worth stating: the draft is repaired first, and
// everything after that describes the repaired draft. Resolving the fields against
// what was sent and returning what was repaired would produce a form whose greyed
// option and whose replacement disagree, which is exactly the fork the contract
// exists to remove.
//
// The built-in presets are resolved against the repaired draft for a sharper form of the
// same reason: each candidate is kept only if the repair leaves it untouched, so a draft
// with a stranded value still in it would leave every preset unreachable (presets.go).
func Resolve(d Deps, draft settings.Settings) *screensharev1.Form {
	s, repaired := Repair(d, draft)

	est := estimate(d, s)
	diags := diagnostics(d, s, est)

	// What a fresh installation starts with, which every field states beside its own
	// value. It is read once and handed down rather than per field: the same row
	// functions read it as read the draft, and Defaults asks the machine for its
	// hostname.
	fresh := settings.Defaults()

	form := &screensharev1.Form{
		Settings:          wire.Settings(s),
		RepairedFieldKeys: repaired,
		Groups:            resolveGroups(d, s, fresh),
		Diagnostics:       diags,
		Summary:           summarize(d, s, est),
		Presets:           resolvePresets(d, s),
		Publishable:       publishable(diags),
	}

	assert.IsNotNil(form.GetSettings(), "a resolved form carries the draft it describes")
	assert.Assert(len(form.GetGroups()) > 0, "a resolved form has something to draw", len(form.GetGroups()))
	return form
}

// resolveGroups renders every group in table order, and within it every field the
// fields table assigns to it, in that table's order.
//
// A hidden field contributes nothing: the contract's visible flag exists so a shell
// can be told about a control it should not draw, and a control no shell should ever
// draw for these settings is one the form has no reason to name. Both are the same
// decision - availability's - and this is where the second half of it is spent.
func resolveGroups(d Deps, s, fresh settings.Settings) []*screensharev1.FieldGroup {
	out := make([]*screensharev1.FieldGroup, 0, len(groups))
	for _, g := range groups {
		fields := make([]*screensharev1.Field, 0, len(fieldTable))
		for i := range fieldTable {
			f := &fieldTable[i]
			if f.group != g.key {
				continue
			}
			fields = append(fields, resolveField(d, s, fresh, f))
		}
		if len(fields) == 0 {
			continue
		}
		out = append(out, &screensharev1.FieldGroup{Key: g.key, Fields: fields, Applied: g.applied})
	}
	return out
}

// resolveField fills one control: its fixed description, what availability decided
// about it, its current value, what a fresh installation would hold there, and its
// options or range with each option's own verdict on it.
//
// The default is the row's own value function read against the defaults rather than a
// second column of the table. One reader for both is what keeps the value a shell
// writes back and the value it puts back the same shape, and it is why a field added to
// the table carries a default with nothing else to fill in.
func resolveField(d Deps, s, fresh settings.Settings, f *field) *screensharev1.Field {
	assert.IsNotNil(f, "a resolved field belongs to a row of the field table")
	assert.IsNotNil(f.value, "a control has a value to show", f.key)

	st := fieldState(d, s, f.key)

	out := &screensharev1.Field{
		Key:          f.key,
		Control:      f.control,
		Unit:         f.unit,
		Visible:      st.visible,
		Enabled:      st.enabled,
		Reason:       st.reason,
		Note:         st.note,
		Value:        f.value(s),
		DefaultValue: f.value(fresh),
	}

	if f.options != nil {
		out.Options = resolveOptions(d, s, f)
	}
	if f.bounds != nil {
		out.Range = f.bounds(d, s)
	}

	assert.Assert(st.enabled || st.reason != nil, "a disabled control says why", f.key)
	assert.IsNotNil(out.GetValue(), "a control shows a value", f.key)
	assert.IsNotNil(out.GetDefaultValue(), "a control states what it starts as", f.key)
	return out
}

// resolveOptions is one control's entries with each one's verdict on it, the reachable
// ones in front of the ruled-out ones.
//
// The partition is the only thing this adds to the builder's list, and it is stable, so
// each half keeps the order the builder gave it: the chroma ladder still runs from most
// colour detail to least, the capture backends still arrive in the registry's order, and
// the roadmap codecs still follow the implemented ones. What moves is that everything
// this combination allows is reachable from the top of the list - a Windows machine meets
// Desktop Duplication before it meets a capture backend only macOS runs, and a reader
// scanning a dropdown reads the answers they can pick before the ones they cannot.
//
// Nothing is dropped, which is the rule this sits under rather than an exception to it:
// an option a neighbouring combination allows stays present and greyed with its reason,
// because that reason is what names the thing to change (docs/field-availability.md).
// Sinking it says the same thing about priority that removing it would say about
// existence, and only one of the two is true.
//
// It is decided here rather than on a surface for the reason every other verdict is. The
// enabled flag is this package's answer, and a shell re-sorting on it would be a second
// place deciding what the list looks like - one that a second shell could disagree with,
// and that the repair walking a stranded value to the first legal entry could not see
// (repair.go, docs/ipc-api.md, "The rule").
func resolveOptions(d Deps, s settings.Settings, f *field) []*screensharev1.FieldOption {
	assert.IsNotNil(f.options, "an ordered option list belongs to a control that offers entries", f.key)

	built := f.options(d, s)
	reachable := make([]*screensharev1.FieldOption, 0, len(built))
	var ruledOut []*screensharev1.FieldOption

	for _, o := range built {
		enabled, reason := optionState(d, s, f.key, o.GetValue())
		o.Enabled = enabled
		o.Reason = reason

		if enabled {
			reachable = append(reachable, o)
			continue
		}
		ruledOut = append(ruledOut, o)
	}

	out := append(reachable, ruledOut...)
	assert.Assert(len(out) == len(built), "every entry the builder made is offered", f.key, len(out), len(built))
	assert.Assert(len(out) == 0 || out[0].GetEnabled() || len(reachable) == 0,
		"a reachable entry leads the list", f.key, len(reachable))
	return out
}

// publishable reports whether the settings can be published as they stand, which is
// whether no diagnostic ranks as an error.
//
// It is stated on the form rather than left to be derived so a shell disables its
// start button without ranking diagnostics itself, which is the same reason every other
// verdict here is computed in Go.
func publishable(diags []*screensharev1.Diagnostic) bool {
	for _, w := range diags {
		if w.GetSeverity() == screensharev1.Severity_SEVERITY_ERROR {
			return false
		}
	}
	return true
}

// The value constructors. A field's value function returns one of these, and the
// oneof is what lets a shell bind a control generically instead of switching on the
// key.

func stringValue(v string) *screensharev1.FieldValue {
	return &screensharev1.FieldValue{Kind: &screensharev1.FieldValue_Text{Text: v}}
}

func number(v int) *screensharev1.FieldValue {
	return &screensharev1.FieldValue{Kind: &screensharev1.FieldValue_Number{Number: int64(v)}}
}

func decimal(v float64) *screensharev1.FieldValue {
	return &screensharev1.FieldValue{Kind: &screensharev1.FieldValue_Decimal{Decimal: v}}
}

func flag(v bool) *screensharev1.FieldValue {
	return &screensharev1.FieldValue{Kind: &screensharev1.FieldValue_Flag{Flag: v}}
}

// bounded is the range constructor. A step of zero means one, which is what the
// contract says absence means, so a caller states the step only where it is not one.
func bounded(min, max, step int) *screensharev1.NumericRange {
	assert.Assert(min <= max, "a range's floor is under its ceiling", min, max)
	return &screensharev1.NumericRange{Min: int64(min), Max: int64(max), Step: int64(step)}
}
