package groupsvc

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/token"
)

// listing is the index request for one key.
//
// The key is escaped rather than pasted in: a key is standard base64 and a '+' in a query string
// decodes to a space, so an unescaped key carrying one reaches the service corrupted.
func listing(key group.Key) string {
	return "/streams?group=" + url.QueryEscape(key.String())
}

// The service is three derivations and a bound.
// These hold that each derivation answers what the other side computes, and that the one decision
// the service makes, who may see which streams, is made here and not left to a caller to filter.

// paths is a relay carrying these streams.
// All ready, because what an index answers about a path that is not is the same question as what it
// answers about another group's, and that has its own test.
type paths []string

func (p paths) Paths() []Stream {
	out := make([]Stream, 0, len(p))
	for _, path := range p {
		out = append(out, Stream{Path: path, Ready: true, Format: "h264"})
	}
	return out
}

// service signs with a fresh key and reads a relay carrying these streams.
func service(t *testing.T, streams ...string) *Service {
	t.Helper()
	signer, err := token.NewSigner()
	if err != nil {
		t.Fatalf("drawing a signing key: %v", err)
	}
	return New(signer, paths(streams))
}

// call makes one request and returns its status and decoded body.
func call(t *testing.T, s *Service, method, target, body string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)

	var answer map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s %s answered %q, which is not JSON: %v", method, target, w.Body.String(), err)
	}
	return w.Code, answer
}

// Creating a group draws a key, and the key is what the client keeps.
// The id beside it is the prefix that key derives, so a client can check the two agree without
// deriving anything itself.
func TestCreatingAGroupHandsBackAKeyAndThePrefixItDerives(t *testing.T) {
	s := service(t)

	status, body := call(t, s, "POST", "/groups", "")
	if status != http.StatusOK {
		t.Fatalf("creating a group answered %d: %v", status, body)
	}

	key, err := group.ParseKey(body["key"].(string))
	if err != nil {
		t.Fatalf("the service handed back a key it cannot read: %v", err)
	}
	if body["id"] != key.ID() {
		t.Errorf("the service says the group is %v, where the key derives %s", body["id"], key.ID())
	}

	// Two creations are two groups: a service handing out one key is one group everybody is in.
	_, second := call(t, s, "POST", "/groups", "")
	if second["key"] == body["key"] {
		t.Error("two creations handed back one key")
	}
}

// A key buys a token granting that key's prefix and nothing else.
// Nothing is looked up on the way: a caller holding a well-formed key is a member, and that is the
// whole model.
func TestAKeyBuysATokenForItsOwnPrefix(t *testing.T) {
	s := service(t)
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a key: %v", err)
	}

	status, body := call(t, s, "POST", "/tokens", `{"key":"`+key.String()+`"}`)
	if status != http.StatusOK {
		t.Fatalf("issuing a token answered %d: %v", status, body)
	}
	if body["prefix"] != key.Prefix() {
		t.Errorf("the token is for %v, where the key derives %s", body["prefix"], key.Prefix())
	}

	signed, _ := body["token"].(string)
	if strings.Count(signed, ".") != 2 {
		t.Fatalf("the token is %q, which is not a JWT", signed)
	}
	// The grant is anchored at this group's prefix, which keeps the token off every group whose id
	// merely contains it.
	if !strings.Contains(claims(t, signed), "~^"+key.ID()) {
		t.Errorf("the token grants %s, which does not name this group's prefix", claims(t, signed))
	}
}

// claims is a token's payload as text, for an assertion about what it grants.
func claims(t *testing.T, signed string) string {
	t.Helper()
	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		t.Fatalf("a token carries %d segments", len(parts))
	}
	payload, err := base64Raw.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("reading the payload: %v", err)
	}
	return string(payload)
}

// A key nothing produced buys nothing.
// A prefix derived from one grants a token for a path nobody publishes to, which reads as a group
// that exists.
func TestAKeyTheServiceCannotReadBuysNothing(t *testing.T) {
	s := service(t)
	for _, body := range []string{``, `{}`, `{"key":"nonsense"}`, `{"key":"c2hvcnQ="}`} {
		if status, _ := call(t, s, "POST", "/tokens", body); status != http.StatusBadRequest {
			t.Errorf("%q was answered %d, want a refusal", body, status)
		}
	}
}

// The index enforces the split rather than leaving a shell to filter: a key sees its own group,
// no key sees the public streams, and neither sees the other's.
func TestTheIndexAnswersOneGroupAndNeverAnother(t *testing.T) {
	mine, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a key: %v", err)
	}
	theirs, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a key: %v", err)
	}
	s := service(t,
		mine.ID()+"/standup",
		theirs.ID()+"/secret",
		PublicPrefix+"demo",
		"loose",
	)

	_, body := call(t, s, "GET", listing(mine), "")
	if got := names(body); len(got) != 1 || got[0] != "standup" {
		t.Errorf("a member sees %v, want their own group's one stream", got)
	}

	// Without a key: the public streams and neither group's.
	// A group's listing hides the public ones for the same reason it hides another group's.
	_, public := call(t, s, "GET", "/streams", "")
	if got := names(public); len(got) != 1 || got[0] != "demo" {
		t.Errorf("a caller with no key sees %v, want the public streams", got)
	}
}

// names is the stream names off one answer.
// A row carries what a viewer opens the stream with beside the name, so the name is read out of the
// row rather than being it.
func names(body map[string]any) []string {
	var out []string
	for _, v := range body["streams"].([]any) {
		out = append(out, v.(map[string]any)["name"].(string))
	}
	return out
}

// A path with no group is in no listing, and neither is one nested a segment deeper than a group's
// own grant reaches.
func TestAPathOutsideAGroupIsInNoListing(t *testing.T) {
	key, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a key: %v", err)
	}
	s := service(t, "loose", key.ID()+"/team/standup", key.ID()+"/")

	_, body := call(t, s, "GET", listing(key), "")
	if got := names(body); len(got) != 0 {
		t.Errorf("the listing carries %v, and none of those is a stream of this group", got)
	}
}

// Creation is open, so it is bounded: a script filling the relay with prefixes meets the bound,
// where a person creating groups all afternoon does not.
func TestCreationIsBounded(t *testing.T) {
	s := service(t)
	for i := range CreationsPerHour {
		if status, _ := call(t, s, "POST", "/groups", ""); status != http.StatusOK {
			t.Fatalf("creation %d of %d was refused", i, CreationsPerHour)
		}
	}
	if status, _ := call(t, s, "POST", "/groups", ""); status != http.StatusTooManyRequests {
		t.Errorf("creation past the bound answered %d, want a refusal", status)
	}

	// The bound ages out an hour on, which is what makes it a rate and not a total.
	at := time.Now().Add(2 * time.Hour)
	s.now = func() time.Time { return at }
	if status, _ := call(t, s, "POST", "/groups", ""); status != http.StatusOK {
		t.Errorf("creation an hour after the bound answered %d, want it allowed again", status)
	}
}

// The relay fetches the key once and verifies every connection locally, so nothing here runs per
// stream.
// What it fetches has to be what signs the tokens.
func TestTheServicePublishesTheKeyItsTokensAreVerifiedWith(t *testing.T) {
	s := service(t)

	r := httptest.NewRequest("GET", "/jwks.json", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("the JWKS answered %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("the JWKS is served as %q", got)
	}
	if !strings.Contains(w.Body.String(), s.signer.KeyID()) {
		t.Error("the JWKS does not name the key the service signs with")
	}
}

// base64Raw is how every token segment is spelled, for the test that reads one back.
var base64Raw = base64.RawURLEncoding
