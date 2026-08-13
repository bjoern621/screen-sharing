package token

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

// A token is verified by the relay and never by this app, so what these hold is the shape a
// verifier reads: the three segments, the signature over the first two, and a JWKS carrying the key
// that checks it.
// A token this package signs and nothing can verify is a stream nobody can open.

func mustSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner()
	if err != nil {
		t.Fatalf("drawing a signing key: %v", err)
	}
	return s
}

// The signature is over the header and the payload, in the algorithm the header names.
// It is verified here the way a relay verifies it: recompute the digest over the first two segments
// and check it against the third with the public key.
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
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(publishedKey(t, s), crypto.SHA256, digest[:], signature); err != nil {
		t.Errorf("a token does not verify against the key its own JWKS publishes: %v", err)
	}
}

// publishedKey rebuilds the public key out of the JWKS, which is the only way a relay ever sees it.
// Reading the signer's own key instead would verify a token against something no verifier holds.
func publishedKey(t *testing.T, s *Signer) *rsa.PublicKey {
	t.Helper()
	var document struct {
		Keys []struct {
			Kty, Alg, Use, Kid, N, E string
		} `json:"keys"`
	}
	if err := json.Unmarshal(s.JWKS(), &document); err != nil {
		t.Fatalf("reading the JWKS: %v", err)
	}
	if len(document.Keys) != 1 {
		t.Fatalf("the JWKS carries %d keys, want one", len(document.Keys))
	}
	key := document.Keys[0]
	if key.Kty != "RSA" || key.Alg != Algorithm || key.Use != "sig" {
		t.Errorf("the JWKS describes a %s/%s/%s key, want an RSA %s signing key", key.Kty, key.Alg, key.Use, Algorithm)
	}
	if key.Kid != s.KeyID() {
		t.Errorf("the JWKS names the key %q where its tokens name it %q", key.Kid, s.KeyID())
	}

	n, err := raw.DecodeString(key.N)
	if err != nil {
		t.Fatalf("reading the modulus: %v", err)
	}
	e, err := raw.DecodeString(key.E)
	if err != nil {
		t.Fatalf("reading the exponent: %v", err)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
}

// The claims are what the relay reads: the window it checks the connection against,
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

// The header names one algorithm and this package produces one.
// A verifier that read the algorithm out of the token is one an attacker points at "none";
// the relay is configured with this algorithm, and a token claiming another is a token it refuses.
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

	var read struct{ Alg, Typ, Kid string }
	if err := json.Unmarshal(header, &read); err != nil {
		t.Fatalf("reading the header: %v", err)
	}
	if read.Alg != Algorithm || read.Typ != "JWT" {
		t.Errorf("the header says %s/%s, want %s/JWT", read.Alg, read.Typ, Algorithm)
	}
	if read.Kid != s.KeyID() {
		t.Errorf("the header names key %q, where the JWKS publishes %q", read.Kid, s.KeyID())
	}
}

// A grant is anchored at both ends of the prefix.
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

// One key is always called the same thing, and two keys are never called one:
// a relay holding both during a rotation picks the one a token's header names.
func TestAKeyIsNamedByWhatItPublishes(t *testing.T) {
	first, second := mustSigner(t), mustSigner(t)
	if first.KeyID() != SignerFor(first.key).KeyID() {
		t.Error("one key is named two things")
	}
	if first.KeyID() == second.KeyID() {
		t.Errorf("two keys are named one thing: %s", first.KeyID())
	}
}
