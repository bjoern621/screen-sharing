package receive

import "testing"

// TestStatSourcesAreWellFormed holds the table's invariants. A reader prints
// these rows without knowing what they count, so a field that names no key would
// never render and one that explains nothing would render as a bare number.
func TestStatSourcesAreWellFormed(t *testing.T) {
	for _, src := range statSources {
		if src.factory == "" {
			t.Error("a stat source names no factory")
		}
		if src.tip == "" {
			t.Errorf("source %q explains nothing", src.factory)
		}
		if len(src.fields) == 0 {
			t.Errorf("source %q takes no fields", src.factory)
		}
		for _, f := range src.fields {
			if f.key == "" || f.label == "" {
				t.Errorf("source %q holds a field without a key or a label: %+v", src.factory, f)
			}
			if f.tip == "" {
				t.Errorf("field %q/%q explains nothing", src.factory, f.label)
			}
		}
	}
}
