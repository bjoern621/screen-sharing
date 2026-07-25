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

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	isolateConfig(t)

	if got := Load(); !reflect.DeepEqual(got, Defaults()) {
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
	if got := Load(); !reflect.DeepEqual(got, want) {
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

	got := Load()
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

	if got := Load(); got.Audio != "none" {
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

	if got := Load(); got.WatchTransport != Defaults().WatchTransport {
		t.Errorf("watch transport = %q, want migrated to %q", got.WatchTransport, Defaults().WatchTransport)
	}
}

func TestLoadCorruptFileReturnsDefaults(t *testing.T) {
	isolateConfig(t)

	if err := os.WriteFile(configPath(), []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}

	if got := Load(); !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load() on corrupt file = %+v, want Defaults()", got)
	}
}
