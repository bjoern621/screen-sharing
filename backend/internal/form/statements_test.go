package form

import screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

// Statement readers, for the tests that assert on what a rule said.
//
// They ask what a surface asks: which statement this is, and which identifier it is about.
// A test matching on wording would assert the one thing this package does not decide,
// and would break the first time a shell reworded its screen (api/proto/screenshare/v1/text.proto).

// codeOf is the statement's code, TEXT_CODE_UNSPECIFIED for no statement at all.
func codeOf(t *screensharev1.Text) screensharev1.TextCode {
	return t.GetCode()
}

// idOf is one identifier argument, empty where the statement carries none under that name.
// Absence is an answer: a statement names a value some combinations have and others do not,
// and a caller with none states the shorter form of the same fact.
func idOf(t *screensharev1.Text, name screensharev1.TextArgName) string {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetId()
		}
	}
	return ""
}

// idsOf is a list argument, nil where the statement carries none under that name.
func idsOf(t *screensharev1.Text, name screensharev1.TextArgName) []string {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetIds().GetIds()
		}
	}
	return nil
}

// nestedOf is a statement carried inside a statement, nil where there is none.
func nestedOf(t *screensharev1.Text, name screensharev1.TextArgName) *screensharev1.Text {
	for _, arg := range t.GetArgs() {
		if arg.GetName() == name {
			return arg.GetText()
		}
	}
	return nil
}
