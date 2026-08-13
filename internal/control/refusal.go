package control

import (
	"errors"
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"
)

// A backend refusal the request earned, as against the machine's state.
//
// The contract states INVALID_ARGUMENT for a request naming something that does not exist on this
// build or this machine, and FAILED_PRECONDITION for one whose arguments are all real and whose
// moment is wrong (api/proto/screenshare/v1/control.proto).
// A backend answering both with a plain error leaves this side to tell them apart by the sentence,
// which is a contract derived from prose written for a person (fromBackend).
// The type is what carries the difference instead.

// Refused marks an error as earned by the request: a monitor index no output is enumerated under, a
// transport this build has no viewer for.
// Everything else a backend returns is the world failing at something it was legal to ask for.
type Refused struct {
	cause error
}

// Refuse builds a Refused carrying the sentence the user reads.
// The wording stays the backend's: this side decides the code, never the words.
func Refuse(format string, args ...any) error {
	assert.Assert(format != "", "a refusal says what it refused")

	return &Refused{cause: fmt.Errorf(format, args...)}
}

func (e *Refused) Error() string { return e.cause.Error() }

func (e *Refused) Unwrap() error { return e.cause }

// refused reports whether err carries a Refused anywhere in its chain, so a backend wrapping one
// with context keeps the code it earned.
func refused(err error) bool {
	var r *Refused
	return errors.As(err, &r)
}
