// The half of the endpoint that is the same on both platforms: what a taken address means.
// Opening one: listen_other.go, listen_windows.go.

package control

import "errors"

// ErrAddressInUse is Listen's answer where something else already holds the control endpoint.
//
// Alone among listen failures it says this process has a live twin.
// The endpoint is the whole discovery mechanism (serve.go),
// so a backend that did not get it reaches no shell and supervises nothing anybody asked for.
// The shells talk to the backend that did get it,
// and the two compete for the same relay paths and the same capture device.
// The logs a reader finds first are the ones of the backend nobody is talking to.
//
// A caller meeting it ends the process rather than carrying on headless.
// Every other listen failure, a path that cannot be created or an endpoint whose permissions
// cannot be set, leaves that judgement to the caller.
var ErrAddressInUse = errors.New("another backend is already listening on the control endpoint")
