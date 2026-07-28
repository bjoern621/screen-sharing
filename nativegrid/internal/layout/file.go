package layout

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// dirName and fileName put the remembered state beside the app's settings.json,
// the directory os.UserConfigDir resolves for both processes (%APPDATA% on
// Windows, XDG_CONFIG_HOME or ~/.config on Linux). The directory name is
// repeated from desktop/settings rather than shared because no Go symbol
// crosses the module boundary.
const (
	dirName  = "screenshare"
	fileName = "nativegrid.json"

	dirMode  = 0o755
	fileMode = 0o644
)

// FileStore keeps both remembered records in one JSON file in the app's config
// directory.
//
// The file has two owners: internal/session writes the arrangement (Layout), the
// window writes its geometry (WindowState). Both write from the UI thread, so no
// two writes interleave, but each owner holds only its own record and would drop
// the other's keys by writing the object it knows.
//
// Every write is therefore a read-modify-write of the JSON object: the keys the
// written record declares are replaced, every other key is carried over as it
// was found. No value is invented for a key the writer does not own, and a key
// the writer does own but its record omits is removed rather than left at what a
// previous write put there. A file that cannot be parsed carries nothing over,
// which is the case a missing file is in too.
type FileStore struct {
	path string
}

// NewFileStore resolves the state file's path and creates its directory. A
// config directory that cannot be resolved falls back to the working directory,
// the same fallback the app's settings package takes.
func NewFileStore() *FileStore {
	base, err := os.UserConfigDir()
	if err != nil {
		logger.Warnf("no user config directory, keeping the grid state in the working directory: %v", err)
		base = "."
	}
	dir := filepath.Join(base, dirName)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		logger.Warnf("cannot create %s: %v", dir, err)
	}
	return &FileStore{path: filepath.Join(dir, fileName)}
}

// Load reads the remembered arrangement. A missing file yields the zero Layout,
// which is a first run: nothing watched, no remembered order.
func (f *FileStore) Load() Layout { return load[Layout](f) }

// LoadWindow reads the remembered geometry. A missing file yields the zero
// WindowState, which Remembered reports as the first run it is.
func (f *FileStore) LoadWindow() WindowState { return load[WindowState](f) }

// Save writes the arrangement and leaves the window's keys as they were.
func (f *FileStore) Save(l Layout) {
	assertLayout(l)

	f.save(l)
}

// SaveWindow writes the geometry and leaves the arrangement's keys as they were.
func (f *FileStore) SaveWindow(w WindowState) {
	assert.Assert(w.Width >= 0 && w.Height >= 0, "a remembered window carries a size or none", w.Width, w.Height)

	f.save(w)
}

// assertLayout holds what an arrangement has to be for the next run to open on
// it: streams are keyed by name, the watch set names streams the order ranks,
// and so does the spotlight. A record that breaks any of it puts a stream on
// screen the next run cannot place, one run after the write that caused it.
func assertLayout(l Layout) {
	ranked := make(map[string]bool, len(l.Order))
	for _, n := range l.Order {
		assert.Assert(n != "", "a remembered stream carries a name")
		assert.Assert(!ranked[n], "the remembered order holds a stream once", n)

		ranked[n] = true
	}
	watched := make(map[string]bool, len(l.Watched))
	for _, n := range l.Watched {
		assert.Assert(ranked[n], "a remembered watch is on a stream the order ranks", n)
		assert.Assert(!watched[n], "the remembered watch set holds a stream once", n)

		watched[n] = true
	}
	assert.Assert(l.Spot == "" || ranked[l.Spot], "a remembered spotlight is on a stream the order ranks", l.Spot)
}

// load reads one record out of the state file. Keys the record does not declare
// belong to the other owner and are ignored. A corrupt file is reported and
// yields the zero record, so a bad file cannot keep the window from opening.
func load[T any](f *FileStore) T {
	assert.Assert(f.path != "", "a file store resolves its path at construction")

	var record T
	data, err := os.ReadFile(f.path)
	if err != nil {
		logger.Debugf("no grid state at %s: %v", f.path, err)
		return record
	}
	if err := json.Unmarshal(data, &record); err != nil {
		logger.Warnf("grid state at %s is corrupt, starting fresh: %v", f.path, err)
		// Unmarshal keeps what it read before it failed, so the half-filled record
		// is dropped for the zero one a first run gets.
		var fresh T
		return fresh
	}
	logger.Debugf("%T restored from %s", record, f.path)
	return record
}

// save replaces the keys the record owns and writes the file back. A failed
// write is reported rather than propagated.
func (f *FileStore) save(record any) {
	assert.Assert(f.path != "", "a file store resolves its path at construction")

	own, err := object(record)
	if err != nil {
		logger.Warnf("cannot encode the grid state: %v", err)
		return
	}
	file := f.read()
	// The owned keys go before the copy, not after it: a key the record leaves out
	// this time is absent from own too, and a copy over the top would keep what the
	// last write put there. That is a dropped spotlight coming back.
	for _, k := range keysOf(record) {
		delete(file, k)
	}
	maps.Copy(file, own)

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		logger.Warnf("cannot encode the grid state: %v", err)
		return
	}
	if err := os.WriteFile(f.path, data, fileMode); err != nil {
		logger.Warnf("cannot write %s: %v", f.path, err)
	}
}

// read is the state file as the raw JSON object a write merges into, keys of
// both owners and of no owner alike. A file that is missing or cannot be parsed
// yields an empty object: there is nothing to carry over, and a writer holding
// its own keys back would not bring the other owner's back either.
func (f *FileStore) read() map[string]json.RawMessage {
	file := map[string]json.RawMessage{}
	data, err := os.ReadFile(f.path)
	if err != nil {
		logger.Debugf("no grid state at %s to merge into: %v", f.path, err)
		return file
	}
	if err := json.Unmarshal(data, &file); err != nil {
		logger.Warnf("grid state at %s is corrupt, writing it fresh: %v", f.path, err)
		return map[string]json.RawMessage{}
	}
	return file
}

// object is one record as the JSON keys it writes, which is what a merge works
// on.
func object(record any) (map[string]json.RawMessage, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// keysOf is the JSON key set a record type owns: the keys its writer replaces,
// and the only ones it may touch. It is read off the struct tags rather than
// listed beside them, so a renamed key cannot leave the one it replaced behind
// in the file.
func keysOf(record any) []string {
	t := reflect.TypeOf(record)
	assert.Assert(t.Kind() == reflect.Struct, "a remembered record is a struct", t.Kind().String())

	keys := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		assert.Assert(name != "" && name != "-", "a remembered field names its json key", t.Field(i).Name)

		keys = append(keys, name)
	}
	return keys
}
