package settings

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
)

// isolateConfig points os.UserConfigDir at a fresh temp directory.
//
// All three variables, os.UserConfigDir reading a different one per platform: XDG_CONFIG_HOME on
// Linux, AppData on Windows, HOME on macOS.
// Isolating one platform's is a test that writes the developer's own settings, presets and portal
// token on the others.
//
// The result is read back rather than assumed, the wrong variable failing silently and
// destructively.
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

// mustLoad fails the test on the reason instead of carrying it into an assertion about the values.
func mustLoad(t *testing.T) Settings {
	t.Helper()
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// mustSettingsPath is for a test seeding the file directly rather than through Save.
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
	s.Publish.SrtPublishLatencyMs = 0 // zero is the key a file written before the two hops leaves out
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

// A file written while the second track was one source carries that name under the old key.
// It becomes the one entry, so a stored stream keeps recording what it recorded.
func TestLoadMigratesTheOneAudioSourceOntoTheList(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.AudioSources = nil
	s.Publish.LegacyAudio = platform.AudioSourceDesktop
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if len(got.Publish.AudioSources) != 1 || got.Publish.AudioSources[0].Source != platform.AudioSourceDesktop {
		t.Fatalf("audio sources = %+v, want the one stored source", got.Publish.AudioSources)
	}
	if gain := got.Publish.AudioSources[0].Gain; gain != GainUnity {
		t.Errorf("the migrated source records at %d percent, want unity", gain)
	}
	// A file that has been through the migration no longer carries the old key, which is what keeps a
	// second run off a list that already holds the entry.
	if got.Publish.LegacyAudio != "" {
		t.Errorf("the migrated settings still carry the old key: %q", got.Publish.LegacyAudio)
	}
}

// A file written before the second track existed names no source, and gets the empty list a fresh
// installation has rather than an entry recording nothing.
func TestLoadMigratesAFileWithNoAudioAtAll(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.AudioSources = nil
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t); len(got.Publish.AudioSources) != 0 {
		t.Errorf("audio sources = %+v, want none", got.Publish.AudioSources)
	}
}

// A stream with the source off publishes no track whatever codec the file carries.
// Both engines validate through this one value, which keeps a stale codec from turning a silent
// stream into a refusal.
func TestAudioTrackFollowsTheSource(t *testing.T) {
	cases := []struct {
		source, audioCodec, want string
	}{
		{"desktop", "opus", "opus"},
		{"desktop", "aac", "aac"},
		{"none", "aac", capabilities.AudioNone},
		// A file written before the audio option names no source at all.
		{"", "opus", capabilities.AudioNone},
	}
	for _, tc := range cases {
		s := Defaults()
		s.Publish.AudioCodec = tc.audioCodec
		s.Publish.AudioSources = nil
		if tc.source != "" {
			s.Publish.AudioSources = Recording(tc.source)
		}
		if got := s.Publish.AudioTrack(); got != tc.want {
			t.Errorf("audio source %q with codec %q yields track %q, want %q",
				tc.source, tc.audioCodec, got, tc.want)
		}
	}
}

// A file written before the audio codec became a setting names none, and both engines refuse a
// track whose codec no row carries.
// The migration fills it with the codec those builds encoded, so a stored stream keeps publishing
// the track it had.
func TestLoadMigratesMissingAudioCodec(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.AudioSources, s.Publish.AudioCodec = Recording("desktop"), ""
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
	s.Viewer.PlayerWatchTransport = "" // empty is the key a file written before the option leaves out
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
	s.Viewer.RtspWatchLatencyMs = 0 // zero and empty are the keys such a file leaves out
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

// The builder matches both fields against a table, so a file written before either option has to
// arrive carrying a value that table names.
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

// A file written before the frame memory option names none, and both publish engines refuse a value
// their table does not carry.
// The table's own default is the one value every capture and codec pair satisfies, so migrating to
// it is what keeps an upgrade from turning a working stream into a refusal.
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

// The portal restore token is the compositor's receipt for one consent, and reusing it is what
// keeps the picker off every publish.
// Surviving a restart is the whole reason it is stored rather than held in the session.
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

	// An empty token is the compositor saying the consent was not persisted, so the one on disk is
	// spent and keeping it would send the next SelectSources a value no compositor honours.
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
	// Forgetting what is already gone is the state the caller named, so it succeeds.
	if err := ForgetPortalToken(); err != nil {
		t.Errorf("ForgetPortalToken on a machine with no token: %v", err)
	}
}

// The token is one machine's and one consent's, so a preset copied to another machine would carry
// a token no compositor there issued.
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

// The defaults come back with the reason beside them.
// The form then opens on values the user did not choose, which is a state to report rather than one
// to present as their settings.
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

// The next field change rewrites the working settings, so a corrupt file left in place is one that
// write destroys.
// The bytes have to survive it.
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

// A second failure is the file written after the first one, which holds the defaults.
// The copy from the first failure is the user's own data and outranks it.
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

// Encryption is derived from the address, so nothing about it is kept.
//
// A stored flag is a second copy of a fact the host already carries, and the two disagree the
// moment a host is edited: a "no" left beside a public name would send the picture in the clear on
// the strength of a file, and reset-to-defaults would write that "no" itself.
func TestEncryptionIsDerivedAndNeverStored(t *testing.T) {
	encoded, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatalf("rendering the settings: %v", err)
	}
	if strings.Contains(string(encoded), "tls") {
		t.Errorf("the stored settings carry a tls key: %s", encoded)
	}

	// A file written by an older build still has one, and it is read as the noise it now is.
	var restored Settings
	stored := `{"relay":{"host":"streamrelay.bjoernblessin.de","tls":false}}`
	if err := json.Unmarshal([]byte(stored), &restored); err != nil {
		t.Fatalf("reading settings an older build wrote: %v", err)
	}
	if !restored.Relay.Tls() {
		t.Error("a stored tls:false turned encryption off for a relay across the internet")
	}

	if Defaults().Relay.Tls() != true {
		t.Error("the defaults reach their relay unencrypted")
	}
}

// Which relays are reached in the clear, which is the whole of the question above.
// A name is encrypted whatever it resolves to: resolving it is a question this cannot ask, and the
// wrong guess is a stream on the wire for anyone to read.
func TestOnlyThisMachineAndThisNetworkAreReachedInTheClear(t *testing.T) {
	for host, want := range map[string]bool{
		"192.168.1.9":                  false,
		"10.0.0.5":                     false,
		"172.16.4.1":                   false,
		"127.0.0.1":                    false,
		"localhost":                    false,
		"169.254.7.7":                  false,
		"streamrelay.bjoernblessin.de": true,
		"relay.example":                true,
		"93.184.216.34":                true,
		"":                             false,
	} {
		if got := (Relay{Host: host}).Tls(); got != want {
			t.Errorf("relay %q is encrypted = %v, want %v", host, got, want)
		}
	}
}
