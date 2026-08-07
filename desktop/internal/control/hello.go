package control

import (
	"context"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Hello settles the contract version before any other method is reached.
//
// It is the first call on a connection and the only one a backend and a shell who
// disagree about the contract can both still understand, which is why a major this
// build does not implement is refused here with a sentence naming both numbers rather
// than allowed through to arrive as a field that is silently empty (docs/ipc-api.md,
// "Versioning"). A shell told "this backend is on 1 and you were built against 2" can
// say so to the user; a shell handed a response it half understands cannot.
//
// A shell that names no major at all sends zero, which is not this build's major and
// is refused the same way. That is the right answer rather than a special case: the
// field exists so the version is settled explicitly, and a request that left it unset
// has settled nothing.
//
// The minor is never refused. It is informational in both directions - a shell built
// against a lower minor works, and one built against a higher minor may find a method
// missing - and the missing method reports itself as UNIMPLEMENTED where it is called,
// which is a more useful place to learn it than here.
func (s *Server) Hello(ctx context.Context, req *screensharev1.HelloRequest) (*screensharev1.HelloResponse, error) {
	assert.IsNotNil(req, "a handshake carries a request")

	if req.GetProtocolMajor() != ProtocolMajor {
		return nil, failedPrecondition(
			"this backend implements control protocol major %d and the shell was built against major %d; both sides have to be on the same major",
			ProtocolMajor, req.GetProtocolMajor())
	}

	// The client name carries no behaviour, and is logged for the one question a backend
	// serving several shells cannot otherwise answer: which of them was on the other end
	// of the connection that then misbehaved.
	logger.Infof("control: '%s' connected on protocol major %d", req.GetClient(), req.GetProtocolMajor())

	answer := &screensharev1.HelloResponse{
		ProtocolMajor:  ProtocolMajor,
		ProtocolMinor:  ProtocolMinor,
		BackendVersion: s.version,
	}

	assert.Assert(answer.GetProtocolMajor() == req.GetProtocolMajor(),
		"a settled handshake leaves both sides on one major", answer.GetProtocolMajor(), req.GetProtocolMajor())
	return answer, nil
}
