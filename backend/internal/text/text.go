// Package text builds the statements the backend makes about a configuration.
//
// A statement is a code and the identifiers it is about, never a sentence
// (api/proto/screenshare/v1/text.proto).
// The one place they are assembled, so a rule that greys a control states what is true and nothing
// about how it reads: the wording, the order of the clauses, the short form and the long one are
// the shell's, written where the layout is visible.
//
// A string produced outside this app's vocabulary stays raw rather than crossing as a statement,
// being data and not vocabulary: Summary.command, Summary.command_error, ExitInfo.message,
// RelayStatus.error.
// Each is shown verbatim and is never matched against.
//
// Every constructor asserts what the contract requires of the value it carries.
// An empty identifier and a placeholder figure both render as a sentence with a hole in it, and on
// screen that is indistinguishable from a control greyed for no reason at all.
// Both are Entwicklungsfehler, so both fail here rather than on the surface that cannot tell which
// happened.
package text

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Of is one statement: which one, and what it is about.
//
// The code is asserted and not defaulted, the zero value meaning "not set" on every enum here.
// A shell cannot be asked to render a statement that does not say which statement it is.
func Of(code screensharev1.TextCode, args ...*screensharev1.TextArg) *screensharev1.Text {
	assert.Assert(code != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"a statement names which statement it is")

	out := &screensharev1.Text{Code: code}
	for _, a := range args {
		// A nil argument is an optional clause the caller had nothing for, the way out a table declares
		// on some rows and not others.
		// Dropping it here is what lets a builder pass the same argument list either way.
		if a == nil {
			continue
		}
		out.Args = append(out.Args, a)
	}

	assert.Assert(out.Code != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"an assembled statement still names which statement it is", len(out.Args))
	return out
}

// ID carries one identifier of the axis its name declares.
//
// An empty identifier is dropped, for the reason Of drops a nil argument.
// Several statements name a value some combinations have and others do not, the engine a way out
// points at or the family behind a probe verdict, and a caller with none is stating the shorter
// form of the same fact.
func ID(name screensharev1.TextArgName, id string) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")

	if id == "" {
		return nil
	}
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Id{Id: id}}
}

// IDs carries several identifiers of one axis, in the order the domain table declares them.
// How they read together, the comma, the "or", the last one, is the shell's.
//
// An empty list is dropped, the same shorter form ID's empty identifier is: "no leg carries this"
// and "these legs carry it" are one statement whose list of ways out happens to be empty.
func IDs(name screensharev1.TextArgName, ids []string) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")

	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		assert.Assert(id != "", "a listed identifier is one the domain declares", name.String())
	}
	return &screensharev1.TextArg{
		Name:  name,
		Value: &screensharev1.TextArg_Ids{Ids: &screensharev1.IdList{Ids: ids}},
	}
}

// Num carries a whole figure in the unit its name declares.
//
// Zero is carried and not dropped, being a measurement on every figure this contract states one
// for: a statement quoting no uplink where the user stated none would read as a line with no
// capacity rather than as one nobody measured.
func Num(name screensharev1.TextArgName, n int64) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Number{Number: n}}
}

// Dec carries a fractional figure in the unit its name declares.
// The places it is shown to are the shell's, one rate rounded for a chip and spelled out in a
// tooltip being one figure at two widths.
func Dec(name screensharev1.TextArgName, v float64) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Decimal{Decimal: v}}
}

// Nested carries a statement inside a statement, which is how a reason quotes the fact behind it
// with neither half knowing the other's wording.
//
// A nil statement is dropped, so a caller with nothing to quote passes the same argument and gets
// the shorter form.
func Nested(name screensharev1.TextArgName, t *screensharev1.Text) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")

	if t == nil {
		return nil
	}
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Text{Text: t}}
}
