package settings

import (
	"fmt"
	"strconv"
	"strings"
)

// The output resolution: the one setting that is a pair of numbers the user picks as a
// single thing.
//
// It travels as "WIDTHxHEIGHT" for the reason the domain model gives for every other
// option value: the settings' own spelling is what a form control offers and what a
// refusal names, so the value the encoder is given and the value the reader picked are
// the same string. This file is the only place that spelling is read or written, so the
// two engines cannot disagree about what it means.

// resolutionSeparator is what the two figures are joined by. Lowercase 'x' rather than
// the multiplication sign a label prints: the value is parsed, and a label is not.
const resolutionSeparator = "x"

// minOutputDimension is the smallest picture a scaler is asked for. Below it there is no
// encode worth the round trip, and a zero or negative figure is a value no filter takes.
const minOutputDimension = 16

// Size is a picture size in pixels.
type Size struct {
	Width  int
	Height int
}

// String renders a size as the settings carry it.
func (s Size) String() string {
	return strconv.Itoa(s.Width) + resolutionSeparator + strconv.Itoa(s.Height)
}

// FormatSize is String for a caller holding two figures rather than a Size, which is what
// builds an option list from a monitor's dimensions.
func FormatSize(width, height int) string {
	return Size{Width: width, Height: height}.String()
}

// ParseSize reads a size as the settings carry it.
//
// It is strict about the shape and about the numbers: every value that reaches it was
// written by this side, into a list for this field, so a malformed one is a caller that
// made one up rather than a user who typed badly. The error says which part failed, since
// the caller that made it up is the one reading the message.
func ParseSize(value string) (Size, error) {
	width, height, found := strings.Cut(value, resolutionSeparator)
	if !found {
		return Size{}, fmt.Errorf("output resolution %q is not WIDTH%sHEIGHT", value, resolutionSeparator)
	}

	w, err := strconv.Atoi(width)
	if err != nil {
		return Size{}, fmt.Errorf("output resolution %q has no width: %w", value, err)
	}
	h, err := strconv.Atoi(height)
	if err != nil {
		return Size{}, fmt.Errorf("output resolution %q has no height: %w", value, err)
	}

	if w < minOutputDimension || h < minOutputDimension {
		return Size{}, fmt.Errorf("output resolution %q is below the %d px floor either side", value, minOutputDimension)
	}
	// Every chroma subsampling this app encodes in needs an even picture, and an odd one
	// is a scaler failure rather than a picture. It is refused here rather than rounded,
	// because a size the run silently changed is a size no form can show back.
	if w%2 != 0 || h%2 != 0 {
		return Size{}, fmt.Errorf("output resolution %q is odd on one side, and every chroma subsampling here needs an even picture", value)
	}

	return Size{Width: w, Height: h}, nil
}

// OutputSize is the picture the encoder is fed, and whether the capture is scaled to
// reach it at all.
//
// The empty setting is not an error and not a zero size: it is the capture's own size,
// which is what every build did before the field existed. A caller that gets false back
// adds no scaling stage, which is why the two answers are separated rather than the
// caller being handed a zero to test.
func (s Stream) OutputSize() (Size, bool, error) {
	if s.OutputResolution == "" {
		return Size{}, false, nil
	}

	size, err := ParseSize(s.OutputResolution)
	if err != nil {
		return Size{}, false, err
	}
	return size, true, nil
}
