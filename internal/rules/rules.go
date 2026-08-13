// Package rules is the one constraint system: which settings a combination leaves reachable,
// and what is stated about the ones it does not.
//
// A rule is a row rather than a function.
// It names the facts that have to hold for it to bind, what it then does, which control that lands
// on, which of that control's values it takes, and the fact it is stating.
// A rule naming one axis is broad and a rule naming five is surgical, so one shape covers both "no
// VP8 encoder has a colour range field" and "this codec on this capture backend on this engine
// alone".
//
// It replaces capabilities.Gap, which could name a codec, an engine, an option and a value and
// nothing else.
// Every fact outside that shape grew a table of its own with a consumer written against it,
// and each of those consumers restated the part of the answer the gap mechanism already knew how to
// give.
//
// Rules are declared where the fact lives and registered into one evaluator,
// so the transport package keeps its carriage beside the code that serializes it and the platform
// package keeps its source table.
// What is central is the vocabulary and not the constraints: an axis is declared once (axis.go) and
// a rule names it, which is what lets a reason carry the identifiers it is about with no
// translation table in between.
//
// Nothing here decides how a refusal reads.
// A rule carries a code, the evaluator attaches the identifiers the rule matched on,
// and the sentence is written where the column width and the tone are visible (docs/ipc-api.md).
//
// Every rule is a compiled-in fact, so a malformed one is an Entwicklungsfehler and fails at load.
package rules

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Verdict is what a rule does to the control it names.
//
// The three are the treatments docs/field-availability.md describes, and a rule states which one it
// is rather than leaving it to be worked out from how many values it took.
// Whether a control is hidden or greyed is a judgement about what a reader on another combination
// has any reason to see, which is not derivable from the size of a value set.
type Verdict int

const (
	// Refuse takes values away from a control: the ones Values names, or every value it has where
	// Values names none.
	Refuse Verdict = iota + 1
	// Note leaves the control editable and says what its value means under this combination.
	// It is not a weaker refusal.
	// It exists so a knob a builder does forward is never greyed, which would leave the encoder using
	// a number the form refused to show.
	Note
	// Hide takes the control off the screen, for a backend implementation knob whose help describes a
	// mechanism a reader on any other backend has no reason to meet.
	Hide
	// Live says a change to this control reaches the pipeline that is already carrying the stream,
	// so applying it costs no viewer a reconnect.
	//
	// It is the one verdict that adds rather than takes away, and its default is therefore the
	// opposite of the other three: a control nobody declares live is one a change rebuilds the
	// pipeline for, which is what every control was before any of them could be written to a running
	// child.
	Live
)

// verdicts lists every verdict a rule may state, for the registration check.
var verdicts = []Verdict{Refuse, Note, Hide, Live}

// Rule is one constraint: the facts it binds under, and what it then says.
type Rule struct {
	// When is the facts that have to hold, keyed by axis.
	// An axis left out is one this rule does not care about, so a rule with an empty When binds
	// always.
	// Every axis named here is asserted to be declared, which is what keeps a typo from producing a
	// rule that silently never binds.
	When map[string]Match
	// Verdict is what binding does.
	Verdict Verdict
	// Field is the control the verdict lands on, as the form keys it.
	// It is not necessarily an axis: a rule may refuse a field nothing else ever matches on.
	Field string
	// Values is which of that field's values the verdict takes, and the zero Match is every one of
	// them.
	// A numeric Match refuses a band of a numeric control and is what the offered range is narrowed
	// by, so a codec's ceiling is stated here rather than in a column a second consumer has to read.
	Values Match
	// Reason is the fact being stated, as a code.
	// The identifiers it is about are attached by the evaluator from the axes When named,
	// so a row states which fact it is and never which words carry it.
	Reason screensharev1.TextCode
	// Args is what the statement needs that no axis carries: a figure the row knows and the
	// configuration does not, such as the top of a codec's quantizer scale.
	// A row states only those, because everything the rule matched on is attached from the facts
	// themselves and an argument written twice is one that can disagree.
	Args []*screensharev1.TextArg
}

// registered is every rule this process knows, in registration order.
//
// It is package state on purpose: the rules are compiled-in facts, each package registers its own
// at init, and a second registry would be a second answer to what is legal.
// The guard below is what keeps that honest.
var registered []Rule

// evaluated records that the registry has been read, so a late registration fails loudly.
// A package registering after the first resolve would change what is legal underneath a form that
// had already been answered, which is a bug in the registering package rather than a condition to
// survive.
var evaluated bool

// Register adds rules to the registry.
// It is called from a package's init, and every row is checked as it arrives so a malformed rule
// fails at load rather than by binding nothing for the life of the process.
func Register(rs ...Rule) {
	assert.Assert(!evaluated, "a rule is registered before anything is evaluated", len(rs))

	before := len(registered)
	for _, r := range rs {
		assert.Assert(contains(verdicts, r.Verdict), "a rule states what binding does", int(r.Verdict), r.Field)
		assert.Assert(r.Field != "", "a rule names the control it lands on", int(r.Verdict))
		// Every verdict that replaces something on screen says what it is replacing it with.
		// Live replaces nothing: it grants the control a property it did not have,
		// and there is no sentence to write in place of a control nothing was taken from.
		assert.Assert((r.Reason != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED) == (r.Verdict != Live),
			"a rule states the fact behind it, unless it takes nothing away", r.Field, int(r.Verdict))
		for i, a := range r.Args {
			// A nil argument would be dropped on the way to the statement, leaving a sentence with a hole
			// where a figure the row promised should be.
			assert.IsNotNil(a, "an argument a rule carries is one it filled", r.Field, i)
		}
		for name, m := range r.When {
			axis, ok := Declared(name)
			assert.Assert(ok, "a rule matches on a declared axis", name, r.Field)
			assert.Assert(!m.everything(), "a matched axis names what it has to read", name, r.Field)
			assert.Assert(m.numeric == (axis.Kind == KindNumber),
				"a match is written in the kind its axis reads", name, r.Field)
		}
		// A note goes either way, and both are in use: the bitrate carries one about the whole control
		// where a mode turns it into a burst ceiling, and the pixel format carries one per entry saying
		// what that entry costs a viewer to decode.
		//
		// Hiding is about the control and never about one of its values, so a value set here would be a
		// rule asking for a treatment that does not exist.
		// Liveness is about the control for the same reason: what reaches a running pipeline is a field's
		// value, whichever value it is.
		assert.Assert(r.Verdict != Hide || r.Values.everything(),
			"hiding names no value", r.Field)
		assert.Assert(r.Verdict != Live || r.Values.everything(),
			"liveness names no value", r.Field)
	}
	registered = append(registered, rs...)

	assert.Assert(len(registered) == before+len(rs),
		"every registered rule reaches the registry", len(registered), before, len(rs))
}

// Registered is every rule in the registry, for a caller that reports on the rule set rather than
// evaluating it.
// It hands back the slice this package holds, which callers read and do not write.
func Registered() []Rule {
	return registered
}

func contains[T comparable](xs []T, x T) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
