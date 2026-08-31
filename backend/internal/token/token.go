// Package token issues the short-lived credentials the relay checks a connection against,
// and publishes the key it checks them with.
//
// Every token is an ECDSA JWT on P-256, ES256 in JWS terms, minted per call and stored nowhere.
// Asking twice yields two tokens and leaves no state behind, so a lost answer is asked for again.
//
// The relay fetches the public key once through its JWKS setting and verifies locally,
// calling nothing per connection.
// This service is therefore out of a stream's path,
// and a connection is refused by arithmetic rather than by a round trip that can time out.
//
// A token is checked at the handshake and not again, so a live connection survives its own
// expiring and revocation lands at the next connection.
// Group rotation is what covers that (docs/plan.md, "Groups, auth and encryption").
//
// Written against crypto/ecdsa and encoding/json rather than a JWT library:
// ES256 is a base64 header, a base64 payload and a signature over the two,
// and a JWKS is five fields of JSON.
// A library would carry the other twenty algorithms, the ones whose presence is the vulnerability
// included.
package token

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Algorithm signs every token here, and is the only one this package produces.
//
// One algorithm and not a choice, the choice being the attack:
// a verifier reading the algorithm out of the token it verifies can be pointed at "none"
// or at a symmetric algorithm keyed with the public key.
// The relay is told this one in its configuration.
//
// SRT picks which: a token travels inside the SRT stream id, capped at 512 bytes everywhere.
// An RS256 signature is 342 characters on its own and does not fit.
// An ES256 one is 86 (MaxTokenBytes).
const Algorithm = "ES256"

// Curve as a JWKS names it, fixed by the algorithm rather than chosen: ES256 means P-256.
const Curve = "P-256"

// CoordinateBytes is the width each half of a P-256 signature and each public coordinate takes.
// Fixed width, not shortest form: JWS reads two numbers of exactly this length, so a leading zero
// byte stays.
const CoordinateBytes = 32

// MaxTokenBytes is what a token has to fit in, which SRT decides.
// The cap is 512 and SRT truncates rather than refusing,
// so an over-long token reaches the relay as a signature error instead of a length one here.
// Beside the token travel "publish:", a 27-character prefix, the stream name and two separators:
// 512 - 80.
// The name is bounded nowhere, so the 80 is a budget rather than a guarantee,
// held by the publish leg measuring the assembled id and refusing one that overflows
// (internal/transport, srtStreamIDFits).
const MaxTokenBytes = 432

// PermissionClaim is the claim the relay reads its permissions out of,
// MediaMTX's authJWTClaimKey default.
// Token and relay configuration have to agree,
// a claim the relay does not read being a token that grants nothing while looking valid.
const PermissionClaim = "mediamtx_permissions"

// Actions a permission may grant, as the relay names them.
// A stream takes two, one to push it and one to pull it.
// The third is the relay's own API,
// granted to no member (GroupPermissions) and reached by the index alone (APIPermissions).
const (
	ActionPublish = "publish"
	ActionRead    = "read"
	ActionAPI     = "api"
)

// Permission is one thing a token allows: an action, and the path it is allowed on.
// A path starting with a tilde is a regular expression, which is how the relay reads one,
// so a group's grant is a prefix match rather than an enumeration kept current.
type Permission struct {
	Action string `json:"action"`
	Path   string `json:"path"`
}

// GroupPermissions is what a holder of one prefix may do: publish and read every stream under it,
// and nothing outside it.
//
// One builder for a group's prefix and for the public one, the grant being the same shape:
// a public token drops the secret needed to ask for it, never a permission this states.
//
// The expression is anchored at the start of the prefix,
// an unanchored one granting every group whose id merely contains this one's.
func GroupPermissions(prefix string) []Permission {
	assert.Assert(prefix != "", "a group's permissions name the prefix they are granted on")

	path := "~^" + regexpQuote(prefix)
	out := []Permission{
		{Action: ActionPublish, Path: path},
		{Action: ActionRead, Path: path},
	}

	for _, p := range out {
		assert.Assert(strings.HasPrefix(p.Path, "~^"),
			"a granted path is anchored at the start of the prefix", p.Path)
	}
	return out
}

// APIPermissions is what reading the relay's own state takes, granted to nobody but the service
// that signs it, the index being what reads which streams exist.
// Holding the signing key is the whole credential, so no second one is configured and no member
// reaches the API (GroupPermissions).
//
// No path, the API not being one: a permission naming a path would match nothing.
func APIPermissions() []Permission {
	return []Permission{{Action: ActionAPI}}
}

// regexpQuote escapes what a regular expression would otherwise read as syntax.
// A group prefix is base32 and a slash, so nothing in one needs escaping.
// Escaped all the same, a prefix reaching the relay as a pattern matching more than itself being
// the one thing that must not happen.
func regexpQuote(s string) string {
	const special = `\.+*?()|[]{}^$`
	out := make([]byte, 0, len(s))
	for i := range len(s) {
		for j := range len(special) {
			if s[i] == special[j] {
				out = append(out, '\\')
				break
			}
		}
		out = append(out, s[i])
	}
	return string(out)
}

// Signer issues tokens and publishes the key they are verified with.
// Holds the key and nothing else: no token is kept, so two calls with one window are two tokens.
type Signer struct {
	key *ecdsa.PrivateKey
	// id names this key in the JWKS and in every token's header, so a relay holding two keys through
	// a rotation knows which one verifies a token.
	// Derived from the key rather than drawn, so one key always publishes under one name.
	id string
}

// NewSigner draws a signing key.
// A random source that cannot answer is an Umgebungsfehler and leaves as an error,
// a key drawn from something weaker signing tokens indistinguishable from real ones.
func NewSigner() (*Signer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("drawing a signing key: %w", err)
	}
	return SignerFor(key), nil
}

// SignerFor is a signer over a key from elsewhere, which is what a restart off a stored key hands
// it (cmd/groupd).
func SignerFor(key *ecdsa.PrivateKey) *Signer {
	assert.IsNotNil(key, "a signer carries the key it signs with")
	assert.Assert(key.Curve == elliptic.P256(),
		"a signer's key is on the curve its algorithm names", Curve)

	s := &Signer{key: key, id: keyID(&key.PublicKey)}
	assert.Assert(s.id != "", "a signer's key is named")
	return s
}

// KeyID is what this signer's key is called in the JWKS and in the tokens it signs.
func (s *Signer) KeyID() string { return s.id }

// Sign mints a token granting these permissions, valid from now for the window.
//
// The window is the caller's, being policy:
// short enough that a member who has left cannot open a new connection for long,
// long enough that a client is not asking for one between every reconnect (groupsvc.TokenWindow).
// A window of nothing is refused, that being a token expired before it arrives.
// The relay measures the window against its own clock, and two clocks agree to within seconds,
// which is the floor under a window a caller can pick.
func (s *Signer) Sign(subject string, permissions []Permission, now time.Time, window time.Duration) (string, error) {
	assert.IsNotNil(s.key, "a signing token is signed with a key")
	assert.Assert(window > 0, "a token is valid for some length of time", window.String())
	assert.Assert(len(permissions) > 0, "a token grants something", subject)

	// No "typ": optional, and a header byte is a byte the stream id does not have (MaxTokenBytes).
	// A verifier acts on the algorithm and the key id.
	header := map[string]string{"alg": Algorithm, "kid": s.id}
	claims := map[string]any{
		"sub":           subject,
		"iat":           now.Unix(),
		"exp":           now.Add(window).Unix(),
		PermissionClaim: permissions,
	}

	signing, err := segment(header)
	if err != nil {
		return "", err
	}
	payload, err := segment(claims)
	if err != nil {
		return "", err
	}
	signing += "." + payload

	digest := sha256.Sum256([]byte(signing))
	r, v, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing a token: %w", err)
	}

	// JWS signature: the two numbers back to back, each in the curve's width.
	// Not the ASN.1 form crypto/ecdsa also offers, which a verifier reads with the DER header as half
	// a coordinate.
	signature := append(coordinate(r), coordinate(v)...)

	token := signing + "." + raw.EncodeToString(signature)
	assert.Assert(strings.Count(token, ".") == 2, "a signed token is a header, a payload and a signature")
	assert.Assert(len(token) <= MaxTokenBytes, "a signed token fits the stream id that carries it", len(token))
	return token, nil
}

// coordinate writes one number of a signature or a public key,
// left-padded to the fixed width JWS reads.
func coordinate(n *big.Int) []byte {
	assert.IsNotNil(n, "a written coordinate is a number")

	out := make([]byte, CoordinateBytes)
	n.FillBytes(out)
	return out
}

// JWKS is the document the relay fetches to verify tokens with: this signer's public key and
// nothing else.
// One key per document, an overlapping rotation publishing two,
// and the key id in each token's header is what lets a verifier holding both pick the signer.
func (s *Signer) JWKS() []byte {
	assert.IsNotNil(s.key, "a published key set is built from a key")

	pub := s.key.PublicKey
	document := map[string]any{"keys": []map[string]string{{
		"kty": "EC",
		"crv": Curve,
		"alg": Algorithm,
		"use": "sig",
		"kid": s.id,
		"x":   raw.EncodeToString(coordinate(pub.X)),
		"y":   raw.EncodeToString(coordinate(pub.Y)),
	}}}
	out, err := json.Marshal(document)
	assert.IsNil(err, "a JWKS built from a key renders")
	return out
}

// raw is the base64 every part of a JWT and a JWKS is spelled in, URL-safe and unpadded,
// so a token travels as one string needing no escaping.
var raw = base64.RawURLEncoding

func segment(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("rendering a token: %w", err)
	}
	return raw.EncodeToString(out), nil
}

// keyID names a key by a digest of what it publishes, so one key is always called one thing and two
// keys are never called one.
func keyID(pub *ecdsa.PublicKey) string {
	assert.IsNotNil(pub, "a key is named after the key it is")

	digest := sha256.Sum256(append(coordinate(pub.X), coordinate(pub.Y)...))
	return raw.EncodeToString(digest[:8])
}

// PrivateKey is the key this signer signs with, for the caller that stores it so a restart keeps
// issuing tokens the relay's cached JWKS verifies.
//
// The one place the secret leaves this package.
// Whatever stores it owes the file a mode nothing else can read (cmd/groupd writes 0o600),
// a readable key being every group's streams.
func (s *Signer) PrivateKey() *ecdsa.PrivateKey { return s.key }
