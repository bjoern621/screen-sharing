// Package share names what of this machine a capture reads.
//
// Settings, capture backends and the form all read these values,
// so they live in a package depending on nothing.
// Which backend serves which kind lives with the backend (internal/publish/share.go).
package share

// Monitor is one whole output, Window one window of one application,
// Region a rectangle of the virtual desktop.
const (
	Monitor = "monitor"
	Window  = "window"
	Region  = "region"
)

// Kinds lists every kind, in the order a form offers them.
// Widest first: a reader who wants the whole screen stops at the first entry.
var Kinds = []string{Monitor, Window, Region}

// Known reports whether kind is one of Kinds.
// Every string is legal input, the value coming off settings or another process,
// so unknown is an answer rather than a broken contract.
func Known(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}
