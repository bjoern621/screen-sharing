package control

import (
	"errors"
	"strings"

	"google.golang.org/grpc"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/events"
)

// Subscribe carries what changed, for as long as the shell holds the call.
//
// The third kind of method, and the one the other two lean on: an effect answers with an empty
// message because the new state arrives here, and a read exists so a shell that has just mounted
// starts from the picture this keeps current.
// Every event carries a whole state and never a delta, so a duplicate is harmless and a dropped
// connection is recovered from by reading state again (docs/ipc-api.md, "Events").
// Nothing is computed here, the broker's events being the contract's own messages already: this
// opens a subscription, forwards it, and lets it go.
//
// An unknown kind is refused rather than ignored, with INVALID_ARGUMENT naming the kind and listing
// the ones this build carries.
// Ignored, it would leave the shell holding an open stream that never delivers, which reads as a
// backend where nothing is happening rather than as a name got wrong.
//
// A defer releases the subscription on every path out.
// The broker sends on a channel it holds until the cancel removes it, so a call that returned
// without cancelling would leave a subscriber nothing reads and every publish still walks past.
// The cancel is safe to call twice, which is what lets it be deferred here and still be called on
// an error path.
func (s *Server) Subscribe(req *screensharev1.SubscribeRequest, out grpc.ServerStreamingServer[screensharev1.Event]) error {
	assert.IsNotNil(out, "a subscription writes to the client's stream")

	feed, cancel, err := s.events.Subscribe(req.GetKinds())
	if err != nil {
		var unknown *events.UnknownKindError
		if !errors.As(err, &unknown) {
			// The broker refuses a subscription for one reason, in one type.
			// A second reason reaching here is a change to the broker this method was never told about, an
			// Entwicklungsfehler rather than anything to report to a shell.
			assert.Never("a subscription is refused only for a kind this build has none of", err)
		}
		return invalidArgument("no event kind named '%s'; this build carries %s",
			unknown.Kind, strings.Join(events.KindNames(), ", "))
	}
	defer cancel()

	done := out.Context().Done()
	for {
		select {
		case <-done:
			// A client gone or no longer listening: a shell closing its window, a process quitting.
			// That is how a subscription ends normally, so no failure is reported.
			return nil
		case event, open := <-feed:
			if !open {
				// The channel closes when the subscription is released, which on this path is the backend
				// shutting the broker down under an open call.
				return nil
			}
			if err := out.Send(event); err != nil {
				// A send that failed is the transport reporting the client gone.
				// The error carries a status already, so it goes back unwrapped rather than reclassified.
				return err
			}
		}
	}
}
