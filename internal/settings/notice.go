package settings

import (
	"errors"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/text"
)

// StoreNotice turns a store failure into the statement a surface shows about it,
// and nil for no failure at all.
//
// The code is the caller's because the two stores fail the same way and mean different things.
// The working settings coming back as defaults and the preset list coming back empty are two
// different things for a user to be told, and one function that guessed which had happened would be
// guessing from the error text.
//
// A failure that could not move the old values aside carries no path, and the statement then says
// the shorter thing rather than pointing at a file that is not there.
func StoreNotice(code screensharev1.TextCode, err error) *screensharev1.Text {
	assert.Assert(code != screensharev1.TextCode_TEXT_CODE_UNSPECIFIED,
		"a store notice names which statement it is")

	if err == nil {
		return nil
	}
	kept := ""
	var unreadable *StoreUnreadable
	if errors.As(err, &unreadable) {
		kept = unreadable.Kept
	}
	return text.Of(code, text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_PATH, kept))
}
