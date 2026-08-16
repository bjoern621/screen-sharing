package control

import (
	"context"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/wire"
)

// CheckRelay dials every leg of the relay the draft names and answers what each listener said.
//
// An effect for the reason the measurements are: it reaches the network and waits out a listener
// that is not there, so a shell asking on every keystroke would dial the relay on every keystroke.
// Unlike them it is not refused beside a live stream, a handshake per leg competing with nothing
// the stream is spending.
//
// A relay that answers nothing is a response and never a status: every leg comes back with its own
// verdict, and "the relay is down" is a thing the screen says rather than a thing the call failed
// at (docs/ipc-api.md, "Errors").
//
// The draft decides what is dialled, so a request carrying none is refused by the gate every effect
// uses: an empty one would check the relay nobody named and report every leg unaddressed, which
// reads as an answer about the relay on screen.
func (s *Server) CheckRelay(ctx context.Context, req *screensharev1.CheckRelayRequest) (*screensharev1.CheckRelayResponse, error) {
	draft, err := draftOf(req.GetSettings(), "check")
	if err != nil {
		return nil, err
	}

	return &screensharev1.CheckRelayResponse{Legs: wire.RelayLegs(s.backend.CheckRelay(ctx, draft))}, nil
}
