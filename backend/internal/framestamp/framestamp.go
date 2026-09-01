// Package framestamp carries what a frame left the publishing machine with inside the encoded frame
// itself: the moment it left its encoder, and where the pointer was on it.
//
// A relay terminates one protocol and re-muxes per listener, so nothing about a leg survives
// the hop and neither end can time the relay's own share.
// The bitstream survives: a relay that re-muxes rather than re-encodes hands every listener
// the coded picture it was given, byte for byte.
// A wall clock written into the picture reaches a viewer over any transport,
// and the subtraction against it is the whole path between the two machines as one measurement.
//
// The pointer rides the same unit for the same reason, no leg carrying a channel beside the picture.
// It moves at the frame rate there rather than at the rate it is read,
// and it moves with the picture it was read over: a position in the frame it belongs to cannot lead
// the picture a viewer is drawing it on.
//
// The codec's own user-data unit carries no picture data, so a decoder that does not know this app
// skips it: an H.264 or H.265 unregistered SEI message.
// A codec with no such unit measures nothing, which is stated rather than worked around.
//
// The subtraction is against two machines' clocks, so a reading is worth what their agreement
// is worth.
// A viewer of its own stream reads one clock and measures exactly.
// Two machines measure to whatever their time synchronisation holds them to, and a negative result
// is refused rather than shown as a path of no length (internal/receive, stampDelay).
//
// How a codec frames the unit is carriage.go, and where in a picture it goes is offset.go.
package framestamp

import (
	"bytes"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// marker identifies this app's stamp inside a user-data unit,
// the 16 bytes of the message's UUID field.
//
// No byte is zero, so a reader matches it in a bitstream it has not parsed.
// A coded stream escapes two zero bytes followed by a small value with an inserted 0x03,
// so a marker holding none is never rewritten by an encoder, a muxer or a relay.
var marker = [16]byte{
	0x9A, 0x51, 0xD7, 0x2E, 0xC4, 0x63, 0xB8, 0x1F,
	0x7D, 0x36, 0xE9, 0x52, 0xA4, 0x8C, 0x71, 0xF3,
}

// Stamp is what one frame carries out of the publishing machine.
//
// The publishing figures are cumulative, carried as a sum and a count, never as a rate:
// a viewer divides over its own sampling interval, as every other counter in this app is read.
// Zero frames is a publish that measured none of its own stages.
//
// Milliseconds where the clock is nanoseconds, a stage of the path being read to the tenth of one
// and every byte here paid on every frame.
type Stamp struct {
	At time.Time
	// PublishMs is what capture and encode have cost that pipeline in total,
	// over PublishFrames frames.
	// Wraps at its carried width, which a reader sees as one interval it cannot divide.
	PublishMs     uint32
	PublishFrames uint32
	// LinkMs is the delivery window the publish leg settled on with the relay,
	// 0 for a leg stating none.
	// No transport states a window of nothing, so 0 is absent rather than a reading.
	LinkMs uint16
	// Pointer says whether this frame carries a position at all,
	// and whether the pointer was over the captured surface when it did.
	Pointer PointerState
	// PointerX and PointerY are where the pointer sat on this picture,
	// in parts of PointerWhole across it and down it.
	// A fraction rather than a pixel, so nothing that scales the picture on the way out or draws it
	// at another size on the way in has to be known at either end.
	// Read only under PointerHere.
	PointerX, PointerY uint16
}

// PointerState is what a frame says about the pointer.
//
// Three answers rather than two.
// A publish whose cursor mode draws the pointer into the frames or leaves it out sends no position,
// which is a different thing from a pointer that has left the captured screen,
// and both are different from a position to draw.
type PointerState uint8

const (
	PointerNone PointerState = 0
	PointerAway PointerState = 1
	PointerHere PointerState = 2
)

// PointerWhole is the far edge of the picture, PointerX and PointerY being parts of it.
const PointerWhole = 0xFFFF

// The width of each field, in nibbles, in the order they are written.
//
// One byte per nibble is the price of the escape rule the marker is chosen against.
// A whole second lands on an instant whose low nanoseconds are all zero bits,
// and packed two nibbles to the byte that is five zero bytes in a row for an encoder to escape.
// Spread one to a byte, every byte carries a set high bit and nothing in the unit is rewritten.
const (
	clockNibbles         = 16
	publishMsNibbles     = 8
	publishFramesNibbles = 8
	linkNibbles          = 4
	pointerStateNibbles  = 1
	pointerAxisNibbles   = 4
)

// stampBytes is what the reading itself takes, the marker aside.
const stampBytes = clockNibbles + publishMsNibbles + publishFramesNibbles + linkNibbles +
	pointerStateNibbles + 2*pointerAxisNibbles

// nibbleBase is the value a nibble is carried above, keeping every encoded byte in 0x10..0x1F.
const nibbleBase = 0x10

// payloadSize is the whole user-data payload: what identifies the stamp, then the stamp.
const payloadSize = len(marker) + stampBytes

// unregisteredMessage is the SEI payload type an application writes its own bytes under.
// An H.264 or H.265 decoder is required to skip it on a UUID it does not know.
const unregisteredMessage = 0x05

// rbspTrailing closes a unit's bit string, a set bit followed by zeroes to the byte boundary.
//
// The one byte of a unit that may be zero-adjacent, and it is last, so nothing a reader matches
// on sits behind it.
const rbspTrailing = 0x80

// Unit is the framed user-data unit carrying s, ready to be put in front of an access unit carried
// under c.
//
// false where nothing is written: a codec with no user-data unit, a stream framed in buffers
// smaller than a picture, a length prefix too small to state this unit's size.
// None of those is a failure.
// A publish measures what its codec and its framing allow and reports the rest as unmeasured,
// an unstamped stream being a stream.
func Unit(c Carriage, s Stamp) ([]byte, bool) {
	assert.Assert(!s.At.IsZero(), "a stamp carries an instant")

	header, carried := headers[c.Media]
	if !carried || c.Alignment != alignmentAccessUnit {
		return nil, false
	}

	message := make([]byte, 0, len(header)+2+payloadSize+1)
	message = append(message, header...)
	message = append(message, unregisteredMessage, byte(payloadSize))
	message = append(message, marker[:]...)
	message = appendField(message, uint64(s.At.UnixNano()), clockNibbles)
	message = appendField(message, uint64(s.PublishMs), publishMsNibbles)
	message = appendField(message, uint64(s.PublishFrames), publishFramesNibbles)
	message = appendField(message, uint64(s.LinkMs), linkNibbles)
	message = appendField(message, uint64(s.Pointer), pointerStateNibbles)
	message = appendField(message, uint64(s.PointerX), pointerAxisNibbles)
	message = appendField(message, uint64(s.PointerY), pointerAxisNibbles)
	message = append(message, rbspTrailing)

	unit, framed := frame(c, message)
	if !framed {
		return nil, false
	}
	// Read back rather than trusted: a row framing a unit its own reader cannot find publishes
	// a stream nobody can measure, and by the time a viewer says so the frames are on the wire.
	read, found := Read(unit)
	assert.Assert(found && read.At.Equal(s.At) && read.PublishMs == s.PublishMs &&
		read.PublishFrames == s.PublishFrames && read.LinkMs == s.LinkMs &&
		read.Pointer == s.Pointer && read.PointerX == s.PointerX && read.PointerY == s.PointerY,
		"a written stamp reads back as what it carries", s, read)
	return unit, true
}

// Read is what frame was stamped with, and false for a frame nothing stamped.
//
// The marker is searched for rather than the bitstream parsed.
// A viewer holds the framing its own decoder negotiated, not necessarily the one the stamp
// was written under, and it needs neither: the marker cannot occur by escape and cannot be
// rewritten, so finding it is finding the stamp.
//
// All of it or none.
// A marker with a short or malformed reading behind it is refused rather than read as far as it
// goes, the bytes past a real stamp's end being picture data.
func Read(frame []byte) (Stamp, bool) {
	at := bytes.Index(frame, marker[:])
	if at < 0 {
		return Stamp{}, false
	}
	stamp := frame[at+len(marker):]
	if len(stamp) < stampBytes {
		return Stamp{}, false
	}

	ns, read := readField(stamp, clockNibbles)
	stamp = stamp[clockNibbles:]
	publishMs, msRead := readField(stamp, publishMsNibbles)
	stamp = stamp[publishMsNibbles:]
	publishFrames, framesRead := readField(stamp, publishFramesNibbles)
	stamp = stamp[publishFramesNibbles:]
	link, linkRead := readField(stamp, linkNibbles)
	stamp = stamp[linkNibbles:]
	pointer, pointerRead := readField(stamp, pointerStateNibbles)
	stamp = stamp[pointerStateNibbles:]
	pointerX, xRead := readField(stamp, pointerAxisNibbles)
	stamp = stamp[pointerAxisNibbles:]
	pointerY, yRead := readField(stamp, pointerAxisNibbles)
	if !read || !msRead || !framesRead || !linkRead || !pointerRead || !xRead || !yRead {
		return Stamp{}, false
	}
	// A state no writer here spells is a marker that matched something this did not write,
	// so the whole reading goes rather than the one field.
	if pointer > uint64(PointerHere) {
		return Stamp{}, false
	}

	return Stamp{
		At:            time.Unix(0, int64(ns)),
		PublishMs:     uint32(publishMs),
		PublishFrames: uint32(publishFrames),
		LinkMs:        uint16(link),
		Pointer:       PointerState(pointer),
		PointerX:      uint16(pointerX),
		PointerY:      uint16(pointerY),
	}, true
}

// appendField writes value as one byte per nibble, most significant first.
func appendField(out []byte, value uint64, nibbles int) []byte {
	assert.Assert(nibbles > 0 && nibbles <= 16, "a field is between one and sixteen nibbles", nibbles)

	for i := range nibbles {
		shift := 4 * (nibbles - 1 - i)
		out = append(out, nibbleBase|byte(value>>shift&0xF))
	}
	return out
}

// readField is one field off the front of stamp, and false where any of its bytes is not a nibble
// this wrote.
func readField(stamp []byte, nibbles int) (uint64, bool) {
	value := uint64(0)
	for _, b := range stamp[:nibbles] {
		if b&0xF0 != nibbleBase {
			return 0, false
		}
		value = value<<4 | uint64(b&0xF)
	}
	return value, true
}
