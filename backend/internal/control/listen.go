// The half of the endpoint that is the same on both platforms: what a taken address means.
// Opening one is in listen_other.go and listen_windows.go.

package control

import "errors"

// ErrAddressInUse is what Listen answers with where something else already holds the control
// endpoint.
//
// It is told apart from every other reason a listen fails because it alone says this process has a
// live twin.
// The endpoint is the whole discovery mechanism (serve.go), so a backend that did not get it is one
// no shell ever reaches: it supervises nothing anybody asked for while the shells talk to the
// backend that did get it.
// The two then compete for the same relay paths and the same capture device, and the logs of the
// one nobody is talking to are the ones a reader finds first.
//
// A caller that meets it ends the process rather than carrying on headless.
// Every other listen failure, a path that cannot be created or an endpoint whose permissions cannot
// be set, leaves that judgement to the caller.
var ErrAddressInUse = errors.New("another backend is already listening on the control endpoint")
