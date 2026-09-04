// Package group derives a stream's place on the relay from the secret its viewers hold.
//
// A group is a path prefix, so the relay's own per-path permissions do the enforcing
// and "which streams may I see" is a string match rather than a query the relay API cannot answer.
//
// Holding the group key is what lets somebody join, and no accounts stand behind it.
// The service draws a key, the client distributes it, and everyone holding it derives one prefix.
// Whatever hands the key on, a Discord bot serving a voice channel included, is a transport for it
// and never its source, so a second integration changes nothing about the security story.
// A key derived from a channel identifier would leave prefixes enumerable,
// a channel id being a public snowflake.
//
// Who somebody is inside a group is a second derivation,
// from a secret the joining app draws for itself and hands to nobody (MemberSecret).
// Both derive under the group key and under labels of their own,
// so a member id says nothing about the group's secret and a prefix cannot be replayed as a member.
//
// Both sides run this derivation, the client for the prefix it publishes under
// and the service for the prefix it grants a token on.
// Two implementations of one hash issue a member a token for a path nobody publishes to.
// Nothing here holds state or reaches anything.
//
// None of it is end to end.
// MediaMTX terminates every protocol and re-muxes per listener, so it sees plaintext,
// and a relay that did not would cost HLS, WebRTC, the browser viewer and every relay statistic.
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
// 32: the key is the whole of the invite, with no account behind it, so guessing one is joining.
// Nobody types a key, whatever distributes it handing it over instead.
const KeyBytes = 32

// IDChars is how much of the derived digest a prefix keeps.
//
// Truncated because the id is no secret and appears in every URL a member pastes.
// 26 base32 characters is 130 bits, unguessable by accident
// and collision-free among one relay's groups by a wide margin.
// Lengthening it moves every existing group's path, so it is stated once and read everywhere.
const IDChars = 26

// idEncoding spells an id in the characters a URL path takes unescaped,
// in one case so a member reading one aloud has no case to get wrong.
// Padding is stripped: a truncated digest needs none, and "=" in a path is one more escape.
var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// idLabel separates the id derivation from every other use of the key.
//
// Each use derives under its own label, so the id, which every URL carries,
// cannot be replayed as another use's input,
// and a second use changes nothing about what the first produces.
const idLabel = "screenshare/group-id/v1"

// separator divides a group's id from a stream's own name.
//
// A slash, because MediaMTX matches path permissions on prefixes
// and treats a slash as the segment boundary:
// a permission on "<id>/" grants that group's streams and nothing else,
// where a flat separator would let one group's id be a prefix of another's.
const separator = "/"

// Key is a group's secret.
// Holding one is what lets somebody join.
type Key []byte

// NewKey draws a fresh group key.
//
// Crypto randomness and never a seeded source:
// the key is the invite, so a predictable one is a group anybody can join.
// A source that cannot answer is an Umgebungsfehler and leaves as an error rather than a fallback,
// a key drawn from something weaker looking exactly like a real one.
func NewKey() (Key, error) {
	key := make([]byte, KeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("drawing a group key: %w", err)
	}

	assert.Assert(len(key) == KeyBytes, "a drawn key carries the whole of its entropy", len(key))
	return key, nil
}

// String is the key as it travels, standard base64:
// what a client stores and what whatever distributes it hands over.
//
// The wire encoding rather than a rendering for a log line, the key being a secret,
// and the only way to the bytes.
func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k)
}

// ParseKey reads a key back off its encoding.
//
// A key of the wrong length is refused rather than padded or truncated:
// this app did not make it, and a prefix derived from it puts a stream where no member is looking.
// The encoding arrives from a settings file the user owns,
// so a malformed one is an Umgebungsfehler and leaves as an error.
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
// A keyed digest under a label rather than a plain hash of the key:
// the id is public and appears in every URL,
// so it says nothing about the secret behind it and nothing a second use would repeat.
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
// Each use of a key derives under its own label,
// so a member id cannot be replayed as a prefix nor a prefix as a member.
const memberLabel = "screenshare/group-member-secret/v1"

// MemberIDChars is how much of a member's digest a token subject keeps.
//
// Shorter than an id: a subject tells one member of a group from the others in that group,
// where an id tells a group from every group on a relay.
// 16 base32 characters is 80 bits.
const MemberIDChars = 16

// MemberSecretBytes is a member secret's entropy.
//
// 32, as a group key: guessing one is speaking for that member,
// presence being stated over the id the secret derives.
const MemberSecretBytes = 32

// MemberSecret is what an app knows itself by inside one group.
//
// Drawn by the app that holds it and issued by nobody,
// so no other member and no service can state this member's presence or take the name it claimed.
type MemberSecret []byte

// NewMemberSecret draws the secret an app joins one group under.
//
// Crypto randomness and never a seeded source,
// a predictable secret being an identity anybody can take over.
// A source that cannot answer is an Umgebungsfehler and leaves as an error.
func NewMemberSecret() (MemberSecret, error) {
	secret := make([]byte, MemberSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("drawing a member secret: %w", err)
	}

	assert.Assert(len(secret) == MemberSecretBytes, "a drawn secret carries the whole of its entropy", len(secret))
	return secret, nil
}

// String is the secret as it travels and as it is stored, standard base64.
func (s MemberSecret) String() string {
	return base64.StdEncoding.EncodeToString(s)
}

// ParseMemberSecret reads a secret back off its encoding.
//
// A secret of the wrong length is refused rather than padded:
// it arrives from an identity file the user owns or over HTTP,
// so a malformed one is an Umgebungsfehler and leaves as an error.
func ParseMemberSecret(encoded string) (MemberSecret, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("reading a member secret: %w", err)
	}
	if len(raw) != MemberSecretBytes {
		return nil, fmt.Errorf("a member secret is %d bytes, this one is %d", MemberSecretBytes, len(raw))
	}
	return raw, nil
}

// MemberID names one member of this group,
// as a relay token's subject and as what membership is stated over.
//
// Keyed under the group's key, so one secret is one member per group
// and a relay listing of one group names nothing about the same app in another.
// The relay writes a subject into its log lines and its session listings,
// so what travels is the digest and never the secret behind it.
//
// The lengths are asserted rather than tolerated.
// Every secret here came from NewMemberSecret or ParseMemberSecret, both of which fix it,
// and an id derived from something shorter is a subject membership can neither match nor explain.
func (k Key) MemberID(secret MemberSecret) string {
	assert.Assert(len(k) == KeyBytes, "a member id is derived from a whole group key", len(k))
	assert.Assert(len(secret) == MemberSecretBytes, "a member id is derived from a whole member secret", len(secret))

	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(memberLabel))
	mac.Write(secret)

	id := idEncoding.EncodeToString(mac.Sum(nil))[:MemberIDChars]
	assert.Assert(len(id) == MemberIDChars, "a derived member id is the declared length", len(id))
	return id
}

// srtLabel separates the SRT passphrase from every other use of the key.
const srtLabel = "screenshare/srt-passphrase/v1"

// srtEncoding spells a passphrase in characters no URL query and no pipeline description escapes.
var srtEncoding = base64.RawURLEncoding

// SrtPassphrase keys this group's SRT legs, both directions.
//
// SRT is UDP with no TLS, so a passphrase is the whole of what encrypts it,
// and deriving it from the group key makes the audience of the packets the group and nothing wider.
// A keyed digest under its own label, as ID is and for the same reason:
// the value reaches the relay's configuration and every handshake,
// and must say nothing about the key behind it.
//
// The whole digest, 43 characters: libsrt takes 10 to 79,
// and the relay refuses a value outside that on both the publish and the read key.
func (k Key) SrtPassphrase() string {
	assert.Assert(len(k) == KeyBytes, "a passphrase is derived from a whole group key", len(k))

	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(srtLabel))

	passphrase := srtEncoding.EncodeToString(mac.Sum(nil))
	assert.Assert(len(passphrase) >= 10 && len(passphrase) <= 79,
		"a derived passphrase is inside libsrt's bounds", len(passphrase))
	return passphrase
}

// ErrNoGroup is what a Key operation answers for a key nobody gave.
//
// A group's path is derived from its key, so there is none to derive without one.
// Where a stream lives with no key given is PublicPath,
// a different question with no Key to ask it of.
var ErrNoGroup = errors.New("a group's path is derived from its key, and no group key was given")

// Path is where a stream of this group lives on the relay: id, slash, name,
// name being the stream's own or the publishing member's own ahead of it.
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

// checkStreamName holds a name against what may follow a group's own prefix:
// the stream's own name alone, or the publishing member's own name and the stream's own together.
//
// The name is computed by this app rather than typed by its user,
// so a bad one is an Entwicklungsfehler everywhere but the boundary a stored file crosses,
// where it is carried as an error like every other value that file could hold.
// Two segments stay inside the permission a group's prefix grants, covering everything under it
// however deep, so a third buys no wider a claim and is refused along with an empty one at either end.
func checkStreamName(name string) error {
	if name == "" || strings.HasPrefix(name, separator) || strings.HasSuffix(name, separator) {
		return errors.New("a stream has a name of its own inside its group")
	}
	if strings.Count(name, separator) > 1 {
		return fmt.Errorf("a stream name is one or two path segments, and %q is more", name)
	}
	return nil
}

// NameHolds reports whether name is one a stream may carry inside its group.
//
// The index reads it to decide which relay paths are streams of a group and which are nested deeper
// than a name reaches (internal/groupsvc), off the one rule a publish is checked against.
func NameHolds(name string) bool {
	return checkStreamName(name) == nil
}

// Prefix leads every path of this group:
// what a relay permission is written against and what a listing matches on.
func (k Key) Prefix() string {
	return k.ID() + separator
}

// Split reads a relay path back into the group it belongs to and the stream's own name.
//
// The id rather than the key: a path carries no secret.
// A reader of the relay's listing holds the prefix,
// and whether it is theirs is a comparison against their own key's id.
//
// A path with no separator belongs to no group, being a stream published outside the group model,
// and reporting it as a group of its own would let a listing match on a stream name.
func Split(path string) (id, name string, ok bool) {
	id, name, ok = strings.Cut(path, separator)
	if !ok || id == "" || name == "" {
		return "", "", false
	}
	return id, name, true
}

// PrefixOf is the prefix the group of this relay path is enforced under,
// and false for a path belonging to no group.
//
// The path rather than the key: what names a group here is the relay's own word,
// a connection's path, which carries the id and never the secret behind it.
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
