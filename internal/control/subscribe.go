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
// It is the third kind of method and the one the other two lean on: every effect answers with an
// empty message because this is where the new state arrives, and every read exists so a shell that
// has just mounted starts from the same picture this keeps current.
// Nothing is computed here - the broker's events are already the contract's own messages - so what
// this method does is open a subscription, forward it, and let it go.
//
// An unknown kind is refused rather than ignored.
// A shell that asked for a kind this build has none of would otherwise hold an open stream that
// never delivers, and would read that as a backend with nothing happening rather than as a name it
// got wrong.
// The refusal names the kind and lists the ones this build carries, so the shell learns it on the
// call it made the mistake on.
//
// The subscription is released on every path out, by a defer.
// The broker sends on a channel it holds until the cancel removes it, so a call that returned
// without cancelling would leave a subscriber that nothing reads and that every publish still has
// to walk past.
// The cancel is safe to call twice, which is what lets it be deferred here and still be called
// anywhere else on an error path.
func (s *Server) Subscribe(req *screensharev1.SubscribeRequest, out grpc.ServerStreamingServer[screensharev1.Event]) error {
	assert.IsNotNil(out, "a subscription writes to the client's stream")

	feed, cancel, err := s.events.Subscribe(req.GetKinds())
	if err != nil {
		var unknown *events.UnknownKindError
		if !errors.As(err, &unknown) {
			// The broker refuses a subscription for one reason and states it in one type.
			// A second reason arriving here is a change to the broker that this method was not told about,
			// which is a broken internal contract rather than a condition to report to a shell.
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
			// The client went away or stopped listening.
			// That is how a subscription ends normally - a shell closing its window,
			// a process quitting - so it is not a failure and is not reported as one.
			return nil
		case event, open := <-feed:
			if !open {
				// The channel closes when the subscription is released, which on this path means the backend is
				// shutting the broker down under an open call.
				return nil
			}
			if err := out.Send(event); err != nil {
				// A send that failed is the transport saying the client is gone.
				// It already carries a status, so it travels as it is rather than being reclassified.
				return err
			}
		}
	}
}
