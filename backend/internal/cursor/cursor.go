// Package cursor names what the pointer does in the captured frames.
//
// The settings hold one of these, the capture backends decide which they serve and the form offers
// the list, so the values live in a package that depends on nothing and all three read it.
// Which backend serves which mode is a fact about the backend and lives with it
// (internal/publish/cursor.go).
package cursor

// The pointer modes, three values rather than a flag with an exception.
// An embedded pointer is drawn into the picture, costing bitrate and blurring with everything else
// the encoder spends bits on.
// A hidden one is absent.
// A metadata pointer travels beside the stream as a position, so a viewer that draws one keeps it
// sharp at any scale and a viewer that does not sees none at all.
const (
	Embedded = "embedded"
	Hidden   = "hidden"
	Metadata = "metadata"
)

// Modes lists every mode, in the order a form offers them.
var Modes = []string{Embedded, Hidden, Metadata}

// Known reports whether mode is one of Modes.
// Every string is a legal input: the value comes off settings or another process, so an unknown one
// is the answer rather than a broken contract.
func Known(mode string) bool {
	for _, m := range Modes {
		if m == mode {
			return true
		}
	}
	return false
}
