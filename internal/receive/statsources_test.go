package receive

import "testing"

// TestStatSourcesAreWellFormed holds the table's invariants.
// A source that names no factory matches no element, and one that takes no field would report an
// empty group for every pipeline holding that element.
func TestStatSourcesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, src := range statSources {
		if src.factory == "" {
			t.Error("a stat source names no factory")
		}
		if seen[src.factory] {
			t.Errorf("factory %q is named by two sources", src.factory)
		}
		seen[src.factory] = true

		if len(src.fields) == 0 {
			t.Errorf("source %q takes no fields", src.factory)
		}
		keys := map[string]bool{}
		for _, key := range src.fields {
			if key == "" {
				t.Errorf("source %q holds a field with no key", src.factory)
			}
			if keys[key] {
				t.Errorf("source %q takes field %q twice", src.factory, key)
			}
			keys[key] = true
		}
	}
}

// TestStatValueReadsTheTypesElementsCount guards the one narrowing this file does.
// A counter arrives as whatever GValue the element chose, so a type statValue leaves out is a row
// that silently never appears.
func TestStatValueReadsTheTypesElementsCount(t *testing.T) {
	counted := []any{float64(1), float32(1), int(1), int32(1), int64(1), uint32(1), uint64(1)}
	for _, v := range counted {
		if got, ok := statValue(v); !ok || got != 1 {
			t.Errorf("%T read as (%v, %v), want (1, true)", v, got, ok)
		}
	}

	if _, ok := statValue("42"); ok {
		t.Error("a string read as a figure")
	}
}
