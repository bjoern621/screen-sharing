// Package token issues the short-lived credentials the relay checks a connection against,
// and publishes the key it checks them with.
//
// The relay validates a JWT through its JWKS setting, which is what keeps it from calling anything
// per connection: it fetches the public key once and verifies every token against it locally.
// So the service that hands out tokens is not in the path of a stream, and a connection is refused
// by arithmetic rather than by a round trip that can time out.
//
// Tokens are short and validated at connect.
// A live connection survives its token expiring, since the relay checks at the handshake and not
// again, so revocation lands at the next connection, which is what group rotation is for
// (docs/plan.md, "Groups, auth and encryption").
//
// It is written against crypto/rsa and encoding/json rather than a JWT library,
// because what is needed is one algorithm, one claim set and one key: RS256 is a base64 header,
// a base64 payload and a PKCS#1 v1.5 signature over the two, and a JWKS is four fields of JSON.
// A dependency would carry the other twenty algorithms, including the ones whose presence is the
// vulnerability.
package token

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Algorithm is what every token this package signs is signed with, and the only one it will
// produce.
//
// One algorithm rather than a choice, because the choice is the attack: a verifier that reads the
// algorithm out of the token it is verifying is one an attacker can point at "none" or at a
// symmetric algorithm keyed with the public key.
// The relay is told this one in its configuration, and this package produces nothing else.
const Algorithm = "RS256"

// KeyBits is the size of the signing key.
// 2048 because it is what every JWKS consumer takes and what the relay's own library expects;
// the tokens live for minutes, so a longer key would buy nothing this threat model can spend.
const KeyBits = 2048

// PermissionClaim is the claim the relay reads its permissions out of, as MediaMTX's
// authJWTClaimKey names it by default.
// It is stated here because the token and the relay's configuration have to agree,
// and a claim the relay does not read is a token that grants nothing while looking valid.
const PermissionClaim = "mediamtx_permissions"

// The actions a permission may grant, as the relay names them.
// A stream needs two: one to push it and one to pull it.
const (
	ActionPublish = "publish"
	ActionRead    = "read"
)

// Permission is one thing a token allows: an action, and the path it is allowed on.
//
// The path is a regular expression where it starts with a tilde, which is how the relay reads one.
// That is what lets a group's token name every stream of that group without listing them,
// and what makes the grant a prefix match rather than an enumeration the service would have to keep
// current.
type Permission struct {
	Action string `json:"action"`
	Path   string `json:"path"`
}

// GroupPermissions is what a member of one group may do: publish and read every stream under that
// group's prefix, and nothing outside it.
//
// The expression is anchored at both ends of the prefix rather than only at the start,
// because an unanchored one would grant every group whose id merely contains this one's.
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

// regexpQuote escapes the characters a relay path may carry that a regular expression would
// otherwise read as syntax.
//
// A group prefix is base32 and a slash, so nothing in it needs escaping today.
// It is done all the same, because the one thing that must not happen is a prefix that reaches the
// relay as a pattern matching more than itself.
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
type Signer struct {
	key *rsa.PrivateKey
	// id names this key in the JWKS and in every token's header, so a relay holding two keys during a
	// rotation knows which one verified a token.
	// It is derived from the key rather than drawn, so the same key always publishes under the same
	// name.
	id string
}

// NewSigner draws a signing key.
//
// A random source that cannot answer is an Umgebungsfehler and leaves as an error:
// a key drawn from something weaker would sign tokens that look exactly like real ones.
func NewSigner() (*Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return nil, fmt.Errorf("drawing a signing key: %w", err)
	}
	return SignerFor(key), nil
}

// SignerFor is a signer over a key from somewhere else, which is what an operator restarting the
// service with the key it had before hands it.
func SignerFor(key *rsa.PrivateKey) *Signer {
	assert.IsNotNil(key, "a signer carries the key it signs with")

	s := &Signer{key: key, id: keyID(&key.PublicKey)}
	assert.Assert(s.id != "", "a signer's key is named")
	return s
}

// KeyID is what this signer's key is called in the JWKS and in the tokens it signs.
func (s *Signer) KeyID() string { return s.id }

// Sign issues a token granting these permissions, valid for the given window from now.
//
// The window is the caller's because it is a policy rather than a fact: short enough that a member
// who has left cannot open a new connection for long, long enough that a client is not asking for
// one between every reconnect.
// What this refuses is a window of nothing, which would be a token that has expired before it
// arrives.
func (s *Signer) Sign(subject string, permissions []Permission, now time.Time, window time.Duration) (string, error) {
	assert.IsNotNil(s.key, "a signing token is signed with a key")
	assert.Assert(window > 0, "a token is valid for some length of time", window.String())
	assert.Assert(len(permissions) > 0, "a token grants something", subject)

	header := map[string]string{"alg": Algorithm, "typ": "JWT", "kid": s.id}
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
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing a token: %w", err)
	}

	token := signing + "." + raw.EncodeToString(signature)
	assert.Assert(strings.Count(token, ".") == 2, "a signed token is a header, a payload and a signature")
	return token, nil
}

// JWKS is the document the relay fetches to verify tokens with: this signer's public key,
// and nothing else.
//
// One key at a time.
// A rotation that had to overlap would publish two, and the key id in each token's header is what
// lets a verifier holding both pick the right one; the field is there for that reason rather than
// because anything publishes two today.
func (s *Signer) JWKS() []byte {
	assert.IsNotNil(s.key, "a published key set is built from a key")

	pub := s.key.PublicKey
	document := map[string]any{"keys": []map[string]string{{
		"kty": "RSA",
		"alg": Algorithm,
		"use": "sig",
		"kid": s.id,
		"n":   raw.EncodeToString(pub.N.Bytes()),
		"e":   raw.EncodeToString(exponent(pub.E)),
	}}}
	out, err := json.Marshal(document)
	assert.IsNil(err, "a JWKS built from a key renders")
	return out
}

// raw is the base64 every part of a JWT and a JWKS is spelled in: URL-safe and unpadded,
// which is what makes a token one token rather than three fields needing escaping.
var raw = base64.RawURLEncoding

// segment renders one part of a token.
func segment(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("rendering a token: %w", err)
	}
	return raw.EncodeToString(out), nil
}

// exponent is the public exponent in the shortest big-endian form, which is what a JWKS carries:
// leading zero bytes are not part of the number and a verifier reading them would compute against a
// different key.
func exponent(e int) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(e))
	for len(out) > 1 && out[0] == 0 {
		out = out[1:]
	}

	assert.Assert(len(out) > 0 && out[0] != 0, "a rendered exponent carries no leading zero byte", out)
	return out
}

// keyID names a key by a digest of what it publishes, so the same key is always called the same
// thing and two keys are never called one.
func keyID(pub *rsa.PublicKey) string {
	assert.IsNotNil(pub, "a key is named after the key it is")

	digest := sha256.Sum256(append(pub.N.Bytes(), exponent(pub.E)...))
	return raw.EncodeToString(digest[:8])
}

// PrivateKey is the key this signer signs with, for a caller that stores it so a restart keeps
// issuing tokens the relay's cached JWKS still verifies.
//
// It is the one thing here that hands the secret out, and the caller that asks is the one that drew
// it.
// A key readable by anything else is every group's streams.
func (s *Signer) PrivateKey() *rsa.PrivateKey { return s.key }
