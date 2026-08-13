package form

import screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

// Reading a statement back, for the tests that assert on what a rule said.
//
// They ask the way a surface asks: which statement is this, and which identifier is it about.
// That is deliberate - a test that matched on wording would be asserting on the one thing this
// package no longer decides, and would start failing the first time a shell reworded its screen
// (api/proto/screenshare/v1/text.proto).

// codeOf is the statement's code, and TEXT_CODE_UNSPECIFIED for no statement at all.
func codeOf(t *screensharev1.Text) screensharev1.TextCode {
	return t.GetCode()
}

// idOf is one identifier argument, and the empty string where the statement carries none under that
// name.
// Absence is a real answer: several statements name a value some combinations have and others do
// not, and a caller with none is stating the shorter form of the same fact.
func idOf(t *screensharev1.Text, name screensharev1.TextArgName) string {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetId()
		}
	}
	return ""
}

// idsOf is a list argument, and nil where the statement carries none under that name.
func idsOf(t *screensharev1.Text, name screensharev1.TextArgName) []string {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetIds().GetIds()
		}
	}
	return nil
}

// nestedOf is a statement carried inside a statement, and nil where there is none.
func nestedOf(t *screensharev1.Text, name screensharev1.TextArgName) *screensharev1.Text {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetText()
		}
	}
	return nil
}
