package framestamp

import "bjoernblessin.de/go-utils/util/assert"

// How a codec carries a user-data unit, and what it takes to put one in front of a picture.
//
// The stamp itself is the same bytes on every codec (framestamp.go).
// What differs is the header a unit opens with and whether the stream separates units by a start
// code or by their own size, which is what this file answers.

// The media types whose bitstreams carry a user-data unit this writes into.
const (
	MediaH264 = "video/x-h264"
	MediaH265 = "video/x-h265"
)

// Carriage is how a stream is framed where a unit is put into it, off that pad's caps.
//
// Alignment decides whether anything is written at all.
// A unit prepended to a whole access unit sits ahead of that picture, where an SEI message belongs.
// Prepended to a fragment it lands wherever the fragment does.
// LengthSize is read only under a length-prefixed format,
// and 4 is what H.264 and H.265 carry where caps state none.
type Carriage struct {
	Media      string
	Format     string
	Alignment  string
	LengthSize int
}

// headers is the codec's own unit header, everything ahead of the SEI message itself.
//
// H.264 spends one byte, H.265 two, the extra byte being the layer and temporal identifiers every
// H.265 unit carries.
// The types are the prefix SEI of each: 6 in H.264, 39 in H.265, carried as 39<<1 in the byte
// the type shares with the layer id.
var headers = map[string][]byte{
	MediaH264: {0x06},
	MediaH265: {0x4E, 0x01},
}

// alignmentAccessUnit is the buffer alignment a stamp is written under:
// one buffer, one coded picture.
const alignmentAccessUnit = "au"

// startCode leads a unit in a byte-stream, the four-byte form so a reader needs no state to find
// the unit's first byte.
var startCode = []byte{0x00, 0x00, 0x00, 0x01}

// lengthPrefixed are the stream formats framing each unit with its size rather than a start code,
// spelled as the caps do.
// H.264 states one and H.265 two, the pairs differing only in where the parameter sets sit,
// which is no business of a unit prepended to a picture.
var lengthPrefixed = map[string]bool{
	"avc":  true,
	"avc3": true,
	"hvc1": true,
	"hev1": true,
}

// defaultLengthSize is the length prefix size on caps that state none,
// what both codecs' parsers write.
const defaultLengthSize = 4

// frame puts the stream's own framing in front of a unit: a start code, or the unit's size
// in as many bytes as the caps declare.
func frame(c Carriage, message []byte) ([]byte, bool) {
	assert.Assert(len(message) > 0, "a framed unit has a body")

	if !lengthPrefixed[c.Format] {
		return append(append([]byte{}, startCode...), message...), true
	}

	size := c.LengthSize
	if size <= 0 {
		size = defaultLengthSize
	}
	// A prefix too narrow to state this unit's size would state a shorter one, and the bytes past it
	// would be read as another unit.
	if size < 4 && len(message) >= 1<<(8*size) {
		return nil, false
	}

	out := make([]byte, size, size+len(message))
	for i := range size {
		out[size-1-i] = byte(len(message) >> (8 * i))
	}
	return append(out, message...), true
}
