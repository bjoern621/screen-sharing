package control

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The two error kinds cross this boundary differently,
// and the difference is the whole error model (docs/ipc-api.md, "Errors").
//
// An Umgebungsfehler, a condition the app must survive, is a gRPC status:
// expected, the user's to see, and carrying prose written for a person.
// These helpers are the only way one is produced here,
// so a code is chosen against the contract's table rather than per call site.
//
// An Entwicklungsfehler never crosses.
// assert panics in the backend, as everywhere else in this repository,
// and a shell that could receive a bug as a status would start handling bugs.
//
// An unreachable relay is not a call failure:
// GetRelayStatus succeeds with a snapshot whose reachable is false,
// "the relay is down" being a thing the screen says rather than a thing the call failed at.

// invalidArgument is a request naming something that cannot exist: an unknown transport,
// an empty stream name.
func invalidArgument(format string, args ...any) error {
	return status.Errorf(codes.InvalidArgument, format, args...)
}

// failedPrecondition is a well-formed request the world is not ready for:
// a different pipeline already publishing, nothing to apply settings to,
// a measurement while a stream is live.
func failedPrecondition(format string, args ...any) error {
	return status.Errorf(codes.FailedPrecondition, format, args...)
}

// unavailable is the world failing at something legal to ask for: a relay out of reach,
// a child process that would not start.
func unavailable(format string, args ...any) error {
	return status.Errorf(codes.Unavailable, format, args...)
}

// notFound is a preset, log or decode that the name does not reach.
func notFound(format string, args ...any) error {
	return status.Errorf(codes.NotFound, format, args...)
}

// resourceExhausted is a bounded resource over-asked: more test streams than this machine runs.
func resourceExhausted(format string, args ...any) error {
	return status.Errorf(codes.ResourceExhausted, format, args...)
}
