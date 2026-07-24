package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

const presetsFileName = "presets.json"

// Preset is a named snapshot of stream settings the user saved for reuse.
// Presets are independent of the last-used settings that Load/Save persist:
// deleting a preset never touches the working settings, and the working
// settings restored on launch need not correspond to any preset.
type Preset struct {
	Name     string `json:"name"`
	Settings Stream `json:"settings"`
}

func presetsPath() string {
	return filepath.Join(configDir(), presetsFileName)
}

// LoadPresets reads the saved presets in the order they were stored.
// A missing or corrupt file yields no presets rather than an error.
func LoadPresets() []Preset {
	data, err := os.ReadFile(presetsPath())
	if err != nil {
		return nil
	}

	var presets []Preset
	err = json.Unmarshal(data, &presets)
	if err != nil {
		logger.Warnf("Presets file is corrupt, ignoring: %v", err)
		return nil
	}

	// Presets saved by an older build carry the old mode names and lack the
	// fields added since; upgrade each the same way Load upgrades the working set.
	for i := range presets {
		presets[i].Settings = migrateStream(presets[i].Settings)
	}
	return presets
}

func savePresets(presets []Preset) error {
	data, err := json.MarshalIndent(presets, "", "  ")
	assert.IsNil(err, "marshalling presets cannot fail")

	return os.WriteFile(presetsPath(), data, 0o644)
}

// SavePreset stores s under name, replacing any existing preset with that name.
func SavePreset(name string, s Stream) error {
	presets := LoadPresets()

	for i := range presets {
		if presets[i].Name == name {
			presets[i].Settings = s
			return savePresets(presets)
		}
	}

	return savePresets(append(presets, Preset{Name: name, Settings: s}))
}

// DeletePreset removes the preset named name. A missing name is not an error.
func DeletePreset(name string) error {
	presets := LoadPresets()

	kept := presets[:0]
	for _, p := range presets {
		if p.Name != name {
			kept = append(kept, p)
		}
	}

	return savePresets(kept)
}
