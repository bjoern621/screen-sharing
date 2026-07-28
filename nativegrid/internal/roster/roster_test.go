package roster

import "testing"

// The literal is the wire format the app's watch package writes.
func TestParseReadsTheAppState(t *testing.T) {
	cfg, err := Parse(`{"streams":[],"app":{"publishing":true,"publishError":"no encoder"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App == nil {
		t.Fatal("the push carried app state and the config has none")
	}
	if !cfg.App.Publishing || cfg.App.PublishError != "no encoder" {
		t.Errorf("app = %+v, want a publishing app with the encoder message", *cfg.App)
	}
}

// A config with no app state is what a window with no app behind it opens on,
// and the sidebar draws no app controls for it. Reading a missing section as an
// idle app instead would put a publish button in front of nobody.
func TestParseReadsAMissingAppStateAsNone(t *testing.T) {
	cfg, err := Parse(`{"streams":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App != nil {
		t.Errorf("app = %+v, want none", *cfg.App)
	}
}
