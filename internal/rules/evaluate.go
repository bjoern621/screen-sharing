package rules

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/text"
)

// Verdicts is what every rule said about one configuration.
//
// It is read through rather than walked: a caller asks about the control it is drawing and gets the
// answer, so nothing outside this package holds a copy of which rule bound.
// Each answer carries every reason that produced it, unordered, because a control blocked by two
// facts is blocked by two facts and picking one of them to show is a judgement about a screen
// (docs/field-availability.md).
type Verdicts struct {
	hidden     map[string]bool
	live       map[string]bool
	fieldStops map[string][]*screensharev1.Text
	valueStops map[string]map[string][]*screensharev1.Text
	fieldNotes map[string][]*screensharev1.Text
	valueNotes map[string]map[string][]*screensharev1.Text
	bands      map[string][]band
}

// band is one numeric refusal: which numbers are gone and why.
type band struct {
	match  Match
	reason *screensharev1.Text
}

// Evaluate answers the registry for one configuration.
//
// Reading the registry closes it: a package registering afterwards would change what is legal
// underneath a form that has already been answered.
func Evaluate(f Facts) Verdicts {
	evaluated = true
	return EvaluateRules(f, registered)
}

// EvaluateRules answers one rule set for one configuration, leaving the registry alone.
// It is what Evaluate delegates to, and it is what a caller reporting on a subset of the rules
// asks, so neither has a second copy of how a rule binds.
func EvaluateRules(f Facts, rs []Rule) Verdicts {
	assert.IsNotNil(f, "an evaluation carries the facts to match against")

	v := Verdicts{
		hidden:     map[string]bool{},
		live:       map[string]bool{},
		fieldStops: map[string][]*screensharev1.Text{},
		valueStops: map[string]map[string][]*screensharev1.Text{},
		fieldNotes: map[string][]*screensharev1.Text{},
		valueNotes: map[string]map[string][]*screensharev1.Text{},
		bands:      map[string][]band{},
	}

	for _, r := range rs {
		if !r.binds(f) {
			continue
		}
		// A live rule states no fact to attach identifiers to, so the statement is not built for one:
		// it grants the control a property rather than replacing it with a sentence.
		if r.Verdict == Live {
			v.live[r.Field] = true
			continue
		}
		reason := r.state(f)
		assert.IsNotNil(reason, "a bound rule states the fact behind it", r.Field, int(r.Verdict))

		switch r.Verdict {
		case Hide:
			v.hidden[r.Field] = true
		case Refuse:
			switch {
			case r.Values.everything():
				v.fieldStops[r.Field] = append(v.fieldStops[r.Field], reason)
			case r.Values.numeric:
				v.bands[r.Field] = append(v.bands[r.Field], band{match: r.Values, reason: reason})
			default:
				v.valueStops[r.Field] = appendUnder(v.valueStops[r.Field], r.Values.listed(), reason)
			}
		case Note:
			if r.Values.everything() {
				v.fieldNotes[r.Field] = append(v.fieldNotes[r.Field], reason)
			} else {
				v.valueNotes[r.Field] = appendUnder(v.valueNotes[r.Field], r.Values.listed(), reason)
			}
		default:
			assert.Never("a bound rule states a known verdict", int(r.Verdict), r.Field)
		}
	}
	return v
}

// binds reports whether every fact this rule names reads what it asks for.
//
// An axis the rule names and the facts do not carry is asserted rather than treated as not
// matching: a rule that quietly binds nothing renders as a combination the app allows,
// which is indistinguishable on screen from one nobody constrained.
func (r Rule) binds(f Facts) bool {
	for name, m := range r.When {
		value, ok := f[name]
		assert.Assert(ok, "the facts carry every axis a rule matches on", name, r.Field)
		if !m.binds(value) {
			return false
		}
	}
	return true
}

// state is the rule's fact with the identifiers it is about attached.
//
// The arguments are the axes the rule matched on, carrying what those axes actually read,
// plus the control and the value the verdict lands on.
// That is what lets a row state a code and nothing else: the sentence naming a codec,
// an engine and a pixel format gets all three without the row spelling any of them a second time.
//
// The axes are attached in vocabulary order rather than in map order, so the same rule against the
// same facts produces the same statement every time.
func (r Rule) state(f Facts) *screensharev1.Text {
	args := make([]*screensharev1.TextArg, 0, len(r.When)+len(r.Args)+2)
	for _, axis := range axes {
		if _, named := r.When[axis.Name]; !named {
			continue
		}
		value := f[axis.Name]
		if axis.Kind == KindNumber {
			args = append(args, text.Num(axis.Arg, int64(value.Number())))
			continue
		}
		args = append(args, text.ID(axis.Arg, value.Text()))
	}

	// What the row knows and the facts do not, before the control it lands on,
	// so a statement reads subject first and target last however many figures ride between.
	args = append(args, r.Args...)

	args = append(args, text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_OPTION, r.Field))
	// The value rides along only where the verdict took exactly one, since a statement about three
	// greyed entries has no single value to name and the list of them is the control's own business.
	if listed := r.Values.listed(); len(listed) == 1 {
		args = append(args, text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_VALUE, listed[0]))
	}
	return text.Of(r.Reason, args...)
}

// Visible reports whether the control is drawn at all.
func (v Verdicts) Visible(field string) bool {
	return !v.hidden[field]
}

// Live reports whether a change to this control reaches a pipeline that is already running.
// False is the answer for every control nothing declared live, which is the answer that costs a
// reconnect and is therefore the safe one to be wrong about.
func (v Verdicts) Live(field string) bool {
	return v.live[field]
}

// Enabled reports whether the control takes edits.
// A control with values taken from it is still enabled: what is greyed there is the entries,
// not the control.
func (v Verdicts) Enabled(field string) bool {
	return len(v.fieldStops[field]) == 0
}

// Reasons is why the control is inert, empty where it is not.
func (v Verdicts) Reasons(field string) []*screensharev1.Text {
	return v.fieldStops[field]
}

// ValueEnabled reports whether one entry of a control may be selected.
func (v Verdicts) ValueEnabled(field, value string) bool {
	return len(v.valueStops[field][value]) == 0
}

// ValueReasons is why one entry is greyed, empty where it is not.
func (v Verdicts) ValueReasons(field, value string) []*screensharev1.Text {
	return v.valueStops[field][value]
}

// Notes is what a still-editable control means under this combination.
func (v Verdicts) Notes(field string) []*screensharev1.Text {
	return v.fieldNotes[field]
}

// ValueNotes is what one entry costs or means, for the entries that carry a note rather than a
// greying.
func (v Verdicts) ValueNotes(field, value string) []*screensharev1.Text {
	return v.valueNotes[field][value]
}

// NumberAllowed reports whether a numeric control accepts this reading.
//
// It is the same fact Bounds states, asked about one number instead of about the ends,
// which is what a validator wants: the form narrows a control and a publish refuses a value,
// and both have to come off the one evaluation or the slider will offer what the encoder rejects.
func (v Verdicts) NumberAllowed(field string, n int) bool {
	return len(v.NumberReasons(field, n)) == 0
}

// NumberReasons is why a numeric control refuses this reading, empty where it takes it.
func (v Verdicts) NumberReasons(field string, n int) []*screensharev1.Text {
	var out []*screensharev1.Text
	for _, b := range v.bands[field] {
		if b.match.binds(NumberValue(n)) {
			out = append(out, b.reason)
		}
	}
	return out
}

// Bounds narrows the ends a numeric control is offered between by every band refused on it,
// so the range a form offers and the range the publish accepts are one answer.
//
// A field with no band is offered what it was given, which is the usual case.
func (v Verdicts) Bounds(field string, low, high int) (int, int) {
	assert.Assert(low <= high, "an offered range runs from its low end to its high one", field, low, high)

	for _, b := range v.bands[field] {
		low, high = b.match.narrow(low, high)
	}
	return low, high
}

// BoundReasons is why a numeric control is offered less than its full range,
// empty where nothing narrowed it.
func (v Verdicts) BoundReasons(field string) []*screensharev1.Text {
	out := make([]*screensharev1.Text, 0, len(v.bands[field]))
	for _, b := range v.bands[field] {
		out = append(out, b.reason)
	}
	return out
}

// appendUnder files one reason under each value it was stated about, creating the inner map on the
// way so a caller never has to.
func appendUnder(
	under map[string][]*screensharev1.Text, values []string, reason *screensharev1.Text,
) map[string][]*screensharev1.Text {
	assert.IsNotNil(reason, "a filed reason is a statement")

	if under == nil {
		under = map[string][]*screensharev1.Text{}
	}
	for _, value := range values {
		under[value] = append(under[value], reason)
	}
	return under
}
