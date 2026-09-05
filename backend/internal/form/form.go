// Package form turns a settings draft into the complete description of the screen: every group,
// field, option, greying, reason, note and derived figure, already decided.
//
// The rules live here rather than in a shell because a rule written twice drifts (docs/ipc-api.md).
// The wording does not live here at all.
// Every reason, note and diagnostic is a code and the identifiers it is about, and every field
// and group is a key: what any of that reads as on screen is written where the screen
// is (api/proto/screenshare/v1/text.proto).
//
// What a value means is the tables' answer and not this package's.
// capabilities holds the codec facts, transport the carriage, gpupath the frame-memory pairs,
// publish the capture backends.
// This package reads them and says what a screen shows about them,
// which is the one thing none of them says.
//
// Resolve is idempotent by construction: one draft resolves to one form,
// so a shell may call it on every keystroke
// and a re-render from an unchanged form changes nothing on screen.
package form

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/text"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Deps is what a resolve reads that is not a fixed table.
//
// The fixed tables are package globals of capabilities, transport, gpupath and publish,
// read through rather than copied in: a copy of a table is a second definition waiting to go stale.
// What travels here is this machine's own, its monitors, its platform and its probe result,
// because a test resolves a form for a machine it is not running on.
type Deps struct {
	Monitors []display.Monitor
	Platform platform.Info
	// Device is the video driver an encode would run through.
	// The zero value is a machine that named no driver, which carries no defect
	// and greys nothing on one.
	Device capabilities.Device
	// Encoders is what the probe found.
	// The zero value is an unprobed machine rather than one with no encoders,
	// and an engine with no verdicts and no reason greys nothing.
	Encoders encoders.Availability
	// AudioDevices is what each audio kind offers on this machine, enumerated once
	// and read back (internal/audiodev).
	// Empty is a sound server that answered nothing, and each kind then keeps its own default,
	// the entry that needs no enumeration.
	AudioDevices []platform.AudioDevice
	// Portal is what the desktop portal on this machine serves, its cursor modes among them.
	// The zero value is a machine nothing asked, which greys nothing,
	// as an unprobed engine does.
	Portal portal.Capabilities
}

// state is what availability decided about one field, in the treatments
// of docs/field-availability.md: hidden, disabled with a reason, live with a note, or plain.
type state struct {
	visible bool
	enabled bool
	// reason stands in place of a disabled field, nil on an enabled one.
	reason *screensharev1.Text
	// note rides a field that stays editable and means something its label does not describe here.
	// Not a third form of unavailability.
	note *screensharev1.Text
}

// field is one control's fixed description: what it edits, how it is drawn, what its number means,
// and how its value, options and range come out of a draft.
//
// Its name and the paragraph teaching what it does are absent,
// looked up by key on the surface that draws it, where the column width, the tone
// and the reading level are known.
//
// Its availability is absent for a different reason.
// The fixed facts hold across resolves and availability is decided on every one,
// so they are two tables rather than two halves of one row.
type field struct {
	key     string
	group   string
	control screensharev1.ControlKind
	// unit is what the number means.
	// UNIT_UNSPECIFIED on a field that is not a quantity.
	unit screensharev1.Unit

	// repeat marks a row drawn once per entry of the audio source list,
	// plus once for the row that grows the list.
	// It carries a key template rather than a key and reads its value off one entry (itemValue):
	// the control is about an entry and not about the draft.
	//
	// No column names which list, because there is one: a second repeated field is what adds
	// that column, where the rest of the row already is.
	repeat bool

	// value reads what the row holds out of the draft.
	// Every row that is not repeated has one: a control with no value is one a shell cannot render.
	value func(settings.Settings) *screensharev1.FieldValue
	// itemValue is value for a repeated row, read off the entry being drawn.
	// Exactly one of the two is set, and the table asserts it.
	itemValue func(settings.AudioSource) *screensharev1.FieldValue
	// options lists what a select or radio offers, with value, note and recommended filled.
	// The enabled flag and its reason are availability's,
	// which keeps one place deciding what is greyed.
	// nil on a control that takes no options.
	options func(Deps, settings.Settings) []*screensharev1.FieldOption
	// itemOptions is options for a repeated row, given the entry as well:
	// what an audio kind holds depends on which kind that entry named.
	itemOptions func(Deps, settings.Settings, settings.AudioSource) []*screensharev1.FieldOption
	// bounds sizes a number or a slider. nil on a control that takes no range.
	bounds func(Deps, settings.Settings) *screensharev1.NumericRange
}

// group is one heading's key.
type group struct {
	key string
	// applied marks a group whose writes are settings rather than a proposal,
	// stated in full on the contract (form.proto, FieldGroup.applied).
	// A fact about what the fields mean,
	// so it is a column of this table rather than a rule each consumer restates.
	applied bool
}

// Resolve answers what may be done with these settings and what the screen says about it.
//
// The order is the fact worth stating: the draft is repaired first,
// and everything after describes the repaired draft.
// Describing what was sent while returning what was repaired would hand a shell a greyed option
// and a replacement that disagree.
//
// A draft following a built-in preset describes that preset's answer for this machine instead:
// the search runs on the repaired draft and its find replaces it whole,
// so every field, figure and diagnostic below is about what a start would run.
// The repair's account is dropped with the draft it described;
// what the search moved getting there is the preset's own business rather than a walked field.
// A preset nothing here reaches leaves the repaired draft standing,
// and blocks the start with the same verdict the preset row carries:
// the fields shown are the seed,
// so publishing them under the preset's name would put a stream on the air nobody asked for.
//
// The built-in presets are resolved against the repaired draft for a sharper form
// of the same reason: a candidate counts only where the repair leaves it untouched,
// so one stranded value still in the draft would leave every preset unreachable (presets.go).
func Resolve(d Deps, draft settings.Settings) *screensharev1.Form {
	s, repaired := Repair(d, draft)

	followed, ok := presetByKey(s.Publish.Preset)
	unreached := ok
	if ok {
		if reached, found := presetResolve(d, followed, s); found {
			s, repaired = reached, nil
			unreached = false
		}
	}

	est := estimate(d, s)
	diags := diagnostics(d, s, est)
	if unreached {
		diags = append([]*screensharev1.Diagnostic{diagnosticFor(
			screensharev1.Severity_SEVERITY_ERROR, "",
			text.Of(screensharev1.TextCode_TEXT_CODE_PRESET_UNREACHABLE,
				text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_PRESET, s.Publish.Preset),
				text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_TRANSPORT, s.Publish.Transport)))}, diags...)
	}

	// What a fresh installation holds, which every field states beside its own value.
	// Read once and handed down: the row functions that read the draft read this too,
	// and Defaults asks the machine for its hostname.
	fresh := settings.Defaults()

	// The rules are evaluated once for the whole render and handed down.
	// Every control and every option of every control asks the same question of the same draft,
	// and availabilityOf walks the entire registry, so asking per control priced a resolve in hundreds
	// of milliseconds where a shell resolves on a keystroke.
	av := availabilityOf(d, s)

	form := &screensharev1.Form{
		Settings:          wire.Settings(s),
		RepairedFieldKeys: repaired,
		Groups:            resolveGroups(av, d, s, fresh),
		Diagnostics:       diags,
		Summary:           summarize(d, s, est),
		Presets:           resolvePresets(d, s),
		Publishable:       publishable(diags),
	}
	if ok {
		// The detach rule, stated so a shell applies it rather than deciding it:
		// a write to one of these clears the followed key with the edit (presetOwnedKeys).
		form.PresetOwnedFieldKeys = presetOwnedKeys
	}

	assert.IsNotNil(form.GetSettings(), "a resolved form carries the draft it describes")
	assert.Assert(len(form.GetGroups()) > 0, "a resolved form has something to draw", len(form.GetGroups()))
	return form
}

// resolveGroups renders the groups in table order, each holding the rows of fieldTable that name
// it, in that table's order.
//
// A group no row names is dropped rather than drawn as a heading with nothing under it.
// A field availability hid is not: it is rendered with its visible flag false,
// which is how a shell is told about a control it is not to draw.
func resolveGroups(av availability, d Deps, s, fresh settings.Settings) []*screensharev1.FieldGroup {
	out := make([]*screensharev1.FieldGroup, 0, len(groups))
	for _, g := range groups {
		fields := make([]*screensharev1.Field, 0, len(fieldTable))
		drawn := false
		for i := range fieldTable {
			f := &fieldTable[i]
			if f.group != g.key {
				continue
			}
			if !f.repeat {
				fields = append(fields, resolveField(av, d, s, fresh, f, noEntry))
				continue
			}
			// One entry's controls are drawn together rather
			// than every entry's kind followed by every entry's gain.
			// The repeated rows are contiguous in the table, so the first of them draws all of them
			// and the rest are skipped.
			if drawn {
				continue
			}
			drawn = true
			fields = append(fields, resolveEntries(av, d, s, fresh, g.key)...)
		}
		if len(fields) == 0 {
			continue
		}
		out = append(out, &screensharev1.FieldGroup{Key: g.key, Fields: fields, Applied: g.applied})
	}
	return out
}

// resolveEntries draws one group's repeated rows per entry of the audio source list,
// plus once for the row that grows the list.
//
// The extra row holds the default entry rather than anything the settings carry.
// Its key names one index past the end,
// so picking a kind on it is the write that appends (keys.go, listField).
// A kind set back to none takes an entry off again, on the repair's next pass.
func resolveEntries(av availability, d Deps, s, fresh settings.Settings, group string) []*screensharev1.Field {
	var out []*screensharev1.Field
	for entry := range len(s.Publish.AudioSources) + 1 {
		for i := range fieldTable {
			f := &fieldTable[i]
			if f.group != group || !f.repeat {
				continue
			}
			out = append(out, resolveField(av, d, s, fresh, f, entry))
		}
	}
	return out
}

// noEntry is what a row that is not repeated is drawn for.
const noEntry = -1

// resolveField fills one control: its fixed description, availability's verdict on it,
// its current value, what a fresh installation would hold there, and its options
// or range with each option's own verdict.
//
// The default comes from the row's own value function read against the defaults,
// not from a second column of the table.
// One reader for both keeps the value a shell writes back and the value it puts back
// the same shape, and a row added to the table carries a default with nothing else to fill in.
func resolveField(av availability, d Deps, s, fresh settings.Settings, f *field, entry int) *screensharev1.Field {
	assert.IsNotNil(f, "a resolved field belongs to a row of the field table")
	assert.Assert(f.repeat == (entry != noEntry), "a repeated control is drawn for an entry", f.key, entry)

	key := f.key
	if f.repeat {
		key = indexedKey(f.key, entry)
	}
	st := fieldStateOf(av, f.key, entry)

	out := &screensharev1.Field{
		Key:     key,
		Control: f.control,
		Unit:    f.unit,
		Visible: st.visible,
		Enabled: st.enabled,
		Reason:  st.reason,
		Note:    st.note,
		// Liveness is what an edit to this control would cost, so a greyed
		// or hidden one promises nothing about an edit nobody can make.
		Live:         st.enabled && st.visible && av.verdicts.Live(f.key),
		Value:        fieldValue(f, s, entry),
		DefaultValue: fieldValue(f, fresh, entry),
	}

	if f.options != nil || f.itemOptions != nil {
		out.Options = resolveOptions(av, d, s, f, entry)
	}
	if f.bounds != nil {
		out.Range = f.bounds(d, s)
	}

	assert.Assert(st.enabled || st.reason != nil, "a disabled control says why", f.key)
	assert.IsNotNil(out.GetValue(), "a control shows a value", f.key)
	assert.IsNotNil(out.GetDefaultValue(), "a control states what it starts as", f.key)
	return out
}

// fieldValue is what one control holds in these settings: the draft's own value for a row
// that is not repeated, the entry's for one that is.
//
// An entry past the end of the list is the row that grows it, and it holds the default entry:
// no kind, unity gain, unmuted.
// The entry becomes exactly that the moment a kind is picked on it,
// so reading it out of settings that do not carry it answers rather than leaving a hole.
func fieldValue(f *field, s settings.Settings, entry int) *screensharev1.FieldValue {
	if !f.repeat {
		assert.IsNotNil(f.value, "a control has a value to show", f.key)
		return f.value(s)
	}
	assert.IsNotNil(f.itemValue, "a repeated control has a value to show", f.key)
	return f.itemValue(audioEntry(s, entry))
}

// audioEntry is one entry of the list, and the default entry for the row past its end.
func audioEntry(s settings.Settings, entry int) settings.AudioSource {
	if entry >= 0 && entry < len(s.Publish.AudioSources) {
		return s.Publish.AudioSources[entry]
	}
	return settings.DefaultAudioSource()
}

// resolveOptions is one control's entries with each one's verdict,
// the reachable ones ahead of the ruled-out ones.
//
// The partition is the only thing this adds to the builder's list, and it is stable,
// so each half keeps the order its builder gave it.
// What moves is that everything this combination allows is reachable from the top of the list.
//
// Nothing is dropped: an option a neighbouring combination allows stays, greyed,
// because its reason is what names the thing to change (docs/field-availability.md).
// Sinking it says the same about priority that removing it would say about existence,
// and only one of the two is true.
//
// The order is decided here rather than on a surface because the enabled flag is.
// A shell re-sorting on it would be a second place deciding what the list looks like,
// one a second shell could disagree with and one the repair walking a stranded value to the first
// legal entry cannot see (repair.go, docs/ipc-api.md, "The rule").
func resolveOptions(av availability, d Deps, s settings.Settings, f *field, entry int) []*screensharev1.FieldOption {
	var built []*screensharev1.FieldOption
	if f.repeat {
		assert.IsNotNil(f.itemOptions, "an ordered option list belongs to a control that offers entries", f.key)
		built = f.itemOptions(d, s, audioEntry(s, entry))
	} else {
		assert.IsNotNil(f.options, "an ordered option list belongs to a control that offers entries", f.key)
		built = f.options(d, s)
	}
	reachable := make([]*screensharev1.FieldOption, 0, len(built))
	var ruledOut []*screensharev1.FieldOption

	for _, o := range built {
		enabled, reason := optionStateOf(av, f.key, o.GetValue(), entry)
		o.Enabled = enabled
		o.Reason = reason

		if enabled {
			reachable = append(reachable, o)
			continue
		}
		// A recommendation is a hint about this combination, the same combination the greying answers
		// for, so an entry ruled out here carries none: a mark on an entry nobody can pick is the form
		// arguing with itself, and a builder marking one unconditionally has no way to know.
		o.Recommended = false
		ruledOut = append(ruledOut, o)
	}

	out := append(reachable, ruledOut...)
	assert.Assert(len(out) == len(built), "every entry the builder made is offered", f.key, len(out), len(built))
	assert.Assert(len(out) == 0 || out[0].GetEnabled() || len(reachable) == 0,
		"a reachable entry leads the list", f.key, len(reachable))
	return out
}

// publishable reports whether no diagnostic ranks as an error.
//
// Stated on the form rather than left to be derived, so a shell disables its start button
// without ranking diagnostics itself.
func publishable(diags []*screensharev1.Diagnostic) bool {
	for _, w := range diags {
		if w.GetSeverity() == screensharev1.Severity_SEVERITY_ERROR {
			return false
		}
	}
	return true
}

// Value constructors.
// The oneof is what lets a shell bind a control generically instead of switching on the key.

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

// bounded makes a range.
// A step of zero means one, as absence does on the contract, so a caller states the step
// only where it is not one.
func bounded(min, max, step int) *screensharev1.NumericRange {
	assert.Assert(min <= max, "a range's floor is under its ceiling", min, max)
	return &screensharev1.NumericRange{Min: int64(min), Max: int64(max), Step: int64(step)}
}
