package group

import (
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
