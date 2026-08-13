package control

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The two error kinds this repository distinguishes cross this boundary differently,
// and the difference is the whole error model (docs/ipc-api.md).
//
// An Umgebungsfehler - a condition the app must survive - is a gRPC status.
// It is expected, it is the user's to see, and its message is prose written for a person.
// The helpers below are the only way one is produced here, so a status code is chosen against the
// table in the contract rather than per call site.
//
// An Entwicklungsfehler - a broken internal contract - never crosses.
// assert panics in the backend, as it does everywhere else in this repository.
// A shell that could receive a bug as a status would start handling bugs, and a handled bug is a
// bug that ships.
//
// One consequence is worth stating on its own: an unreachable relay is not a call failure.
// GetRelayStatus succeeds and returns a snapshot whose reachable is false,
// because "the relay is down" is a thing the screen has to say rather than a thing the call failed
// at.

// invalidArgument is a request naming something that cannot exist: an unknown transport,
// an empty stream name.
func invalidArgument(format string, args ...any) error {
	return status.Errorf(codes.InvalidArgument, format, args...)
}

// failedPrecondition is a well-formed request the world is not ready for: already publishing,
// nothing to apply settings to, a measurement while a stream is live.
func failedPrecondition(format string, args ...any) error {
	return status.Errorf(codes.FailedPrecondition, format, args...)
}

// unavailable is a relay that could not be reached or a child process that could not be started.
func unavailable(format string, args ...any) error {
	return status.Errorf(codes.Unavailable, format, args...)
}

// notFound is a named preset, log or stream that does not exist.
func notFound(format string, args ...any) error {
	return status.Errorf(codes.NotFound, format, args...)
}

// resourceExhausted is a bounded resource that was over-asked, such as the test-stream count.
func resourceExhausted(format string, args ...any) error {
	return status.Errorf(codes.ResourceExhausted, format, args...)
}
