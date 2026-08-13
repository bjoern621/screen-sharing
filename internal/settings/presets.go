package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"
)

const presetsFileName = "presets.json"

// Preset is a named way of publishing the user saved for reuse.
// It is a Publish group and nothing else: where the relay is belongs to a deployment and how this
// machine watches belongs to a viewer, and neither is part of what a saved configuration means.
//
// Presets are independent of the last-used settings that Load and Save persist.
// Deleting a preset never touches the working settings, and the working settings restored on launch
// need not correspond to any preset.
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

// LoadPresets reads the saved presets in the order they were stored, and answers with none and the
// reason when the file cannot be used.
// A missing file is not a failure: nothing has been saved yet.
//
// An unusable file is moved aside (setAside) for the reason SavePreset and DeletePreset name.
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

	// Presets saved by an older build carry the old mode names and lack the fields added since.
	// Each is upgraded the way Load upgrades the working set.
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
	return os.WriteFile(path, data, storeFileMode)
}

// SavePreset stores s under name, replacing any existing preset with that name.
//
// A presets file that could not be read is reported instead of written over.
// The whole file is rewritten from what LoadPresets returned, so saving into a file that read as
// empty would replace every preset in it with this one entry.
// The unreadable file has been moved aside by then, so the same save repeated writes a fresh file
// and the reason names where the old values went.
func SavePreset(name string, s Publish) error {
	assert.Assert(name != "", "a saved preset is saved under a name")

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

// DeletePreset removes the preset named name.
// A missing name is not an error, which is what makes a repeated delete succeed;
// an unreadable file is, for the reason SavePreset gives.
func DeletePreset(name string) error {
	assert.Assert(name != "", "a deleted preset is named")

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
