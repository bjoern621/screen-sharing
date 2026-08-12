package settings

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
)

// isolateConfig points os.UserConfigDir at a fresh temp directory so tests never
// read or clobber the real settings file.
//
// Every platform's variable is set, because os.UserConfigDir reads a different one
// on each and a test that isolates only one platform's is a test that writes the
// developer's own settings, presets and portal token on the others: XDG_CONFIG_HOME
// on Linux, AppData on Windows, HOME on macOS.
//
// The result is asserted rather than assumed. Setting the wrong variable fails
// silently and destructively, so the one thing this helper must not do is return
// while os.UserConfigDir still answers with the real directory.
func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("HOME", dir)

	got, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("isolating the config directory: %v", err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("os.UserConfigDir is %s, outside the temp directory %s: this test would read and overwrite the real settings store", got, dir)
	}
}

// mustLoad loads the settings and fails the test on the reason instead of
// carrying it into an assertion about the values.
func mustLoad(t *testing.T) Settings {
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
	want.Publish.Name = "roundtrip"
	want.Relay.Host = "relay.example"
	want.Publish.BitrateM = 80
	want.Publish.Fps = 144

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
	s.Publish.SrtPublishLatencyMs = 0 // a pre-split settings file lacks these keys
	s.Viewer.SrtWatchLatencyMs = 0
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.Publish.SrtPublishLatencyMs != Defaults().Publish.SrtPublishLatencyMs {
		t.Errorf("publish latency = %d, want migrated to %d", got.Publish.SrtPublishLatencyMs, Defaults().Publish.SrtPublishLatencyMs)
	}
	if got.Viewer.SrtWatchLatencyMs != Defaults().Viewer.SrtWatchLatencyMs {
		t.Errorf("watch latency = %d, want migrated to %d", got.Viewer.SrtWatchLatencyMs, Defaults().Viewer.SrtWatchLatencyMs)
	}
}

func TestLoadMigratesMissingAudio(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.Audio = "" // a pre-audio settings file lacks the key
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t); got.Publish.Audio != "none" {
		t.Errorf("audio = %q, want migrated to \"none\"", got.Publish.Audio)
	}
}

// The audio codec is read only where the source names one, so a stream with the source off
// publishes no track whatever codec the file carries.
// Both engines validate through this one value, which keeps "no track" from being a branch
// each of them takes on its own, and a stale codec from turning a silent stream into a refusal.
func TestAudioTrackFollowsTheSource(t *testing.T) {
	cases := []struct {
		source, audioCodec, want string
	}{
		{"desktop", "opus", "opus"},
		{"desktop", "aac", "aac"},
		{"none", "aac", capabilities.AudioNone},
		// A settings file written before the audio option names no source at all.
		{"", "opus", capabilities.AudioNone},
	}
	for _, tc := range cases {
		s := Defaults()
		s.Publish.Audio, s.Publish.AudioCodec = tc.source, tc.audioCodec
		if got := s.Publish.AudioTrack(); got != tc.want {
			t.Errorf("audio source %q with codec %q yields track %q, want %q",
				tc.source, tc.audioCodec, got, tc.want)
		}
	}
}

// A file written before the audio codec became a setting names none, and both engines
// refuse a track whose codec no row carries.
// The migration fills it with the codec those builds encoded, so a stored stream keeps
// publishing the track it always did rather than starting on one the file never chose.
func TestLoadMigratesMissingAudioCodec(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.Audio, s.Publish.AudioCodec = "desktop", ""
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.Publish.AudioCodec != defaultAudioCodec {
		t.Errorf("audio codec = %q, want migrated to %q", got.Publish.AudioCodec, defaultAudioCodec)
	}
	if _, ok := capabilities.GetAudio(got.Publish.AudioTrack()); !ok {
		t.Errorf("the migrated track %q is not a row of the audio table", got.Publish.AudioTrack())
	}
}

func TestLoadMigratesMissingWatchTransport(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Viewer.PlayerWatchTransport = "" // a pre-watch-transport settings file lacks the key
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t); got.Viewer.PlayerWatchTransport != Defaults().Viewer.PlayerWatchTransport {
		t.Errorf("watch transport = %q, want migrated to %q", got.Viewer.PlayerWatchTransport, Defaults().Viewer.PlayerWatchTransport)
	}
}

func TestLoadMigratesMissingRtspWatchKnobs(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Viewer.RtspWatchLatencyMs = 0 // a pre-RTSP-knobs settings file lacks these keys
	s.Viewer.RtspWatchProtocol = ""
	s.Publish.RtspPublishProtocol = ""
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.Viewer.RtspWatchLatencyMs != Defaults().Viewer.RtspWatchLatencyMs {
		t.Errorf("rtsp watch latency = %d, want migrated to %d", got.Viewer.RtspWatchLatencyMs, Defaults().Viewer.RtspWatchLatencyMs)
	}
	if got.Viewer.RtspWatchProtocol != Defaults().Viewer.RtspWatchProtocol {
		t.Errorf("rtsp watch protocol = %q, want migrated to %q", got.Viewer.RtspWatchProtocol, Defaults().Viewer.RtspWatchProtocol)
	}
	if got.Publish.RtspPublishProtocol != Defaults().Publish.RtspPublishProtocol {
		t.Errorf("rtsp publish protocol = %q, want migrated to %q", got.Publish.RtspPublishProtocol, Defaults().Publish.RtspPublishProtocol)
	}
}

// Both fields are matched against a table by the builder that reads them, so a
// file written before the option existed has to arrive carrying a value that
// table names.
func TestLoadMigratesMissingDrmMapAndPreset(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.DrmMap = ""
	s.Publish.Effort = ""
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.Publish.DrmMap != Defaults().Publish.DrmMap {
		t.Errorf("drm map = %q, want migrated to %q", got.Publish.DrmMap, Defaults().Publish.DrmMap)
	}
	if got.Publish.Effort != Defaults().Publish.Effort {
		t.Errorf("encoder preset = %q, want migrated to %q", got.Publish.Effort, Defaults().Publish.Effort)
	}
}

// A file written before the frame memory option names none, and both publish engines
// refuse a value their table does not carry. Migrating it to the table's own default
// is what keeps an upgrade from turning a working stream into a refusal, and the
// default is the one value every capture and codec pair satisfies.
func TestLoadMigratesMissingCaptureMemory(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.CaptureMemory = ""
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.Publish.CaptureMemory != gpupath.MemoryAuto {
		t.Errorf("frame memory = %q, want migrated to %q", got.Publish.CaptureMemory, gpupath.MemoryAuto)
	}
}

// The portal restore token is the compositor's receipt for one consent, and reusing it
// is what keeps the picker from popping on every publish. It survives a restart, which
// is the whole reason it is stored rather than held in the session.
func TestPortalTokenSurvivesAReload(t *testing.T) {
	isolateConfig(t)

	if got := PortalToken(); got != "" {
		t.Errorf("a machine that has never consented holds no token, got %q", got)
	}
	if err := SavePortalToken("consent-1"); err != nil {
		t.Fatalf("SavePortalToken: %v", err)
	}
	if got := PortalToken(); got != "consent-1" {
		t.Errorf("token = %q, want the stored one", got)
	}

	// An empty token is the compositor saying the consent was not persisted, so the one
	// on disk is spent. Keeping it would send the next session to SelectSources with a
	// value no compositor will honour.
	if err := SavePortalToken(""); err != nil {
		t.Fatalf("SavePortalToken: %v", err)
	}
	if got := PortalToken(); got != "" {
		t.Errorf("an unpersisted consent must not leave the old token in place, got %q", got)
	}

	if err := SavePortalToken("consent-2"); err != nil {
		t.Fatalf("SavePortalToken: %v", err)
	}
	if err := ForgetPortalToken(); err != nil {
		t.Fatalf("ForgetPortalToken: %v", err)
	}
	if got := PortalToken(); got != "" {
		t.Errorf("a forgotten consent leaves no token, got %q", got)
	}
	// Forgetting what is already gone is the state the caller asked for.
	if err := ForgetPortalToken(); err != nil {
		t.Errorf("ForgetPortalToken on a machine with no token: %v", err)
	}
}

// The token is machine- and consent-local, so it must not ride along in a preset: a
// preset copied to another machine would carry a token no compositor there issued.
func TestPresetsCarryNoPortalToken(t *testing.T) {
	isolateConfig(t)

	if err := SavePortalToken("consent-1"); err != nil {
		t.Fatalf("SavePortalToken: %v", err)
	}
	if err := SavePreset("work", Defaults().Publish); err != nil {
		t.Fatalf("SavePreset: %v", err)
	}
	path, err := presetsPath()
	if err != nil {
		t.Fatalf("presetsPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read presets: %v", err)
	}
	if strings.Contains(string(data), "consent-1") {
		t.Errorf("the presets file carries the portal consent: %s", data)
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
