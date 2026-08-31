package settings

import (
	"fmt"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// Output resolution is one setting for a pair of figures, carried as "1920x1080".
// The only place that spelling is read or written,
// so a form control, a refusal and an encoder name one string,
// and the two publish engines cannot read it differently.

// resolutionSeparator joins the two figures.
// Lowercase 'x', not the multiplication sign a label prints: this value is parsed.
const resolutionSeparator = "x"

// minOutputDimension is the smallest picture a scaler is asked for.
// Below it no encode is worth the round trip, and zero or negative is a figure no filter takes.
const minOutputDimension = 16

// Size is a picture size in pixels.
type Size struct {
	Width  int
	Height int
}

// String writes the spelling ParseSize reads: "1920x1080".
func (s Size) String() string {
	return strconv.Itoa(s.Width) + resolutionSeparator + strconv.Itoa(s.Height)
}

// FormatSize is String for two loose figures,
// which is what builds an option list from a monitor's dimensions.
func FormatSize(width, height int) string {
	return Size{Width: width, Height: height}.String()
}

// ParseSize reads the spelling String writes.
//
// Strict on the shape and on both figures, and refusing with an error rather than asserting:
// the value travels through a file the user owns and can edit,
// so a malformed one is an Umgebungsfehler on the way back in.
// Each refusal names which part failed, for whoever has to repair the value.
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
	// Every chroma subsampling here needs an even picture.
	// Refused rather than rounded: a size the run changed on its own is one no form can show back.
	if w%2 != 0 || h%2 != 0 {
		return Size{}, fmt.Errorf("output resolution %q is odd on one side, and every chroma subsampling here needs an even picture", value)
	}

	size := Size{Width: w, Height: h}
	assert.Assert(size.Width >= minOutputDimension && size.Height >= minOutputDimension,
		"a parsed size clears the floor on both sides", size.Width, size.Height)
	assert.Assert(size.Width%2 == 0 && size.Height%2 == 0,
		"a parsed size is even on both sides", size.Width, size.Height)
	return size, nil
}

// OutputSize is the picture the encoder is fed, and whether the capture is scaled to reach it.
//
// The empty setting is the capture's own size, neither an error nor a zero size.
// A caller that gets false adds no scaling stage,
// so the two answers are separate rather than a zero the caller has to test.
func (p Publish) OutputSize() (Size, bool, error) {
	if p.OutputResolution == "" {
		return Size{}, false, nil
	}

	size, err := ParseSize(p.OutputResolution)
	if err != nil {
		return Size{}, false, err
	}
	return size, true, nil
}
