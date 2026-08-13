// Package cursor names what the pointer does in the captured frames.
//
// It is its own package for the reason gpupath's memory values are.
// The settings hold one of these, the capture backends decide which of them they serve,
// and the form offers the list, so a constant typed in any one of those places would be a second
// spelling of a value the others read.
// The package depends on nothing, which is what lets all three read it.
//
// What each backend serves is not here.
// That is a fact about a capture backend rather than about the pointer, and it lives beside the
// backends themselves (internal/publish/cursor.go).
package cursor

// The three things a pointer can do.
//
// They are three values rather than a flag and an exception, because the third is not more or less
// of the first two.
// An embedded pointer is drawn into the picture, costing bitrate and blurring with everything else
// the encoder spends bits on.
// A hidden one is absent.
// A metadata pointer travels beside the stream as a position, so a viewer that draws one keeps it
// sharp at any scale and a viewer that does not sees none at all.
const (
	// Embedded draws the pointer into the frames.
	Embedded = "embedded"
	// Hidden leaves it out.
	Hidden = "hidden"
	// Metadata sends its position beside the stream instead of drawing it.
	Metadata = "metadata"
)

// Modes lists every mode, in the order a form offers them: what a capture normally does,
// then its absence, then the one that moves the drawing to the viewer.
var Modes = []string{Embedded, Hidden, Metadata}

// Known reports whether a value names one of the modes.
//
// Every string is a legal input: the question this answers is whether a value that came from
// settings or from another process is one of the three, so an unknown one is the answer rather than
// a broken contract.
func Known(mode string) bool {
	for _, m := range Modes {
		if m == mode {
			return true
		}
	}
	return false
}
