package settings

import (
	"os"
	"testing"
)

// mustPresetsPath resolves the presets file for a test seeding one directly.
func mustPresetsPath(t *testing.T) string {
	t.Helper()
	path, err := presetsPath()
	if err != nil {
		t.Fatalf("presetsPath: %v", err)
	}
	return path
}

func TestPresetRoundTrip(t *testing.T) {
	isolateConfig(t)

	want := Defaults().Publish
	want.Name = "kept"
	if err := SavePreset("mine", want); err != nil {
		t.Fatalf("SavePreset: %v", err)
	}

	presets, err := LoadPresets()
	if err != nil {
		t.Fatalf("LoadPresets: %v", err)
	}
	if len(presets) != 1 || presets[0].Name != "mine" || presets[0].Settings.Name != "kept" {
		t.Errorf("presets = %+v, want one entry named mine", presets)
	}
}

func TestLoadPresetsMissingFileIsNotAFailure(t *testing.T) {
	isolateConfig(t)

	presets, err := LoadPresets()
	if err != nil {
		t.Fatalf("LoadPresets with no file: %v", err)
	}
	if len(presets) != 0 {
		t.Errorf("presets = %+v, want none", presets)
	}
}

// SavePreset and DeletePreset both rewrite the whole file from what LoadPresets
// returned. A corrupt file that read as an empty list would take every preset in
// it down with the next save, so the save is refused and the values are kept.
func TestSavePresetRefusesToWriteOverACorruptFile(t *testing.T) {
	isolateConfig(t)

	path := mustPresetsPath(t)
	const body = `[{"name":"mine","settings":{`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}

	if err := SavePreset("new", Defaults().Publish); err == nil {
		t.Error("a preset was saved into a file that failed to parse")
	}

	kept, err := os.ReadFile(path + corruptSuffix)
	if err != nil {
		t.Fatalf("the corrupt file was not kept: %v", err)
	}
	if string(kept) != body {
		t.Errorf("kept copy = %q, want the bytes that failed to parse", kept)
	}
}

func TestDeletePresetRefusesToWriteOverACorruptFile(t *testing.T) {
	isolateConfig(t)

	path := mustPresetsPath(t)
	if err := os.WriteFile(path, []byte("[ not valid json"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}

	if err := DeletePreset("mine"); err == nil {
		t.Error("a delete rewrote a file that failed to parse")
	}
}

// The set-aside leaves no file behind, so the same action repeated writes a fresh
// one rather than staying refused.
func TestSavePresetSucceedsAfterTheCorruptFileIsKept(t *testing.T) {
	isolateConfig(t)

	path := mustPresetsPath(t)
	if err := os.WriteFile(path, []byte("[ not valid json"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}

	if err := SavePreset("new", Defaults().Publish); err == nil {
		t.Fatal("a preset was saved into a file that failed to parse")
	}
	if err := SavePreset("new", Defaults().Publish); err != nil {
		t.Fatalf("the repeated save stayed refused: %v", err)
	}

	presets, err := LoadPresets()
	if err != nil {
		t.Fatalf("LoadPresets: %v", err)
	}
	if len(presets) != 1 || presets[0].Name != "new" {
		t.Errorf("presets = %+v, want one entry named new", presets)
	}
}
