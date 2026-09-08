package share

import (
	"fmt"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// Rect is a rectangle of the virtual desktop, in the monitor enumeration's own pixels.
// The origin is the primary output's top-left corner,
// so a screen placed left of or above it carries negative X or Y.
type Rect struct {
	X, Y, Width, Height int
}

// The settings spell a rectangle as "100,200,1280x720":
// the top-left corner, then the size in the same shape output_resolution carries.
const (
	rectSeparator = ","
	sizeSeparator = "x"
)

// ParseRect reads that spelling, and answers false for anything a capture cannot be cropped to.
//
// Refused is every malformed string and every size at or below zero.
// A width of nought is not an empty capture: it is the value both crop interfaces read
// as "the whole picture", so a rectangle nobody drew would silently become the whole screen.
//
// A negative corner is taken, that being where a screen left of the primary one starts.
func ParseRect(in string) (Rect, bool) {
	corner, size, ok := strings.Cut(in, rectSeparator)
	if !ok {
		return Rect{}, false
	}
	y, size, ok := strings.Cut(size, rectSeparator)
	if !ok {
		return Rect{}, false
	}
	width, height, ok := strings.Cut(size, sizeSeparator)
	if !ok {
		return Rect{}, false
	}

	r := Rect{}
	var err error
	if r.X, err = strconv.Atoi(corner); err != nil {
		return Rect{}, false
	}
	if r.Y, err = strconv.Atoi(y); err != nil {
		return Rect{}, false
	}
	if r.Width, err = strconv.Atoi(width); err != nil || r.Width <= 0 {
		return Rect{}, false
	}
	if r.Height, err = strconv.Atoi(height); err != nil || r.Height <= 0 {
		return Rect{}, false
	}
	return r, true
}

// String is the spelling ParseRect reads, so a rectangle survives the settings it is stored in.
func (r Rect) String() string {
	assert.Assert(r.Width > 0 && r.Height > 0,
		"a rectangle is spelled out at a size a capture can be cropped to", r.Width, r.Height)

	return fmt.Sprintf("%d%s%d%s%d%s%d",
		r.X, rectSeparator, r.Y, rectSeparator, r.Width, sizeSeparator, r.Height)
}

// Within reports whether the rectangle lies inside the output at the given origin and size,
// which is what lets a backend cropping one output be handed monitor-local coordinates.
func (r Rect) Within(offsetX, offsetY, width, height int) bool {
	assert.Assert(width >= 0 && height >= 0,
		"an output a rectangle is held against has a size", width, height)

	return r.X >= offsetX && r.Y >= offsetY &&
		r.X+r.Width <= offsetX+width && r.Y+r.Height <= offsetY+height
}

// Offset moves the rectangle by an output's origin,
// turning desktop coordinates into that output's own.
func (r Rect) Offset(x, y int) Rect {
	return Rect{X: r.X - x, Y: r.Y - y, Width: r.Width, Height: r.Height}
}
