// Package group derives where a stream lives on the relay from the secret its viewers hold.
//
// A group is a path prefix.
// The relay's own per-path permissions then do the enforcing, and "which streams may I see" is a
// string match rather than a query the relay API cannot answer.
//
// Possession of the key is membership.
// There are no accounts: the service creates a group and hands back the secret,
// the client distributes it, and anyone holding it derives the same prefix.
// A Discord bot handing the key to whoever is in a voice channel is a transport for it and never
// its source, which is what keeps the security story unchanged when a second integration arrives.
// Deriving the key from a channel identifier was rejected because a channel id is a public
// snowflake, so anyone could enumerate channels and compute prefixes.
//
// What this package is for is that the derivation runs on both sides.
// The client computes the prefix it publishes under and the service computes the prefix it grants a
// token for, and two implementations of one hash is the failure where a member is issued a token
// for a path nobody is publishing to.
// It holds no state, reaches nothing, and is what both link.
//
// None of it is end to end.
// MediaMTX terminates every protocol and re-muxes for every listener, so it sees plaintext by
// construction, and a relay that did not would take HLS, WebRTC, the browser viewer and every relay
// statistic with it.
// The relay operator and the key service can both watch a private stream, and the interface says so
// rather than implying otherwise.
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

// KeyBytes is how much entropy a group key carries.
//
// 32 bytes because the key is the whole of membership: there are no accounts behind it,
// so guessing one is joining, and it is never typed by a person, it is copied,
// or handed over by whatever distributes it.
const KeyBytes = 32

// IDChars is how much of the derived digest the prefix keeps.
//
// It is a truncation and it is deliberate: the prefix appears in every URL a member pastes,
// and the id is not a secret.
// What it has to be is unguessable-by-accident and collision-free among the groups one relay holds,
// which 26 base32 characters, 130 bits, is by a margin nothing will close.
// Lengthening it later changes every existing group's path, so it is stated once here and read
// everywhere.
const IDChars = 26

// idEncoding spells an id in the characters a URL path takes without escaping,
// in one case so a member reading one aloud has no case to get wrong.
// The padding is stripped because a truncated digest needs none and "=" in a path is one more thing
// to escape.
var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// idLabel separates the group's own derivation from anything else the key is ever used for.
//
// A key with more than one use derives each of them under its own label, so a value one use
// publishes, the id, which appears in every URL, cannot be replayed as the input of another.
// There is one use today, and the label is what keeps adding a second from being a change to what
// the first produces.
const idLabel = "screenshare/group-id/v1"

// separator divides a group's id from the stream's own name in a path.
//
// A slash, because MediaMTX's path permissions match on path prefixes and a slash is what it treats
// as a segment boundary: a permission on "<id>/" grants that group's streams and nothing else,
// where a flat separator would let one group's id be a prefix of another's.
const separator = "/"

// Key is a group's secret.
// Possession of it is membership.
type Key []byte

// NewKey draws a fresh group key.
//
// The randomness is the crypto source and never a seeded one: the key is membership,
// so a predictable one is a group anybody can join.
// A source that cannot answer is an Umgebungsfehler and leaves as an error, never a fallback:
// a key drawn from something weaker would look exactly like a real one.
func NewKey() (Key, error) {
	key := make([]byte, KeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("drawing a group key: %w", err)
	}

	assert.Assert(len(key) == KeyBytes, "a drawn key carries the whole of its entropy", len(key))
	return key, nil
}

// String is the key as it travels: standard base64, which is what a client stores and what whatever
// distributes it hands over.
//
// The key is a secret, so this is deliberately not what a String method usually is,
// a value's rendering for a log line.
// It is the encoding, named so, and the only way to get the bytes out.
func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k)
}

// ParseKey reads a key back off its encoding.
//
// A key of the wrong length is refused rather than padded or truncated: it is not a key this app
// produced, and deriving a prefix from it anyway would put a stream somewhere no member is looking.
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
// It is a keyed digest rather than a plain hash of the key, so the id and the key are not two
// encodings of one value: the id is public, appears in every URL, and must say nothing about the
// secret it came from.
// The label is what keeps a second use of the key from producing the same bytes.
//
// The key's length is asserted rather than tolerated.
// Every key reaching here came from NewKey or ParseKey, both of which fix the length,
// and an id derived from something shorter would be a real prefix computed from a value that is not
// a membership secret.
func (k Key) ID() string {
	assert.Assert(len(k) == KeyBytes, "an id is derived from a whole group key", len(k))

	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(idLabel))

	id := idEncoding.EncodeToString(mac.Sum(nil))[:IDChars]
	assert.Assert(len(id) == IDChars, "a derived id is the declared length", len(id))
	return id
}

// ErrNoGroup is what a path operation answers when no key was given.
//
// Publishing always requires a group, so a stream with no key is not a stream in some default
// place, it is a stream nobody has said who may watch.
// Answering with the bare name would be exactly that, published where every other group can see it.
var ErrNoGroup = errors.New("a stream is published under a group, and no group key was given")

// Path is where a stream of this group lives on the relay: the group's id, a slash,
// and the stream's own name.
func (k Key) Path(name string) (string, error) {
	if len(k) == 0 {
		return "", ErrNoGroup
	}
	if name == "" {
		return "", errors.New("a stream has a name of its own inside its group")
	}
	if strings.Contains(name, separator) {
		return "", fmt.Errorf("a stream name is one path segment, and %q is more than one", name)
	}

	path := k.ID() + separator + name
	assert.Assert(strings.HasPrefix(path, k.Prefix()), "a stream's path starts with its group's prefix", path)
	return path, nil
}

// Prefix is what every path of this group starts with, which is what a relay permission is written
// against and what a listing matches on.
func (k Key) Prefix() string {
	return k.ID() + separator
}

// Split reads a relay path back into the group it belongs to and the stream's own name.
//
// It answers the id rather than the key, because a path carries no secret:
// what a reader of the relay's own listing has is the prefix, and whether that prefix is theirs is
// a comparison against their own key's id.
//
// A path with no separator belongs to no group.
// That is what a stream published before the group model, or by something else entirely,
// looks like, and reporting it as a group of its own would let a listing match on a name.
func Split(path string) (id, name string, ok bool) {
	id, name, ok = strings.Cut(path, separator)
	if !ok || id == "" || name == "" {
		return "", "", false
	}
	return id, name, true
}

// Holds reports whether a relay path belongs to this key's group.
func (k Key) Holds(path string) bool {
	id, _, ok := Split(path)
	return ok && id == k.ID()
}
