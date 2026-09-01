package framestamp

import "bytes"

// Where in an access unit a stamp goes.
//
// Behind the parameter sets and delimiters that open it, in front of the coded picture.
// A parser meeting a stream for the first time discards whatever stands ahead of the parameter set
// it needs to read the stream at all, so a stamp in front of one is dropped wherever a listener
// starts reading.

// pictureUnit reports whether a unit of this type carries part of a coded picture, per codec.
//
// H.264 numbers the picture types 1..5 in the low five bits of its header byte,
// H.265 puts every one of them below 32 in the six bits above the header's low bit.
// Everything else is a parameter set, a delimiter or a message about the picture,
// which is what a stamp goes behind.
var pictureUnit = map[string]func(header []byte) bool{
	MediaH264: func(header []byte) bool {
		t := header[0] & 0x1F
		return t >= 1 && t <= 5
	},
	MediaH265: func(header []byte) bool {
		return header[0]>>1&0x3F < 32
	},
}

// Offset is where in frame a unit goes.
//
// 0 where nothing can be read: a guessed offset into a framing this cannot walk would cut a unit
// in half, and the front is at worst a stamp a parser drops.
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
		// A four-byte start code is the three-byte one behind a leading zero,
		// so the header is whichever byte follows the last zero of the code.
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
