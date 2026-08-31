package token

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

// The relay verifies a token and this app never does, so what these pin is the shape a verifier
// reads: three segments, a signature over the first two, and a JWKS carrying the key that checks
// it.
// A token this package signs and nothing can verify is a stream nobody can open.

func mustSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner()
	if err != nil {
		t.Fatalf("drawing a signing key: %v", err)
	}
	return s
}

// Verified the way a relay verifies:
// digest the first two segments, check the third against it with the published key.
func TestATokenVerifiesAgainstTheKeyItPublishes(t *testing.T) {
	s := mustSigner(t)

	signed, err := s.Sign("group", GroupPermissions("ABCD/"), time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		t.Fatalf("a token carries %d segments, want three", len(parts))
	}

	signature, err := raw.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("reading the signature: %v", err)
	}
	if len(signature) != 2*CoordinateBytes {
		t.Fatalf("the signature is %d bytes, want two %d-byte numbers", len(signature), CoordinateBytes)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:CoordinateBytes])
	v := new(big.Int).SetBytes(signature[CoordinateBytes:])
	if !ecdsa.Verify(publishedKey(t, s), digest[:], r, v) {
		t.Error("a token does not verify against the key its own JWKS publishes")
	}
}

// publishedKey rebuilds the public key out of the JWKS, the only way a relay ever sees it.
// Reading the signer's own key would verify a token against something no verifier holds.
func publishedKey(t *testing.T, s *Signer) *ecdsa.PublicKey {
	t.Helper()
	var document struct {
		Keys []struct {
			Kty, Crv, Alg, Use, Kid, X, Y string
		} `json:"keys"`
	}
	if err := json.Unmarshal(s.JWKS(), &document); err != nil {
		t.Fatalf("reading the JWKS: %v", err)
	}
	if len(document.Keys) != 1 {
		t.Fatalf("the JWKS carries %d keys, want one", len(document.Keys))
	}
	key := document.Keys[0]
	if key.Kty != "EC" || key.Crv != Curve || key.Alg != Algorithm || key.Use != "sig" {
		t.Errorf("the JWKS describes a %s/%s/%s/%s key, want an EC %s %s signing key",
			key.Kty, key.Crv, key.Alg, key.Use, Curve, Algorithm)
	}
	if key.Kid != s.KeyID() {
		t.Errorf("the JWKS names the key %q where its tokens name it %q", key.Kid, s.KeyID())
	}

	x, err := raw.DecodeString(key.X)
	if err != nil {
		t.Fatalf("reading the x coordinate: %v", err)
	}
	y, err := raw.DecodeString(key.Y)
	if err != nil {
		t.Fatalf("reading the y coordinate: %v", err)
	}
	if len(x) != CoordinateBytes || len(y) != CoordinateBytes {
		t.Errorf("the JWKS publishes a %d and a %d byte coordinate, want %d each", len(x), len(y), CoordinateBytes)
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}
}

// Fitting the SRT stream id is why the algorithm is ES256 rather than RS256.
// Measured against a real group's prefix and the longest stream name the app builds a path from,
// that being the token that has to fit rather than the shortest one.
func TestATokenFitsTheStreamIdThatCarriesIt(t *testing.T) {
	s := mustSigner(t)
	prefix := strings.Repeat("A", 26) + "/"

	signed, err := s.Sign(strings.Repeat("A", 26), GroupPermissions(prefix), time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if len(signed) > MaxTokenBytes {
		t.Errorf("a token is %d bytes, over the %d an SRT stream id leaves for one", len(signed), MaxTokenBytes)
	}
}

// What the relay reads:
// the window it checks a connection against,
// and the permissions under the claim its configuration names.
func TestATokenCarriesTheWindowAndThePermissions(t *testing.T) {
	s := mustSigner(t)
	now := time.Unix(1_700_000_000, 0)

	signed, err := s.Sign("group", GroupPermissions("ABCD/"), now, 90*time.Second)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	var claims struct {
		Sub         string       `json:"sub"`
		Iat, Exp    int64        `json:"-"`
		IatValue    int64        `json:"iat"`
		ExpValue    int64        `json:"exp"`
		Permissions []Permission `json:"mediamtx_permissions"`
	}
	payload, err := raw.DecodeString(strings.Split(signed, ".")[1])
	if err != nil {
		t.Fatalf("reading the payload: %v", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("reading the claims: %v", err)
	}

	if claims.ExpValue-claims.IatValue != 90 {
		t.Errorf("the token is valid for %d seconds, want 90", claims.ExpValue-claims.IatValue)
	}
	if claims.IatValue != now.Unix() {
		t.Errorf("the token was issued at %d, want %d", claims.IatValue, now.Unix())
	}
	if len(claims.Permissions) != 2 {
		t.Fatalf("the token grants %+v, want publishing and reading", claims.Permissions)
	}
	for _, p := range claims.Permissions {
		if !strings.HasPrefix(p.Path, "~^ABCD/") {
			t.Errorf("a permission is granted on %q, which is not anchored at this group's prefix", p.Path)
		}
	}
}

// One algorithm in the header because this package produces one.
// A verifier that read the algorithm out of the token could be pointed at "none",
// so the relay is configured with this one and refuses a token claiming another.
func TestTheHeaderNamesTheOneAlgorithm(t *testing.T) {
	s := mustSigner(t)

	signed, err := s.Sign("group", GroupPermissions("ABCD/"), time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	header, err := raw.DecodeString(strings.Split(signed, ".")[0])
	if err != nil {
		t.Fatalf("reading the header: %v", err)
	}

	var read struct{ Alg, Kid string }
	if err := json.Unmarshal(header, &read); err != nil {
		t.Fatalf("reading the header: %v", err)
	}
	if read.Alg != Algorithm {
		t.Errorf("the header says %s, want %s", read.Alg, Algorithm)
	}
	if read.Kid != s.KeyID() {
		t.Errorf("the header names key %q, where the JWKS publishes %q", read.Kid, s.KeyID())
	}
}

// A grant is anchored at the start of the prefix.
// Unanchored, one group's token would open every group whose id merely contains it.
func TestAGrantReachesOneGroupAndNoOther(t *testing.T) {
	permissions := GroupPermissions("ABCD/")
	if len(permissions) != 2 {
		t.Fatalf("a group is granted %+v, want publishing and reading", permissions)
	}
	for _, p := range permissions {
		if !strings.HasPrefix(p.Path, "~^") {
			t.Errorf("%q is not anchored at the start, so it matches every path containing the prefix", p.Path)
		}
	}
	if permissions[0].Action != ActionPublish || permissions[1].Action != ActionRead {
		t.Errorf("a group is granted %s and %s", permissions[0].Action, permissions[1].Action)
	}
}

// One key keeps one name and two keys never share one,
// which is how a relay holding both through a rotation picks the one a token's header names.
func TestAKeyIsNamedByWhatItPublishes(t *testing.T) {
	first, second := mustSigner(t), mustSigner(t)
	if first.KeyID() != SignerFor(first.key).KeyID() {
		t.Error("one key is named two things")
	}
	if first.KeyID() == second.KeyID() {
		t.Errorf("two keys are named one thing: %s", first.KeyID())
	}
}
