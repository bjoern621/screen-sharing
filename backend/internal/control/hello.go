package control

import (
	"context"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Hello settles the contract version before any other method is reached
// (docs/ipc-api.md, "Versioning").
//
// The first call on a connection,
// and the only one two sides that disagree about the contract can both still understand.
// So a major this build does not implement is FAILED_PRECONDITION,
// with both numbers in the sentence rather than a field that silently arrives empty.
// A shell told which major each side is on can say so to the user.
// One handed a response it half understands cannot.
//
// A shell naming no major sends zero, which is not this build's and is refused the same way.
// The field exists so the version is settled explicitly,
// and a request that left it unset settled nothing.
//
// The minor is never refused.
// It is informational in both directions: a shell built against a lower minor works,
// and one built against a higher minor may find a method missing,
// reported as UNIMPLEMENTED at the call rather than here.
func (s *Server) Hello(ctx context.Context, req *screensharev1.HelloRequest) (*screensharev1.HelloResponse, error) {
	assert.IsNotNil(req, "a handshake carries a request")

	if req.GetProtocolMajor() != ProtocolMajor {
		return nil, failedPrecondition(
			"this backend implements control protocol major %d and the shell was built against major %d. Both sides have to be on the same major",
			ProtocolMajor, req.GetProtocolMajor())
	}

	// The client name carries no behaviour.
	// Logged for the one question a backend serving several shells cannot otherwise answer:
	// which of them held the connection that then misbehaved.
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
