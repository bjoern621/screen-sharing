package groupsvc

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"bjoernblessin.de/screenshare/internal/membership"
	"bjoernblessin.de/screenshare/internal/token"
)

// SRT is keyed per group: the app derives the passphrase from the group key, and this service
// writes the same derivation into the relay's per-prefix path configuration.
// These hold that every route handed a group key writes the keys through,
// and that a relay that will not take them costs the leg and never the answer.

// keyed records what the service wrote into the relay's path configuration.
type keyed struct {
	ensured map[string]string // prefix -> passphrase
	fail    error
}

func (k *keyed) Ensure(prefix, passphrase string) error {
	if k.fail != nil {
		return k.fail
	}
	if k.ensured == nil {
		k.ensured = map[string]string{}
	}
	k.ensured[prefix] = passphrase
	return nil
}

// keyedService is a service whose relay records the SRT keys written into it.
func keyedService(t *testing.T) (*Service, *keyed) {
	t.Helper()
	signer, err := token.NewSigner()
	if err != nil {
		t.Fatalf("drawing a signing key: %v", err)
	}
	keys := &keyed{}
	return New(signer, paths(nil), membership.New(&carrying{}), keys), keys
}

// A member's SRT handshake carries the derived passphrase,
// so the prefix has to be keyed at the relay before the connection the token buys is opened.
// The token route runs ahead of every connection, which is what makes it the place to write.
func TestAGroupTokenKeysItsPrefixOnTheRelay(t *testing.T) {
	s, keys := keyedService(t)
	groupKey := mustKey(t)

	status, body := call(t, s, "POST", "/tokens",
		`{"groupKey": "`+groupKey.String()+`", "memberSecret": "`+mustSecret(t).String()+`"}`)
	if status != http.StatusOK {
		t.Fatalf("issuing a token answered %d: %v", status, body)
	}
	if got := keys.ensured[groupKey.Prefix()]; got != groupKey.SrtPassphrase() {
		t.Errorf("the prefix is keyed with %q, want the key's own derivation", got)
	}
}

// Presence rides the poll a member's app already runs,
// so a relay that restarted empty is keyed again within one pass
// rather than when the next token is asked for.
func TestStatingPresenceKeysThePrefix(t *testing.T) {
	s, keys := keyedService(t)
	groupKey := mustKey(t)

	status, body := call(t, s, "PUT", "/members",
		`{"groupKey": "`+groupKey.String()+`", "memberSecret": "`+mustSecret(t).String()+`", "displayName": "bob"}`)
	if status != http.StatusOK {
		t.Fatalf("stating presence answered %d: %v", status, body)
	}
	if got := keys.ensured[groupKey.Prefix()]; got != groupKey.SrtPassphrase() {
		t.Errorf("the prefix is keyed with %q, want the key's own derivation", got)
	}
}

// The public prefix is keyed statically in the relay's own configuration,
// no key existing to derive from, so a public token writes nothing.
func TestAPublicTokenKeysNothing(t *testing.T) {
	s, keys := keyedService(t)

	status, body := call(t, s, "POST", "/tokens", "")
	if status != http.StatusOK {
		t.Fatalf("issuing a public token answered %d: %v", status, body)
	}
	if len(keys.ensured) != 0 {
		t.Errorf("a public token keyed %v", keys.ensured)
	}
}

// An unreachable relay API costs the SRT leg until a later call reaches it,
// and it may not cost the token:
// every other leg still works, and a refusal here would take all of them.
func TestARelayThatWillNotTakeTheKeysStillAnswers(t *testing.T) {
	s, keys := keyedService(t)
	keys.fail = errors.New("the relay is down")
	groupKey := mustKey(t)

	status, body := call(t, s, "POST", "/tokens", `{"groupKey": "`+groupKey.String()+`"}`)
	if status != http.StatusOK {
		t.Fatalf("issuing a token against an unreachable relay answered %d: %v", status, body)
	}
	if body["relayAccessToken"] == "" {
		t.Error("the answer carries no token")
	}
}

// The index route is a read and opens nothing, so it writes nothing either:
// the token every connection needs is what keys the prefix,
// and a listing may be asked for by a member who never connects.
func TestListingStreamsKeysNothing(t *testing.T) {
	s, keys := keyedService(t)

	status, _ := call(t, s, "GET", "/streams?groupKey="+url.QueryEscape(mustKey(t).String()), "")
	if status != http.StatusOK {
		t.Fatalf("listing answered %d", status)
	}
	if len(keys.ensured) != 0 {
		t.Errorf("a listing keyed %v", keys.ensured)
	}
}
