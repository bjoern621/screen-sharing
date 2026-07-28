package settings

import (
	"os"
	"reflect"
	"testing"
)

// isolateConfig points os.UserConfigDir at a fresh temp directory so tests
// never read or clobber the real settings file. Effective on Linux, where
// os.UserConfigDir honors XDG_CONFIG_HOME.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// mustLoad loads the settings and fails the test on the reason instead of
// carrying it into an assertion about the values.
func mustLoad(t *testing.T) Stream {
	t.Helper()
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// mustSettingsPath resolves the settings file for a test seeding one directly.
func mustSettingsPath(t *testing.T) string {
	t.Helper()
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	return path
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	isolateConfig(t)

	if got := mustLoad(t); !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load() with no file = %+v, want Defaults()", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	isolateConfig(t)

	want := Defaults()
	want.Name = "roundtrip"
	want.RelayHost = "relay.example"
	want.BitrateM = 80
	want.Fps = 144

	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := mustLoad(t); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestLoadMigratesZeroLatency(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.SrtPublishLatencyMs = 0 // a pre-split settings file lacks these keys
	s.SrtWatchLatencyMs = 0
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.SrtPublishLatencyMs != Defaults().SrtPublishLatencyMs {
		t.Errorf("publish latency = %d, want migrated to %d", got.SrtPublishLatencyMs, Defaults().SrtPublishLatencyMs)
	}
	if got.SrtWatchLatencyMs != Defaults().SrtWatchLatencyMs {
		t.Errorf("watch latency = %d, want migrated to %d", got.SrtWatchLatencyMs, Defaults().SrtWatchLatencyMs)
	}
}

func TestLoadMigratesMissingAudio(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Audio = "" // a pre-audio settings file lacks the key
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t); got.Audio != "none" {
		t.Errorf("audio = %q, want migrated to \"none\"", got.Audio)
	}
}

func TestLoadMigratesMissingWatchTransport(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.WatchTransport = "" // a pre-watch-transport settings file lacks the key
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t); got.WatchTransport != Defaults().WatchTransport {
		t.Errorf("watch transport = %q, want migrated to %q", got.WatchTransport, Defaults().WatchTransport)
	}
}

func TestLoadMigratesMissingRtspWatchKnobs(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.RtspWatchLatencyMs = 0 // a pre-RTSP-knobs settings file lacks these keys
	s.RtspWatchProtocol = ""
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.RtspWatchLatencyMs != Defaults().RtspWatchLatencyMs {
		t.Errorf("rtsp watch latency = %d, want migrated to %d", got.RtspWatchLatencyMs, Defaults().RtspWatchLatencyMs)
	}
	if got.RtspWatchProtocol != Defaults().RtspWatchProtocol {
		t.Errorf("rtsp watch protocol = %q, want migrated to %q", got.RtspWatchProtocol, Defaults().RtspWatchProtocol)
	}
}

// Both fields are matched against a table by the builder that reads them, so a
// file written before the option existed has to arrive carrying a value that
// table names.
func TestLoadMigratesMissingDrmMapAndPreset(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.DrmMap = ""
	s.EncPreset = ""
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.DrmMap != Defaults().DrmMap {
		t.Errorf("drm map = %q, want migrated to %q", got.DrmMap, Defaults().DrmMap)
	}
	if got.EncPreset != Defaults().EncPreset {
		t.Errorf("encoder preset = %q, want migrated to %q", got.EncPreset, Defaults().EncPreset)
	}
}

// A corrupt file yields the defaults, and yields the reason with it: the form
// opens on values the user did not choose, which is a state to report rather than
// one to present as their settings.
func TestLoadCorruptFileReturnsDefaultsAndReason(t *testing.T) {
	isolateConfig(t)

	path := mustSettingsPath(t)
	if err := os.WriteFile(path, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}

	got, err := Load()
	if err == nil {
		t.Error("a corrupt settings file loaded without a reason")
	}
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load() on corrupt file = %+v, want Defaults()", got)
	}
}

// The working settings are rewritten on the next field change, so a corrupt file
// left in place is a file that write destroys. The values have to survive it.
func TestLoadKeepsACorruptFileOutOfSavesReach(t *testing.T) {
	isolateConfig(t)

	path := mustSettingsPath(t)
	const body = `{"name":"before the corruption`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("a corrupt settings file loaded without a reason")
	}
	if err := Save(Defaults()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	kept, err := os.ReadFile(path + corruptSuffix)
	if err != nil {
		t.Fatalf("the corrupt file was not kept: %v", err)
	}
	if string(kept) != body {
		t.Errorf("kept copy = %q, want the bytes that failed to parse", kept)
	}
}

// A second failure means the file written after the first one is corrupt too. The
// copy from the first failure is the user's own data and outranks it.
func TestLoadKeepsTheFirstCorruptCopy(t *testing.T) {
	isolateConfig(t)

	path := mustSettingsPath(t)
	const first = `{"name":"the real settings`
	if err := os.WriteFile(path+corruptSuffix, []byte(first), 0o644); err != nil {
		t.Fatalf("seed kept copy: %v", err)
	}
	if err := os.WriteFile(path, []byte("{ also broken"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("a corrupt settings file loaded without a reason")
	}

	kept, err := os.ReadFile(path + corruptSuffix)
	if err != nil {
		t.Fatalf("read kept copy: %v", err)
	}
	if string(kept) != first {
		t.Errorf("kept copy = %q, want the first failure's bytes", kept)
	}
}
