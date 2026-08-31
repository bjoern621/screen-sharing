// Package framestamp carries the moment a frame left its encoder inside the encoded frame itself.
//
// A relay terminates one protocol and re-muxes for each listener, so nothing about a leg survives
// the hop and neither end can time the relay's own share.
// What does survive is the bitstream: a relay that re-muxes rather than re-encodes hands every
// listener the coded picture it was given, byte for byte.
// A wall clock written into the picture therefore reaches a viewer over any transport, and the
// subtraction against it is the whole path between the two machines as one measurement.
//
// The unit is the codec's own user-data unit and carries no picture data, so a decoder that does
// not know this app skips it: an H.264 or H.265 unregistered SEI message.
// A codec with no such unit is a codec this measures nothing on, which is stated rather than worked
// around.
//
// Two machines' clocks are what the subtraction is against, so a reading is worth what their
// agreement is worth. A viewer of its own stream reads one clock and measures exactly; two machines
// measure to whatever their time synchronisation holds them to, and a negative result is refused
// rather than shown as a path of no length (internal/receive, stampDelay).
package framestamp

import (
	"bytes"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// The media types whose bitstreams carry a user-data unit this writes into.
const (
	MediaH264 = "video/x-h264"
	MediaH265 = "video/x-h265"
)

// Carriage is how a stream is framed where a unit is put into it, off that pad's caps.
//
// Alignment decides whether anything is written at all. A unit prepended to a buffer holding a
// whole access unit sits ahead of that picture, which is where an SEI message belongs; prepended to
// a fragment it lands wherever the fragment does.
// LengthSize is read only under a length-prefixed format, and four is what H.264 and H.265 carry
// where caps state none.
type Carriage struct {
	Media      string
	Format     string
	Alignment  string
	LengthSize int
}

// marker identifies this app's stamp inside a user-data unit, as the 16 bytes the message's UUID
// field holds.
//
// No byte of it is zero, which is what lets a reader match it in a bitstream it has not parsed.
// Two zero bytes followed by a small value are what a coded stream escapes with an inserted 0x03,
// and a marker holding none is never rewritten on its way through an encoder, a muxer or a relay.
var marker = [16]byte{
	0x9A, 0x51, 0xD7, 0x2E, 0xC4, 0x63, 0xB8, 0x1F,
	0x7D, 0x36, 0xE9, 0x52, 0xA4, 0x8C, 0x71, 0xF3,
}

// Stamp is what one frame carries out of the publishing machine.
//
// The publishing figures are cumulative and are carried as a sum and a count, never as a rate: a
// viewer divides two of them over its own sampling interval, which is how every other counter in
// this app is read. Zero frames is a publish that measured none of its own stages.
//
// Milliseconds, where the clock is nanoseconds, because a stage of the path is read to the tenth of
// one and every byte here is paid on every frame.
type Stamp struct {
	At time.Time
	// PublishMs is what capture and encode have cost that pipeline in total, over PublishFrames
	// frames. It wraps at the width it is carried in, which a reader sees as one interval it cannot
	// divide.
	PublishMs     uint32
	PublishFrames uint32
	// LinkMs is the delivery window the publish leg settled on with the relay, and zero for a leg
	// stating none. No transport states a window of nothing, so zero is absent rather than a reading.
	LinkMs uint16
}

// The width of each field, in nibbles, in the order they are written.
//
// One byte per nibble is the price of the escape rule the marker is chosen against. A whole second
// lands on an instant whose low nanoseconds are all zero bits, and packed two nibbles to the byte
// that is five zero bytes in a row for an encoder to escape and a reader to unpick.
// Spread one to a byte, every byte carries a set high bit and nothing in the unit is ever rewritten.
const (
	clockNibbles         = 16
	publishMsNibbles     = 8
	publishFramesNibbles = 8
	linkNibbles          = 4
)

// stampBytes is what the reading itself takes, the marker aside.
const stampBytes = clockNibbles + publishMsNibbles + publishFramesNibbles + linkNibbles

// nibbleBase is the value a nibble is carried above, keeping every encoded byte in 0x10..0x1F.
const nibbleBase = 0x10

// payloadSize is the whole user-data payload: what identifies the stamp, then the stamp.
const payloadSize = len(marker) + stampBytes

// startCode leads a unit in a byte-stream, the four-byte form so a reader needs no state to find
// the unit's first byte.
var startCode = []byte{0x00, 0x00, 0x00, 0x01}

// unregisteredMessage is the SEI payload type an application writes its own bytes under, and the
// one an H.264 or H.265 decoder is required to skip when it does not know the UUID.
const unregisteredMessage = 0x05

// rbspTrailing closes a unit's bit string, a set bit followed by zeroes to the byte boundary.
//
// The one byte of a unit that may be zero-adjacent, and it is last, so nothing a reader matches on
// sits behind it.
const rbspTrailing = 0x80

// header is the codec's own unit header, everything ahead of the SEI message itself.
//
// H.264 spends one byte on it and H.265 two, the extra byte being the layer and temporal
// identifiers every H.265 unit carries.
// The types are the prefix SEI of each: 6 in H.264, 39 in H.265, which is 39<<1 in the byte the
// type shares with the layer id.
var headers = map[string][]byte{
	MediaH264: {0x06},
	MediaH265: {0x4E, 0x01},
}

// alignmentAccessUnit is the buffer alignment a stamp is written under: one buffer, one coded
// picture.
const alignmentAccessUnit = "au"

// lengthPrefixed are the stream formats framing each unit with its size rather than with a start
// code, spelled as the caps do.
// H.264 states one and H.265 two, the second pair differing from the first only in where the
// parameter sets sit, which is no business of a unit prepended to a picture.
var lengthPrefixed = map[string]bool{
	"avc":  true,
	"avc3": true,
	"hvc1": true,
	"hev1": true,
}

// defaultLengthSize is the size of a length prefix on caps that state none, which is what both
// codecs' parsers write.
const defaultLengthSize = 4

// Unit is the framed user-data unit carrying at, ready to be put in front of an access unit
// carried under c.
//
// false where nothing is written: a codec with no user-data unit, a stream framed in buffers
// smaller than a picture, and a length prefix too small to state this unit's size.
// None of those is a failure. A publish measures what its codec and its framing allow and reports
// the rest as unmeasured, an unstamped stream being a stream.
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
	message = append(message, rbspTrailing)

	unit, framed := frame(c, message)
	if !framed {
		return nil, false
	}
	// Read back rather than trusted: a row framing a unit its own reader cannot find publishes a
	// stream nobody can measure, and by the time a viewer says so the frames are on the wire.
	read, found := Read(unit)
	assert.Assert(found && read.At.Equal(s.At) && read.PublishMs == s.PublishMs &&
		read.PublishFrames == s.PublishFrames && read.LinkMs == s.LinkMs,
		"a written stamp reads back as what it carries", s, read)
	return unit, true
}

// frame puts the stream's own framing in front of a unit: a start code, or the unit's size in as
// many bytes as the caps declare.
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

// picture is whether a unit of this type carries part of a coded picture, per codec.
//
// H.264 numbers the picture types 1 to 5 in the low five bits of its header byte, and H.265 puts
// every one of them below 32 in the six bits above the header's low bit.
// Everything else in either is a parameter set, a delimiter or a message about the picture, which
// is what a stamp goes behind.
var pictureUnit = map[string]func(header []byte) bool{
	MediaH264: func(header []byte) bool {
		t := header[0] & 0x1F
		return t >= 1 && t <= 5
	},
	MediaH265: func(header []byte) bool {
		return header[0]>>1&0x3F < 32
	},
}

// Offset is where in frame a unit goes: behind the parameter sets and delimiters that open an
// access unit, and in front of the coded picture.
//
// The position the codecs give a prefix message, and the one that survives.
// A parser meeting a stream for the first time discards whatever stands ahead of the parameter set
// it needs to read the stream at all, so a stamp put in front of one is dropped at every point a
// listener starts reading.
//
// Zero where nothing can be read: a frame in a framing this cannot walk is one a guessed offset
// would cut a unit in half, and the front of it is at worst a stamp a parser drops.
func Offset(c Carriage, frame []byte) int {
	isPicture, carried := pictureUnit[c.Media]
	if !carried {
		return 0
	}
	if lengthPrefixed[c.Format] {
		return prefixedOffset(c, frame, isPicture)
	}
	return streamOffset(frame, isPicture)
}

// streamOffset walks a byte-stream frame by its start codes, three or four bytes of which open
// each unit.
func streamOffset(frame []byte, isPicture func([]byte) bool) int {
	for at := 0; at+3 < len(frame); {
		if frame[at] != 0x00 || frame[at+1] != 0x00 {
			return 0
		}
		// A four-byte start code is the three-byte one behind a leading zero, so the header is
		// whichever byte follows the last zero of the code.
		header := at + 3
		if frame[at+2] == 0x00 {
			header = at + 4
		}
		if header >= len(frame) {
			return 0
		}
		if isPicture(frame[header:]) {
			return at
		}

		next := bytes.Index(frame[header:], startCode[1:])
		if next < 0 {
			return 0
		}
		at = header + next
		// The search matches a code's last three bytes, so a four-byte one starts a byte earlier.
		if at > 0 && frame[at-1] == 0x00 {
			at--
		}
	}
	return 0
}

// prefixedOffset walks a frame whose units each state their own size.
func prefixedOffset(c Carriage, frame []byte, isPicture func([]byte) bool) int {
	size := c.LengthSize
	if size <= 0 {
		size = defaultLengthSize
	}

	for at := 0; at+size < len(frame); {
		length := 0
		for _, b := range frame[at : at+size] {
			length = length<<8 | int(b)
		}
		if length <= 0 || at+size+length > len(frame) {
			return 0
		}
		if isPicture(frame[at+size:]) {
			return at
		}
		at += size + length
	}
	return 0
}

// Read is what frame was stamped with, and false for a frame nothing stamped.
//
// The marker is searched for rather than the bitstream parsed. What a viewer holds is the framing
// its own decoder negotiated, which is neither necessarily the framing the stamp was written under
// nor something a reader needs to know: the marker cannot occur by escape and cannot be rewritten,
// so finding it is finding the stamp.
//
// All of it or none. A marker with a short or malformed reading behind it is refused rather than
// read as far as it goes, the bytes past a real stamp's end being picture data.
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
	if !read || !msRead || !framesRead || !linkRead {
		return Stamp{}, false
	}

	return Stamp{
		At:            time.Unix(0, int64(ns)),
		PublishMs:     uint32(publishMs),
		PublishFrames: uint32(publishFrames),
		LinkMs:        uint16(link),
	}, true
}
