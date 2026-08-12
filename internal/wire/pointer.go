package wire

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/pointer"
)

// PointerPosition carries where the pointer is onto the contract.
//
// The coordinates are already the picture's own by the time they reach here: the app turns
// what the child read into the pixels the stream carries, because that is the one place
// holding both the reading and the screen it was read on (app/pointer.go). Nothing is
// converted here, which is what keeps this a conversion and not a second opinion.
func PointerPosition(p pointer.Position) *screensharev1.PointerPosition {
	return &screensharev1.PointerPosition{
		X:                   int32(p.X),
		Y:                   int32(p.Y),
		CapturedAtUnixNanos: p.At.UnixNano(),
		Visible:             p.Visible,
	}
}
