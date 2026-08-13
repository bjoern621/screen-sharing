// The half of the endpoint that is the same on both platforms: what it means when the address
// cannot be taken.
// Opening it is in listen_other.go and listen_windows.go.

package control

import "errors"

// ErrAddressInUse is what Listen answers with when something else already holds the control
// endpoint.
//
// It is separated from every other reason a listen fails because it is the only one that says this
// process has a live twin.
// The endpoint is the whole discovery mechanism (serve.go), so a backend that did not get it is a
// backend no shell will ever reach: it would sit there supervising nothing anybody asked for,
// while the shells talk to the backend that did get it.
// The two then compete for the same relay paths and the same capture device,
// and the logs of the one nobody is talking to are the ones a reader finds first.
//
// A caller that meets it should end the process rather than carry on headless.
// Every other listen failure - a path that cannot be created, an endpoint whose permissions cannot
// be set - leaves that judgement to the caller.
var ErrAddressInUse = errors.New("another backend is already listening on the control endpoint")
