package wire

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/pointer"
)

// PointerPosition carries where the pointer is onto the contract.
//
// The coordinates cross as the fraction of the picture they were read as, so this stays
// a conversion rather than a second opinion.
// Which picture they are a fraction of is the publish child's answer, being the one place holding
// both the reading and what was captured at what size (gstrun/pointersource.go).
func PointerPosition(p pointer.Spot) *screensharev1.PointerPosition {
	return &screensharev1.PointerPosition{
		X:                   float32(p.X),
		Y:                   float32(p.Y),
		CapturedAtUnixNanos: p.At.UnixNano(),
		Visible:             p.Visible,
	}
}
