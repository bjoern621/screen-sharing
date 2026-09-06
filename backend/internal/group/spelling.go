package group

import (
	"strings"
	"unicode/utf8"

	"bjoernblessin.de/go-utils/util/assert"
)

// A member goes by the name they claimed, and a relay path carries a narrow alphabet.
// Spelling is the one mapping between the two, and every path this app builds goes through it.
//
// The relay refuses a path outside its alphabet with 400 at the handshake,
// which takes the publish of anybody whose name holds a space, an umlaut or an emoji.
// A name is also brokered from Discord, where a person picks it and nothing holds it to an alphabet.
//
// The trip is exact both ways: NameOf(SpellName(name)) is name.
// A viewer's list therefore shows the name its owner claimed,
// and the path it asks the relay for is the one that name was published under.

// escape leads a spelled byte.
// A name's own underscore is spelled like every other byte outside the alphabet,
// so the character carries one meaning inside a path.
const escape = '_'

const hexDigits = "0123456789abcdef"

// SpellName is name in the characters a relay path carries.
//
// Two segments at most, the member's own name and the stream's own,
// cut at the last separator so a member whose name carries one keeps it inside their segment.
func SpellName(name string) string {
	spelled := spell(name)

	// The producing side fails where the trip does not close,
	// rather than the reader that met the path.
	read, ok := unspell(spelled)
	assert.Assert(ok && read == name, "a spelled name reads back as itself", name, spelled, read)
	assert.Assert(pathHolds(spelled), "a spelled name holds the relay's alphabet", spelled)
	return spelled
}

// NameOf is the name a spelled path segment stands for, and false for one this app did not spell.
//
// A stream published by something else lives under a name of its own choosing,
// which a reader shows as it stands rather than as the bytes a decode made of it.
func NameOf(spelled string) (string, bool) {
	name, ok := unspell(spelled)
	if !ok || spell(name) != spelled {
		return "", false
	}
	return name, true
}

// spell is SpellName with no contract, so the assertions can read their own output back.
func spell(name string) string {
	member, stream, found := cutLast(name, separator)
	if !found {
		return spellSegment(name)
	}
	return spellSegment(member) + separator + spellSegment(stream)
}

// spellSegment writes one path segment.
//
// A leading dot is spelled, so no segment reads as "." or ".." to a URL parser
// and a name cannot walk out of its group's prefix.
func spellSegment(segment string) string {
	var out strings.Builder
	for i := 0; i < len(segment); i++ {
		b := segment[i]
		if alphabetHolds(b) && !(b == '.' && i == 0) {
			out.WriteByte(b)
			continue
		}
		out.WriteByte(escape)
		out.WriteByte(hexDigits[b>>4])
		out.WriteByte(hexDigits[b&0x0f])
	}
	return out.String()
}

// unspell is NameOf with no canonical check, so SpellName can assert against it.
func unspell(spelled string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(spelled); i++ {
		if spelled[i] != escape {
			out.WriteByte(spelled[i])
			continue
		}
		if i+2 >= len(spelled) {
			return "", false
		}
		high, low := hexValue(spelled[i+1]), hexValue(spelled[i+2])
		if high < 0 || low < 0 {
			return "", false
		}
		out.WriteByte(byte(high<<4 | low))
		i += 2
	}

	name := out.String()
	if !utf8.ValidString(name) {
		return "", false
	}
	return name, true
}

// hexValue reads one lowercase hex digit, and -1 for anything else.
// Lowercase alone: a spelled byte has one form, and two spellings of one name
// are two paths a viewer's list cannot match against a publish.
func hexValue(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	}
	return -1
}

// alphabetHolds reports whether a byte stands in a path as it is.
//
// Alphanumerics, dot and minus,
// measured against the deployed relay rather than taken from MediaMTX's own rule:
// the relay refuses a tilde that rule allows.
// Underscore stays out of it, leading a spelled byte instead.
func alphabetHolds(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '.', b == '-':
		return true
	}
	return false
}

// pathHolds reports whether every byte of a path stands in the alphabet, separators included.
func pathHolds(path string) bool {
	for i := 0; i < len(path); i++ {
		if !alphabetHolds(path[i]) && path[i] != escape && path[i] != separator[0] {
			return false
		}
	}
	return true
}

// cutLast is strings.Cut at the last occurrence of sep.
func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}
