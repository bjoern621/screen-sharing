package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

const presetsFileName = "presets.json"

// presetsMu serializes the read-modify-write that saving and deleting both are.
//
// Each reads the whole file, changes one entry and writes all of it back, so two running at once
// lose an edit: the second read happens before the first write and the second write then carries a
// list that never saw the first change.
// Reachable rather than theoretical, since every call arrives on a goroutine of its own and more
// than one shell can be connected at a time.
var presetsMu sync.Mutex

// Preset is a named way of publishing, saved for reuse.
// A Publish group and nothing else: where the relay sits belongs to a deployment and how this
// machine watches belongs to a viewer, and neither is part of what a saved configuration means.
//
// Presets stand apart from the working settings Load and Save persist.
// A delete never touches the working settings, and the settings restored on launch answer to no
// preset.
type Preset struct {
	Name     string  `json:"name"`
	Settings Publish `json:"settings"`
}

func presetsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, presetsFileName), nil
}

// LoadPresets reads the saved presets in the order they were stored, and answers none and a reason
// where the file cannot be used.
// A missing file is no failure: nothing has been saved yet.
//
// An unusable file is an Umgebungsfehler and is moved aside (setAside), for the reason SavePreset
// and DeletePreset name.
// Both rewrite the whole file from what this returns, so answering an unreadable file with an empty
// list would let the next save replace every preset in it.
func LoadPresets() ([]Preset, error) {
	path, err := presetsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, setAside(path, fmt.Errorf("cannot read presets file %s: %w", path, err))
	}

	var presets []Preset
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, setAside(path, fmt.Errorf("presets file %s is corrupt: %w", path, err))
	}

	// A preset saved by an older build carries the old mode names and lacks the keys added since.
	// Each is upgraded the way Load upgrades the working settings.
	for i := range presets {
		presets[i].Settings = migratePublish(presets[i].Settings, Defaults().Publish)
	}
	return presets, nil
}

func savePresets(presets []Preset) error {
	data, err := json.MarshalIndent(presets, "", "  ")
	assert.IsNil(err, "marshalling presets cannot fail")

	path, err := presetsPath()
	if err != nil {
		return err
	}
	return writeStore(path, data)
}

// SavePreset stores s under name, replacing any preset already saved under it.
//
// A presets file that could not be read is reported rather than written over.
// The whole file is rewritten from what LoadPresets returned, so saving into a file that read as
// empty would replace every preset in it with this one entry.
// The unreadable file has been moved aside by then, so the same save repeated writes a fresh file
// and the reason names where the old values went.
func SavePreset(name string, s Publish) error {
	assert.Assert(name != "", "a saved preset is saved under a name")

	presetsMu.Lock()
	defer presetsMu.Unlock()

	presets, err := LoadPresets()
	if err != nil {
		return err
	}

	for i := range presets {
		if presets[i].Name == name {
			presets[i].Settings = s
			return savePresets(presets)
		}
	}

	return savePresets(append(presets, Preset{Name: name, Settings: s}))
}

// DeletePreset removes the preset saved under name.
// A name no preset carries is a success, which is what makes a repeated delete one too.
// An unreadable file is an error, for the reason SavePreset gives.
func DeletePreset(name string) error {
	assert.Assert(name != "", "a deleted preset is named")

	presetsMu.Lock()
	defer presetsMu.Unlock()

	presets, err := LoadPresets()
	if err != nil {
		return err
	}

	kept := presets[:0]
	for _, p := range presets {
		if p.Name != name {
			kept = append(kept, p)
		}
	}

	return savePresets(kept)
}
