package layout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// store is a FileStore on a throwaway path: the state file the arrangement and
// the geometry share.
func store(t *testing.T) *FileStore {
	t.Helper()
	return &FileStore{path: filepath.Join(t.TempDir(), fileName)}
}

// raw is the state file as JSON keys, which is what a merge has to get right.
func raw(t *testing.T, f *FileStore) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatalf("reading %s: %v", f.path, err)
	}
	file := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("%s is not a JSON object: %v", f.path, err)
	}
	return file
}

// The two owners have to claim different keys, or a merge that replaces one
// record's keys silently rewrites the other's.
func TestRecordsOwnDisjointKeys(t *testing.T) {
	window := keysOf(WindowState{})
	for _, k := range keysOf(Layout{}) {
		if slices.Contains(window, k) {
			t.Errorf("%q is claimed by both owners of the state file", k)
		}
	}
}

func TestFileStoreReadsAFirstRun(t *testing.T) {
	f := store(t)
	if got := f.Load(); !reflect.DeepEqual(got, Layout{}) {
		t.Errorf("Load() on a missing file = %+v, want the zero Layout", got)
	}
	if got := f.LoadWindow(); got.Remembered() {
		t.Errorf("LoadWindow() on a missing file = %+v, want nothing remembered", got)
	}
}

func TestFileStoreKeepsBothOwners(t *testing.T) {
	f := store(t)
	arrangement := Layout{Order: []string{"a", "b"}, Watched: []string{"a"}, Spot: "a"}
	geometry := WindowState{Width: 800, Height: 600, Maximized: true, SidebarShown: true}

	// Each owner writes last once, so neither order of the two can drop the
	// other's record.
	f.Save(arrangement)
	f.SaveWindow(geometry)
	if got := f.Load(); !reflect.DeepEqual(got, arrangement) {
		t.Errorf("the geometry write cost the arrangement: Load() = %+v, want %+v", got, arrangement)
	}

	arrangement.Spot = "b"
	f.Save(arrangement)
	if got := f.LoadWindow(); got != geometry {
		t.Errorf("the arrangement write cost the geometry: LoadWindow() = %+v, want %+v", got, geometry)
	}
	if got := f.Load(); !reflect.DeepEqual(got, arrangement) {
		t.Errorf("Load() = %+v, want %+v", got, arrangement)
	}
}

// A key neither owner declares is a key neither owner may take away: an older
// binary sharing the file with a newer one has to leave what it cannot read.
func TestFileStoreLeavesForeignKeysAlone(t *testing.T) {
	f := store(t)
	if err := os.WriteFile(f.path, []byte(`{"fromALaterVersion": 3}`), fileMode); err != nil {
		t.Fatalf("seeding %s: %v", f.path, err)
	}
	f.Save(Layout{Order: []string{"a"}})
	f.SaveWindow(WindowState{Width: 640, Height: 480})

	if _, ok := raw(t, f)["fromALaterVersion"]; !ok {
		t.Error("a key of neither owner was dropped by a write")
	}
}

// An omitted key is written as absent rather than left at what the last write
// put there: dropping the spotlight has to reach the file.
func TestFileStoreDropsAnOmittedKey(t *testing.T) {
	f := store(t)
	f.Save(Layout{Order: []string{"a"}, Spot: "a"})
	f.Save(Layout{Order: []string{"a"}})

	if spot, ok := raw(t, f)["spot"]; ok {
		t.Errorf("a dropped spotlight stayed in the file as %s", spot)
	}
	if got := f.Load(); got.Spot != "" {
		t.Errorf("Load().Spot = %q, want the spotlight dropped", got.Spot)
	}
}

// A file that cannot be parsed carries nothing over, and the write that follows
// it is a fresh file rather than a lost one.
func TestFileStoreStartsFreshOnACorruptFile(t *testing.T) {
	f := store(t)
	if err := os.WriteFile(f.path, []byte("{not json"), fileMode); err != nil {
		t.Fatalf("seeding %s: %v", f.path, err)
	}
	if got := f.Load(); !reflect.DeepEqual(got, Layout{}) {
		t.Errorf("Load() on a corrupt file = %+v, want the zero Layout", got)
	}

	geometry := WindowState{Width: 640, Height: 480, SidebarShown: true}
	f.SaveWindow(geometry)
	if got := f.LoadWindow(); got != geometry {
		t.Errorf("LoadWindow() after a corrupt file = %+v, want %+v", got, geometry)
	}
}

func TestWindowStateRemembered(t *testing.T) {
	cases := []struct {
		name  string
		state WindowState
		want  bool
	}{
		{"a run that wrote nothing", WindowState{}, false},
		{"a record without a height", WindowState{Width: 800}, false},
		{"a size to open on", WindowState{Width: 800, Height: 600}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.state.Remembered(); got != c.want {
				t.Errorf("%+v.Remembered() = %t, want %t", c.state, got, c.want)
			}
		})
	}
}
