package gstbundle

import (
	"os"
	"path/filepath"
	"testing"
)

// The bundle leads whatever the environment names, and a directory already named is not named
// again: the value travels from this process to a child and to that child's child, each asking
// once more, so a second run over the answer of the first is the answer of the first.
func TestLeadingIsIdempotent(t *testing.T) {
	sep := string(os.PathListSeparator)
	dir := filepath.Join(string(filepath.Separator)+"app", Dir)
	other := filepath.Join(string(filepath.Separator)+"usr", "lib", "gstreamer-1.0")

	cases := []struct {
		name string
		set  string
		want string
	}{
		{name: "nothing set", set: "", want: dir},
		{name: "another directory set", set: other, want: dir + sep + other},
		{name: "already leading", set: dir + sep + other, want: dir + sep + other},
		{name: "already set alone", set: dir, want: dir},
		{name: "already on it, behind a value set on purpose", set: other + sep + dir, want: other + sep + dir},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := leading(dir, c.set)
			if got != c.want {
				t.Fatalf("leading(%q, %q) = %q, want %q", dir, c.set, got, c.want)
			}
			// The property the cases above spell out, checked once more on every answer.
			if again := leading(dir, got); again != got {
				t.Fatalf("a second leading over %q answered %q", got, again)
			}
		})
	}
}

// The two directories are named to a process through two variables, and a name of one where the
// other is expected sends a whole runtime looking in the wrong place.
func TestTheBundleDirectoriesAreNamedApart(t *testing.T) {
	if Dir == ModuleDir {
		t.Fatalf("the plugins and the GIO module share the directory %q", Dir)
	}
	if PathVar == ModuleVar {
		t.Fatalf("the plugins and the GIO module share the variable %q", PathVar)
	}
}
