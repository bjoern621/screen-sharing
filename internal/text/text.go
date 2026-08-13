// Package text builds the statements the backend makes about a configuration.
//
// A statement is a code and the identifiers it is about, never a sentence
// (api/proto/screenshare/v1/text.proto).
// This package is the one place they are assembled, so a rule that greys a control states what is
// true and nothing about how it reads: the wording, the ordering of the clauses,
// the short form and the long one are the shell's, written where the layout is visible.
//
// Every constructor asserts what the contract requires of the value it carries.
// An identifier that arrived empty and a figure that is a placeholder both render as a sentence
// with a hole in it, and a hole in a reason is indistinguishable on screen from a control that was
// greyed for no reason at all.
// Both are Entwicklungsfehler, so both fail here rather than on the surface that cannot tell which
// happened.
package text

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Of is one statement: which one, and what it is about.
//
// The code is asserted rather than defaulted because the zero value means "not set" on every enum
// in this package, and a statement that failed to say which statement it is is not a statement a
// shell can be asked to render.
func Of(code screensharev1.TextCode, args ...*screensharev1.TextArg) *screensharev1.Text {
	assert.Assert(code != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"a statement names which statement it is")

	out := &screensharev1.Text{Code: code}
	for _, a := range args {
		// A nil argument is an optional clause the caller had nothing for, the way out a table declares
		// for some rows and not others, and dropping it here is what lets a builder pass the same
		// argument list either way.
		if a == nil {
			continue
		}
		out.Args = append(out.Args, a)
	}

	assert.Assert(out.Code != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"an assembled statement still names which statement it is", len(out.Args))
	return out
}

// ID carries one identifier of the axis the name declares.
//
// An empty identifier is dropped rather than carried, for the same reason Of drops a nil argument.
// Several statements name a value that some combinations have and others do not,
// such as the engine a way out points at or the family behind a probe verdict,
// and the caller that has none is stating the shorter form of the same fact.
func ID(name screensharev1.TextArgName, id string) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")

	if id == "" {
		return nil
	}
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Id{Id: id}}
}

// IDs carries several identifiers of one axis, in the order the domain table declares them.
// How they read together, the comma, the "or", the final one, is the shell's.
//
// An empty list is dropped, which is the same shorter form ID's empty identifier is:
// "no leg carries this" and "these legs carry it" are one statement whose list of ways out happens
// to be empty.
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
// Zero is carried rather than dropped: it is a measurement on every figure this contract states one
// for, and a statement that quoted no uplink where the user had stated none would read as a line
// with no capacity rather than as one nobody measured.
func Num(name screensharev1.TextArgName, n int64) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Number{Number: n}}
}

// Dec carries a fractional figure in the unit its name declares.
// How many places it is shown to is the shell's: a rate rounded for a chip and the same rate
// spelled out in a tooltip are one figure at two widths.
func Dec(name screensharev1.TextArgName, v float64) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Decimal{Decimal: v}}
}

// Nested carries a statement inside a statement, which is how a reason quotes the fact behind it
// without either half knowing the other's wording.
//
// A nil statement is dropped, so a caller with nothing to quote passes the same argument and
// produces the shorter form.
func Nested(name screensharev1.TextArgName, t *screensharev1.Text) *screensharev1.TextArg {
	assert.Assert(name != screensharev1.TextArgName_TEXT_ARG_NAME_UNSPECIFIED,
		"an argument names which substitution it fills")

	if t == nil {
		return nil
	}
	return &screensharev1.TextArg{Name: name, Value: &screensharev1.TextArg_Text{Text: t}}
}
