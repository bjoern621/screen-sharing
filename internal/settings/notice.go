package settings

import (
	"errors"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/text"
)

// StoreNotice turns a store failure into the statement a surface shows, nil for no failure.
//
// The code is the caller's, the two stores failing alike and meaning different things: the working
// settings coming back as defaults and the preset list coming back empty are two things to tell a
// user, and one function choosing between them would be reading the error text.
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
