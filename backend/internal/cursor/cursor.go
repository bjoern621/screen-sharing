// Package cursor names what the pointer does in the captured frames.
//
// Settings, capture backends and the form all read these values,
// so they live in a package depending on nothing.
// Which backend serves which mode lives with the backend (internal/publish/cursor.go).
package cursor

// Embedded is drawn into the picture, costing bitrate.
// Hidden is absent.
// Metadata travels beside the stream as a position,
// so a viewer that draws one keeps it sharp at any scale and one that does not sees none.
const (
	Embedded = "embedded"
	Hidden   = "hidden"
	Metadata = "metadata"
)

// Modes lists every mode, in the order a form offers them.
var Modes = []string{Embedded, Hidden, Metadata}

// Known reports whether mode is one of Modes.
// Every string is legal input: the value comes off settings or another process,
// so unknown is an answer rather than a broken contract.
func Known(mode string) bool {
	for _, m := range Modes {
		if m == mode {
			return true
		}
	}
	return false
}
