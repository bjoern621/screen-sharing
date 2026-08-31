package group

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both sides of the wire run this derivation, the client for the prefix it publishes under and the
// service for the prefix it grants a token on.
// These hold that the two can only agree, and that what the derivation publishes says nothing about
// the secret behind it.

// mustKey fails the test on a drawing failure rather than carrying it into a path assertion.
func mustKey(t *testing.T) Key {
	t.Helper()
	key, err := NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	return key
}

// One key, one prefix, every time: the whole contract between the two sides.
// A derivation answering differently on two calls issues a member a token for a path nobody
// publishes to.
func TestOneKeyDerivesOnePrefix(t *testing.T) {
	key := mustKey(t)
	if key.ID() != key.ID() {
		t.Error("one key derived two ids")
	}

	// Through the encoding is the path the derivation takes in practice: the client stores it,
	// the service is handed it.
	same, err := ParseKey(key.String())
	if err != nil {
		t.Fatalf("reading back a key this package wrote: %v", err)
	}
	if same.ID() != key.ID() {
		t.Errorf("a key read back derives %s, where the key itself derives %s", same.ID(), key.ID())
	}
}

// Membership is possession, so two secrets landing on one prefix are two groups watching each
// other's streams.
func TestTwoKeysAreTwoGroups(t *testing.T) {
	first, second := mustKey(t), mustKey(t)
	if first.ID() == second.ID() {
		t.Errorf("two keys derived one id %s", first.ID())
	}
}

// The id is public, in every URL a member pastes, so it must say nothing about the key.
// A digest of the key alone makes the two one value under a hash anyone can compute,
// where the keyed derivation under a label separates them.
func TestTheIdCarriesNothingOfTheKey(t *testing.T) {
	key := mustKey(t)
	id := key.ID()

	if strings.Contains(key.String(), id) {
		t.Error("the id appears inside the key's own encoding")
	}
	// The encoding's own alphabet, so a path carrying an id needs no escaping and a member reading one
	// aloud has no case to get wrong.
	for _, c := range id {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", c) {
			t.Errorf("the id carries %q, which is outside the alphabet a path takes unescaped", c)
		}
	}
	if len(id) != IDChars {
		t.Errorf("the id is %d characters, want %d", len(id), IDChars)
	}
}

// A path is id, slash and the stream's own name, and reads back into those two.
// Relay permissions match on the prefix, so the separator stops one group's grant at its own
// streams.
func TestAPathIsTheGroupAndTheStream(t *testing.T) {
	key := mustKey(t)

	path, err := key.Path("standup")
	if err != nil {
		t.Fatalf("building a path: %v", err)
	}
	if path != key.ID()+"/standup" {
		t.Errorf("path = %q, want the id and the name", path)
	}
	if !strings.HasPrefix(path, key.Prefix()) {
		t.Errorf("path %q does not start with the prefix %q a relay permission is written against", path, key.Prefix())
	}

	id, name, ok := Split(path)
	if !ok || id != key.ID() || name != "standup" {
		t.Errorf("splitting %q gives (%q, %q, %v)", path, id, name, ok)
	}
	if !key.Holds(path) {
		t.Error("a key does not hold the path it derived")
	}
}

// Publishing always takes a group, so a stream with no key is refused rather than published under
// its bare name, which is a path every other group can see.
func TestAStreamWithNoGroupIsRefused(t *testing.T) {
	if _, err := Key(nil).Path("standup"); err != ErrNoGroup {
		t.Errorf("a stream with no group yielded %v, want %v", err, ErrNoGroup)
	}
}

// A name carrying a separator lands a segment deeper than the group's permission covers,
// which is a stream inside a group its own grant does not reach.
func TestANameIsOneSegment(t *testing.T) {
	key := mustKey(t)
	if _, err := key.Path("team/standup"); err == nil {
		t.Error("a stream name carrying a separator was accepted")
	}
	if _, err := key.Path(""); err == nil {
		t.Error("a stream with no name of its own was accepted")
	}
}

// A path with no separator belongs to no group: a stream published outside the group model.
// Reporting it as a group of its own would let a listing match on a stream name.
func TestAPathWithNoGroupBelongsToNone(t *testing.T) {
	for _, path := range []string{"standup", "", "/standup", "standup/"} {
		if id, name, ok := Split(path); ok {
			t.Errorf("%q reads as group %q stream %q, and it names no group", path, id, name)
		}
	}
	if mustKey(t).Holds("standup") {
		t.Error("a key holds a path that names no group")
	}
}

// This app did not produce a key of the wrong length, and a prefix derived from one would put a
// stream where no member is looking.
func TestAKeyOfTheWrongLengthIsRefused(t *testing.T) {
	for _, encoded := range []string{"", "not base64 at all", "c2hvcnQ="} {
		if _, err := ParseKey(encoded); err == nil {
			t.Errorf("%q was accepted as a group key", encoded)
		}
	}
}

// mustSecret fails the test on a drawing failure rather than carrying it into a derivation.
func mustSecret(t *testing.T) MemberSecret {
	t.Helper()
	secret, err := NewMemberSecret()
	if err != nil {
		t.Fatalf("drawing a member secret: %v", err)
	}
	return secret
}

// A member id names one member of a group on the relay, and the relay logs and lists it.
// These hold that it identifies without carrying what it was derived from.
func TestOneSecretDerivesOneMemberID(t *testing.T) {
	key := mustKey(t)
	secret := mustSecret(t)

	if key.MemberID(secret) != key.MemberID(secret) {
		t.Error("one secret derived two member ids")
	}

	// Through the encoding is the path the derivation takes in practice: the app writes the secret to
	// its identity file and reads it back on the next start.
	same, err := ParseMemberSecret(secret.String())
	if err != nil {
		t.Fatalf("reading back a secret this package wrote: %v", err)
	}
	if key.MemberID(same) != key.MemberID(secret) {
		t.Errorf("a secret read back derives %s, where the secret itself derives %s",
			key.MemberID(same), key.MemberID(secret))
	}
}

// Identity inside a group cannot be forged, so two secrets are two members however they are named.
func TestTwoSecretsAreTwoMembers(t *testing.T) {
	key := mustKey(t)

	if key.MemberID(mustSecret(t)) == key.MemberID(mustSecret(t)) {
		t.Error("two secrets derived one member id")
	}
}

// The id travels to the relay, which writes it into a log line and a session listing.
// The secret behind it is what a member's whole identity rests on, so a relay operator reading one
// off a listing would be able to state that member's presence.
func TestAMemberIDCarriesNeitherTheSecretNorTheKey(t *testing.T) {
	key := mustKey(t)
	secret := mustSecret(t)

	id := key.MemberID(secret)
	if strings.Contains(secret.String(), id) {
		t.Errorf("the member id %s appears inside the secret's own encoding", id)
	}
	if strings.Contains(id, key.String()) || strings.Contains(id, key.ID()) {
		t.Errorf("the member id %s carries the group's own secret or id", id)
	}
	if len(id) != MemberIDChars {
		t.Errorf("the member id is %d characters, want %d", len(id), MemberIDChars)
	}
	for _, c := range id {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", c) {
			t.Errorf("the member id carries %q, which is outside the alphabet a relay listing takes", c)
		}
	}
}

// One app joining two groups holds one secret per group, and a member id derives under the group's
// key, so neither group's relay listing names the other's member.
func TestOneSecretInTwoGroupsIsTwoMembers(t *testing.T) {
	here, there := mustKey(t), mustKey(t)
	secret := mustSecret(t)

	if here.MemberID(secret) == there.MemberID(secret) {
		t.Error("one secret derived one member id across two groups")
	}
}

// A secret of the wrong length is one this app did not draw, and a member id derived from it would
// be a subject membership can neither match nor explain.
func TestAMemberSecretOfTheWrongLengthIsRefused(t *testing.T) {
	for _, encoded := range []string{"", "not base64 at all", "c2hvcnQ="} {
		if _, err := ParseMemberSecret(encoded); err == nil {
			t.Errorf("%q was accepted as a member secret", encoded)
		}
	}
}

// The SRT leg is keyed per group, both ends deriving the passphrase from the key they share:
// the app for the legs it opens,
// the group service for the path configuration it writes into the relay.
// Two derivations that disagree are a publish the relay's own listener refuses.
func TestOneKeyDerivesOnePassphrase(t *testing.T) {
	key := mustKey(t)
	if key.SrtPassphrase() != key.SrtPassphrase() {
		t.Error("one key derived two passphrases")
	}

	// Through the encoding is the path the derivation takes in practice: the app reads the key off
	// its settings file, the service is handed it per request.
	same, err := ParseKey(key.String())
	if err != nil {
		t.Fatalf("reading back a key this package wrote: %v", err)
	}
	if same.SrtPassphrase() != key.SrtPassphrase() {
		t.Error("a key read back derives another passphrase")
	}
}

// One group's members cannot read another group's SRT packets, which is what a passphrase per key
// buys over one value for the whole relay.
func TestTwoKeysAreTwoPassphrases(t *testing.T) {
	if mustKey(t).SrtPassphrase() == mustKey(t).SrtPassphrase() {
		t.Error("two keys derived one passphrase")
	}
}

// libsrt refuses a passphrase outside 10 to 79 characters,
// and the group service writes this one into the relay's path configuration,
// so an out-of-bounds derivation is a leg nothing can open.
// The public one is under the same bound for the same reason.
func TestEveryPassphraseFitsSrtsBounds(t *testing.T) {
	for name, passphrase := range map[string]string{
		"a derived passphrase": mustKey(t).SrtPassphrase(),
		"the public one":       PublicSrtPassphrase,
	} {
		if len(passphrase) < 10 || len(passphrase) > 79 {
			t.Errorf("%s is %d characters, and libsrt takes 10 to 79", name, len(passphrase))
		}
	}
}

// The public prefix's passphrase is spelled twice,
// in PublicSrtPassphrase and in the relay's own configuration,
// no group service standing in the path to write it there.
// This holds the two spellings together:
// a relay keyed on one and an app on the other is a public SRT leg that never connects.
func TestTheRelayConfigurationSpellsThePublicPassphrase(t *testing.T) {
	config, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "mediamtx-groups.yml"))
	if err != nil {
		t.Fatalf("reading the relay configuration every deployment runs: %v", err)
	}

	for _, key := range []string{"srtPublishPassphrase", "srtReadPassphrase"} {
		want := key + `: "` + PublicSrtPassphrase + `"`
		if !strings.Contains(string(config), want) {
			t.Errorf("the relay configuration does not key the public prefix with %s", want)
		}
	}
}

// The passphrase reaches the relay's configuration and every SRT handshake,
// so it must say nothing about the key behind it
// and nothing another derivation of the same key publishes.
func TestThePassphraseCarriesNeitherTheKeyNorTheId(t *testing.T) {
	key := mustKey(t)
	passphrase := key.SrtPassphrase()

	if strings.Contains(key.String(), passphrase) {
		t.Error("the passphrase appears inside the key's own encoding")
	}
	if strings.Contains(passphrase, key.ID()) || strings.Contains(key.ID(), passphrase) {
		t.Error("the passphrase and the public id share their content")
	}
}

// Enforcement is handed a connection's path by the relay and has to find the group it belongs to,
// there being no key on that side of the exchange.
func TestAPathNamesThePrefixItIsEnforcedUnder(t *testing.T) {
	key := mustKey(t)

	path, err := key.Path("desk")
	if err != nil {
		t.Fatalf("deriving a stream path: %v", err)
	}
	prefix, ok := PrefixOf(path)
	if !ok {
		t.Fatalf("the path %s named no group", path)
	}
	if prefix != key.Prefix() {
		t.Errorf("the path %s named the prefix %s, where its key derives %s", path, prefix, key.Prefix())
	}
}

// A stream published outside the group model belongs to no group, and reporting one as a group of
// its own would enforce membership against a stream name.
func TestAPathOutsideAGroupNamesNoPrefix(t *testing.T) {
	if prefix, ok := PrefixOf("desk"); ok {
		t.Errorf("a bare stream name named the group %s", prefix)
	}
}
