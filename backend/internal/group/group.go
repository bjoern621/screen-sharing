// Package group derives a stream's place on the relay from the secret its viewers hold.
//
// A group is a path prefix, so the relay's own per-path permissions do the enforcing and "which
// streams may I see" is a string match rather than a query the relay API cannot answer.
//
// Possession of the key is membership, and there are no accounts behind it.
// The service draws a key, the client distributes it, and everyone holding it derives one prefix.
// Whatever hands the key on, a Discord bot serving a voice channel included, is a transport for it
// and never its source, so a second integration changes nothing about the security story.
// Deriving the key from a channel identifier was rejected: a channel id is a public snowflake,
// so prefixes would be enumerable.
//
// Both sides run this derivation, the client for the prefix it publishes under and the service for
// the prefix it grants a token on.
// Two implementations of one hash issue a member a token for a path nobody publishes to.
// Nothing here holds state or reaches anything.
//
// None of it is end to end.
// MediaMTX terminates every protocol and re-muxes per listener, so it sees plaintext by
// construction, and a relay that did not would cost HLS, WebRTC, the browser viewer and every relay
// statistic.
// Relay operator and key service can both watch a private stream, and the interface says so.
package group

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// KeyBytes is a group key's entropy.
//
// 32: the key is the whole of membership, with no account behind it, so guessing one is joining.
// Nobody types a key, it is copied or handed over by whatever distributes it.
const KeyBytes = 32

// IDChars is how much of the derived digest a prefix keeps.
//
// A deliberate truncation: the id is not a secret and appears in every URL a member pastes.
// 26 base32 characters is 130 bits, unguessable by accident and collision-free among one relay's
// groups by a margin nothing will close.
// Lengthening it moves every existing group's path, so it is stated once and read everywhere.
const IDChars = 26

// idEncoding spells an id in the characters a URL path takes unescaped,
// in one case so a member reading one aloud has no case to get wrong.
// Padding is stripped: a truncated digest needs none, and "=" in a path is one more escape.
var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// idLabel separates the id derivation from anything else the key is ever used for.
//
// Each use of a key derives under its own label, so the id, which every URL carries,
// cannot be replayed as another use's input.
// Adding a second use then changes nothing about what the first produces.
const idLabel = "screenshare/group-id/v1"

// PublicPrefix leads every stream nobody restricted the audience of.
//
// It has an id's shape and is not one: no key derives it, so a group's prefix can never collide
// with it and holding a key cannot publish into it.
// The relay still authenticates a publisher and a viewer here, and still carries them over an
// encrypted transport.
// What "public" drops is who may watch, and nothing else.
const PublicPrefix = "public" + separator

// separator divides a group's id from a stream's own name.
//
// A slash, because MediaMTX matches path permissions on prefixes and treats a slash as the segment
// boundary: a permission on "<id>/" grants that group's streams and nothing else,
// where a flat separator would let one group's id be a prefix of another's.
const separator = "/"

// Key is a group's secret.
// Possession is membership.
type Key []byte

// NewKey draws a fresh group key.
//
// Crypto randomness and never a seeded source: the key is membership, so a predictable one is a
// group anybody can join.
// A source that cannot answer is an Umgebungsfehler and leaves as an error rather than a fallback,
// since a key drawn from something weaker looks exactly like a real one.
func NewKey() (Key, error) {
	key := make([]byte, KeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("drawing a group key: %w", err)
	}

	assert.Assert(len(key) == KeyBytes, "a drawn key carries the whole of its entropy", len(key))
	return key, nil
}

// String is the key as it travels, standard base64: what a client stores and what whatever
// distributes it hands over.
//
// Deliberately the wire encoding rather than a rendering for a log line, the key being a secret,
// and the only way to the bytes.
func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k)
}

// ParseKey reads a key back off its encoding.
//
// A key of the wrong length is refused rather than padded or truncated: this app did not make it,
// and a prefix derived from it would put a stream where no member is looking.
// The encoding arrives from a settings file the user owns, so a malformed one is an Umgebungsfehler
// and leaves as an error.
func ParseKey(encoded string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("reading a group key: %w", err)
	}
	if len(raw) != KeyBytes {
		return nil, fmt.Errorf("a group key is %d bytes, this one is %d", KeyBytes, len(raw))
	}
	return raw, nil
}

// ID is the path prefix this key's streams live under.
//
// A keyed digest under a label, not a plain hash of the key: the id is public and appears in every
// URL, so it must say nothing about the secret behind it and nothing a second use would repeat.
//
// The length is asserted rather than tolerated.
// Every key here came from NewKey or ParseKey, both of which fix it,
// and an id derived from something shorter is a real prefix computed off a non-secret.
func (k Key) ID() string {
	assert.Assert(len(k) == KeyBytes, "an id is derived from a whole group key", len(k))

	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(idLabel))

	id := idEncoding.EncodeToString(mac.Sum(nil))[:IDChars]
	assert.Assert(len(id) == IDChars, "a derived id is the declared length", len(id))
	return id
}

// memberLabel separates a member's derivation from the id every URL carries.
//
// Each use of a key derives under its own label, so a member id cannot be replayed as a prefix nor
// a prefix as a member.
const memberLabel = "screenshare/group-member/v1"

// MemberIDChars is how much of a member's digest a token subject keeps.
//
// Shorter than an id: a subject tells one member of a group from the others in that group, where an
// id tells a group from every group on a relay.
// 16 base32 characters is 80 bits.
const MemberIDChars = 16

// MemberID names one member of this group, as a relay token's subject.
//
// The relay writes a subject into its log lines and its session listings, which is why this is a
// keyed digest under its own label and not the name: enforcement matches ids, and whatever named the
// member stays off the relay.
// Enforcement rests on both sides deriving it the same way, the service for the token it signs and
// the roster for the set it holds against, so the name is trimmed once here rather than at each
// caller.
//
// A name nobody gave is refused. It arrives over HTTP from whatever distributes the key, so an
// empty one is an Umgebungsfehler and leaves as an error.
func (k Key) MemberID(name string) (string, error) {
	assert.Assert(len(k) == KeyBytes, "a member id is derived from a whole group key", len(k))

	named := strings.TrimSpace(name)
	if named == "" {
		return "", errors.New("a member of a group has a name to derive an id from")
	}

	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(memberLabel))
	mac.Write([]byte(named))

	id := idEncoding.EncodeToString(mac.Sum(nil))[:MemberIDChars]
	assert.Assert(len(id) == MemberIDChars, "a derived member id is the declared length", len(id))
	return id, nil
}

// ErrNoGroup is what a Key operation answers for a key nobody gave.
//
// A group's path is derived from its key, so there is none to derive without one.
// Where a stream lives when no key was given is PublicPath, which is a different question and has
// no Key to ask it of.
var ErrNoGroup = errors.New("a group's path is derived from its key, and no group key was given")

// Path is where a stream of this group lives on the relay: id, slash, the stream's own name.
func (k Key) Path(name string) (string, error) {
	if len(k) == 0 {
		return "", ErrNoGroup
	}
	if err := checkStreamName(name); err != nil {
		return "", err
	}

	path := k.ID() + separator + name
	assert.Assert(strings.HasPrefix(path, k.Prefix()), "a stream's path starts with its group's prefix", path)
	return path, nil
}

// PublicPath is where a stream nobody restricted the audience of lives.
//
// The counterpart of Key.Path for a publisher holding no key, and a whole answer rather than a
// fallback: a stream published here is one anybody may watch, which is a choice the publisher made
// and not a group that went missing.
func PublicPath(name string) (string, error) {
	if err := checkStreamName(name); err != nil {
		return "", err
	}

	path := PublicPrefix + name
	assert.Assert(strings.HasPrefix(path, PublicPrefix), "a public stream's path starts with the public prefix", path)
	return path, nil
}

// checkStreamName holds a name against what one path segment may be.
//
// The name comes from a settings file the user owns, so a bad one is an Umgebungsfehler and leaves
// as an error.
// A name carrying a separator would publish into a prefix nobody granted, which is why it is
// refused rather than escaped.
func checkStreamName(name string) error {
	if name == "" {
		return errors.New("a stream has a name of its own inside its group")
	}
	if strings.Contains(name, separator) {
		return fmt.Errorf("a stream name is one path segment, and %q is more than one", name)
	}
	return nil
}

// Prefix leads every path of this group: what a relay permission is written against and what a
// listing matches on.
func (k Key) Prefix() string {
	return k.ID() + separator
}

// Split reads a relay path back into the group it belongs to and the stream's own name.
//
// The id and not the key: a path carries no secret.
// A reader of the relay's listing holds the prefix, and whether it is theirs is a comparison
// against their own key's id.
//
// A path with no separator belongs to no group.
// That is a stream published outside the group model, and reporting it as a group of its own would
// let a listing match on a stream name.
func Split(path string) (id, name string, ok bool) {
	id, name, ok = strings.Cut(path, separator)
	if !ok || id == "" || name == "" {
		return "", "", false
	}
	return id, name, true
}

// PrefixOf is the prefix the group of this relay path is enforced under, and false for a path
// belonging to no group.
//
// The path and not the key: what names a group here is the relay's own word, a connection's path,
// which carries the id and never the secret behind it.
func PrefixOf(path string) (string, bool) {
	id, _, ok := Split(path)
	if !ok {
		return "", false
	}
	return id + separator, true
}

// Holds reports whether a relay path is inside this key's group.
func (k Key) Holds(path string) bool {
	id, _, ok := Split(path)
	return ok && id == k.ID()
}
