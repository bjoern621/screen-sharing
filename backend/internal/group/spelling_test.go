package group

import (
	"strings"
	"testing"
)

// A name a person carries is spelled into the characters a relay path may hold, and read back.
// Every byte outside the path alphabet survives the trip, so a display name decides nothing about
// whether its owner can publish.
func TestANameSurvivesBeingSpelledIntoAPath(t *testing.T) {
	for _, name := range []string{
		"bjoern",
		"monitor-0",
		"bjoern/monitor-0",
		"Björn",
		"Björn/monitor-0",
		"Alice Smith/monitor-1",
		"under_score",
		"dot.dot",
		"emoji 😀",
		"DJ/Rex/monitor-0",
		"100%",
		"~tilde~",
		"ﬀ ligature ünïcode",
		".",
		"..",
		"dot.dot",
		".hidden/monitor-0",
	} {
		spelled := SpellName(name)
		if !pathHolds(spelled) {
			t.Errorf("%q spells %q, which a relay path cannot carry", name, spelled)
		}
		if strings.Count(spelled, separator) > 1 {
			t.Errorf("%q spells %q, which is more than two path segments", name, spelled)
		}
		read, ok := NameOf(spelled)
		if !ok || read != name {
			t.Errorf("%q spelled %q reads back as (%q, %v)", name, spelled, read, ok)
		}
	}
}

// A name already inside the path alphabet is spelled as it stands,
// so the path a stream lives at is the one a reader recognises in a log line.
func TestANameInsideTheAlphabetIsSpelledAsItStands(t *testing.T) {
	for _, name := range []string{"bjoern", "monitor-0", "bjoern/monitor-0", "Desk-2", "dot.dot"} {
		if spelled := SpellName(name); spelled != name {
			t.Errorf("%q spells %q, want it unchanged", name, spelled)
		}
	}
}

// A separator inside a display name is spelled rather than left to split the path:
// the member's own name and the stream's own are one segment each,
// whatever characters the member goes by.
func TestASeparatorInsideANameStaysInsideItsSegment(t *testing.T) {
	spelled := SpellName("DJ/Rex/monitor-0")
	member, stream, found := strings.Cut(spelled, separator)
	if !found || strings.Contains(stream, separator) {
		t.Fatalf("%q is not two path segments", spelled)
	}
	if strings.Contains(member, "/") {
		t.Errorf("the member segment %q carries a separator", member)
	}
	if read, ok := NameOf(spelled); !ok || read != "DJ/Rex/monitor-0" {
		t.Errorf("%q reads back as (%q, %v)", spelled, read, ok)
	}
}

// Reading is refused where the path was not spelled by this app,
// so a stream somebody else published under a name of their own is reported as it stands
// rather than as bytes nobody can read.
func TestAPathNobodySpelledIsRefused(t *testing.T) {
	for _, spelled := range []string{"half_", "odd_c3", "not_hexy", "lone_ff"} {
		if read, ok := NameOf(spelled); ok {
			t.Errorf("%q read as %q, want a refusal", spelled, read)
		}
	}
}

// A path is built from the spelled name, so a member whose name leaves the alphabet
// still publishes under their group's prefix.
func TestAPathCarriesANameOutsideTheAlphabet(t *testing.T) {
	key := mustKey(t)

	path, err := key.Path("Björn/monitor-0")
	if err != nil {
		t.Fatalf("building a path: %v", err)
	}
	if !strings.HasPrefix(path, key.Prefix()) {
		t.Errorf("path %q does not start with the prefix %q", path, key.Prefix())
	}
	if !pathHolds(path) {
		t.Errorf("path %q carries characters a relay refuses", path)
	}

	_, name, ok := Split(path)
	if !ok {
		t.Fatalf("splitting %q found no group", path)
	}
	if read, ok := NameOf(name); !ok || read != "Björn/monitor-0" {
		t.Errorf("the path's name reads back as (%q, %v)", read, ok)
	}
}
