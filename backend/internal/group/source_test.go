package group

import "testing"

func TestSourcesLeadWithTheKey(t *testing.T) {
	if len(Sources) != 2 || Sources[0] != SourceKey {
		t.Fatalf("sources %v", Sources)
	}
}

func TestKnownSourceTakesBothAndNothingElse(t *testing.T) {
	for _, s := range Sources {
		if !KnownSource(s) {
			t.Fatalf("%q is a source", s)
		}
	}
	for _, s := range []string{"", "Discord", "voice"} {
		if KnownSource(s) {
			t.Fatalf("%q is no source", s)
		}
	}
}
