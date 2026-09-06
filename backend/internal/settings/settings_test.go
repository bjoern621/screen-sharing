package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/platform"
)

// isolateConfig points os.UserConfigDir at a fresh temp directory.
//
// All three variables, os.UserConfigDir reading a different one per platform:
// XDG_CONFIG_HOME on Linux, AppData on Windows, HOME on macOS.
// Isolating one platform's is a test that writes the developer's own settings,
// presets and portal token on the others.
//
// The result is read back rather than assumed,
// the wrong variable failing silently and destructively.
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
	want.Publish.Monitor = 3
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
	s.Publish.FlatAudio = platform.AudioSourceDesktop
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
	// A file that has been through the migration carries no old key,
	// which is what keeps a second run off a list that already holds the entry.
	if got.Publish.FlatAudio != "" {
		t.Errorf("the migrated settings still carry the old key: %q", got.Publish.FlatAudio)
	}
}

// A file written before the second track existed names no source,
// and gets the empty list a fresh installation has rather than an entry recording nothing.
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
// Both engines validate through this one value,
// which keeps a stale codec from turning a silent stream into a refusal.
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

// A file written before the audio codec became a setting names none,
// and both engines refuse a track whose codec no row carries.
// The migration fills it with the codec those builds encoded,
// so a stored stream keeps publishing the track it had.
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

func TestLoadMigratesMissingTileWatchTransport(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Viewer.TileWatchTransport = "" // empty is the key a file written before the option leaves out
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t); got.Viewer.TileWatchTransport != Defaults().Viewer.TileWatchTransport {
		t.Errorf("tile watch transport = %q, want migrated to %q", got.Viewer.TileWatchTransport, Defaults().Viewer.TileWatchTransport)
	}
}

// The card draws one of three routes and has no reading for a fourth,
// so a file written before the toggle became a setting,
// and one a hand edit put a route no build draws into, both arrive naming one this build draws.
func TestLoadRepairsThePreviewRoute(t *testing.T) {
	for _, stored := range []string{"", "sideways"} {
		isolateConfig(t)

		s := Defaults()
		s.Viewer.PreviewRoute = stored
		if err := Save(s); err != nil {
			t.Fatalf("Save: %v", err)
		}

		if got := mustLoad(t); got.Viewer.PreviewRoute != Defaults().Viewer.PreviewRoute {
			t.Errorf("preview route = %q from a stored %q, want repaired to %q",
				got.Viewer.PreviewRoute, stored, Defaults().Viewer.PreviewRoute)
		}
	}
}

// Every route the card draws survives the store, off is the one a repair could swallow:
// it is the route a reader picks to give a reader slot back,
// and a load that walked it to the local one would open a decode nobody asked for.
func TestEveryPreviewRouteSurvivesAReload(t *testing.T) {
	for _, route := range PreviewRoutes {
		isolateConfig(t)

		s := Defaults()
		s.Viewer.PreviewRoute = route
		if err := Save(s); err != nil {
			t.Fatalf("Save: %v", err)
		}

		if got := mustLoad(t); got.Viewer.PreviewRoute != route {
			t.Errorf("preview route = %q after a reload, want the stored %q", got.Viewer.PreviewRoute, route)
		}
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

// The builder matches both fields against a table,
// so a file written before either option has to arrive carrying a value that table names.
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

// A file written before the frame memory option names none,
// and both publish engines refuse a value their table does not carry.
// The table's own default is the one value every capture and codec pair satisfies,
// so migrating to it keeps an upgrade from turning a working stream into a refusal.
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

// The portal restore token is the compositor's receipt for one consent,
// and reusing it is what keeps the picker off every publish.
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

	// An empty token is the compositor saying the consent was not persisted,
	// so the one on disk is spent,
	// and keeping it would send the next SelectSources a value no compositor honours.
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

// The token is one machine's and one consent's,
// so a preset copied to another machine would carry a token no compositor there issued.
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
// The form then opens on values the user did not choose,
// a state to report rather than one to present as their settings.
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

// The next field change rewrites the working settings,
// so a corrupt file left in place is one that write destroys.
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
// A stored flag is a second copy of a fact the host already carries,
// and the two disagree the moment a host is edited:
// a "no" left beside a public name would send the picture in the clear on the strength of a file,
// and reset-to-defaults would write that "no" itself.
func TestEncryptionIsDerivedAndNeverStored(t *testing.T) {
	encoded, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatalf("rendering the settings: %v", err)
	}
	if strings.Contains(string(encoded), "tls") {
		t.Errorf("the stored settings carry a tls key: %s", encoded)
	}

	// A file written by an older build still has one, and it is read as noise.
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

// Which relays are reached in the clear, the whole of the question above.
// A name is encrypted whatever it resolves to:
// resolving it is a question this cannot ask,
// and the wrong guess is a stream on the wire for anyone to read.
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

// Membership is a key to derive the prefix from and a name claimed under it, both or neither:
// a machine holding a key it has claimed no name in states no presence,
// and the relay closes what it publishes on the next sweep (internal/membership).
func TestMembershipTakesAKeyAndANameTogether(t *testing.T) {
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}

	for state, relay := range map[string]Relay{
		"a member":          {Host: "relay.example", GroupKey: groupKey.String(), DisplayName: "bjoern"},
		"a key and no name": {Host: "relay.example", GroupKey: groupKey.String()},
		"a name and no key": {Host: "relay.example", DisplayName: "bjoern"},
		"neither":           {Host: "relay.example"},
	} {
		if got, want := relay.InGroup(), state == "a member"; got != want {
			t.Errorf("%s is in a group = %v, want %v", state, got, want)
		}
	}
}

// The prefix a list shortens a name by and the prefix a path is published under are one string.
// Two derivations of it would let a viewer's list print a name the relay has no path for,
// which reads as a stream that will not open.
func TestThePrefixIsWhatAPathIsBuiltWith(t *testing.T) {
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}

	for deployment, relay := range map[string]Relay{
		"a group":             {Host: "relay.example", GroupKey: groupKey.String()},
		"no group key":        {Host: "relay.example"},
		"a LAN relay":         {Host: "192.168.1.9"},
		"a damaged group key": {Host: "relay.example", GroupKey: "not a group key"},
	} {
		if got, want := relay.Path("standup"), relay.Prefix()+"standup"; got != want {
			t.Errorf("%s publishes to %q, and its prefix builds %q", deployment, got, want)
		}
	}

	if got := (Relay{Host: "relay.example", GroupKey: groupKey.String()}).Prefix(); got != groupKey.Prefix() {
		t.Errorf("a member reaches under %q, want the group's own %q", got, groupKey.Prefix())
	}
	// A stream lives in a group, so a machine holding no key reaches under no prefix,
	// wherever that relay stands.
	for _, host := range []string{"relay.example", "192.168.1.9"} {
		if got := (Relay{Host: host}).Prefix(); got != "" {
			t.Errorf("a keyless machine on %s reaches under %q, want no prefix", host, got)
		}
	}
	if got := (Relay{}).Prefix(); got != "" {
		t.Errorf("a relay nobody named derives the prefix %q, want none", got)
	}
}

// The name is derived from what is captured, so two settings differing only in which monitor is
// selected land on two different names, and reading it twice off the same settings agrees.
func TestTheStreamNameFollowsTheMonitor(t *testing.T) {
	for monitor, want := range map[int]string{0: "monitor-0", 1: "monitor-1", 3: "monitor-3"} {
		if got := (Publish{Monitor: monitor}).Name(); got != want {
			t.Errorf("monitor %d names the stream %q, want %q", monitor, got, want)
		}
	}
}

// The claim a machine holds in its group leads the stream's own name, so two machines capturing the
// same monitor still land on two names, the claim being unique per group (internal/membership).
// A machine holding no claim shows the bare name, there being nothing to lead it with.
func TestTheFullStreamNameLeadsWithTheDisplayName(t *testing.T) {
	s := Settings{Relay: Relay{DisplayName: "bjoern"}, Publish: Publish{Monitor: 0}}
	if got, want := s.StreamName(), "bjoern/monitor-0"; got != want {
		t.Errorf("stream name = %q, want %q", got, want)
	}

	s.Relay.DisplayName = ""
	if got, want := s.StreamName(), "monitor-0"; got != want {
		t.Errorf("an unclaimed machine names its stream %q, want the bare %q", got, want)
	}
}

// A name given rather than derived leads with the same claim,
// so two machines running the synthetic set land on two paths
// instead of publishing over each other under one (internal/publish, teststream.go).
func TestAGivenStreamNameLeadsWithTheDisplayName(t *testing.T) {
	s := Settings{Relay: Relay{DisplayName: "bjoern"}, Publish: Publish{Monitor: 0}}.WithStreamName("test-1")
	if got, want := s.StreamName(), "bjoern/test-1"; got != want {
		t.Errorf("stream name = %q, want %q", got, want)
	}

	s.Relay.DisplayName = ""
	if got, want := s.StreamName(), "test-1"; got != want {
		t.Errorf("an unclaimed machine names its stream %q, want the bare %q", got, want)
	}
}

// The publish path is the stream name carried through the relay's own prefixing,
// the one derivation transport builders read rather than composing the two themselves.
func TestPublishPathCarriesTheStreamNameThroughThePrefix(t *testing.T) {
	s := Settings{Relay: Relay{Host: "relay.example", DisplayName: "bjoern"}, Publish: Publish{Monitor: 2}}
	if got, want := s.PublishPath(), s.Relay.Path(s.StreamName()); got != want {
		t.Errorf("publish path = %q, want %q", got, want)
	}
}

// A member goes by the name they claimed, and a relay path carries a narrow alphabet,
// so a path is built from the name spelled for one (internal/group, SpellName).
// The relay refuses a path outside that alphabet at the handshake,
// which takes the publish of anybody whose name holds a space, an umlaut or an emoji.
func TestAPathSpellsANameTheRelayWouldRefuse(t *testing.T) {
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	s := Settings{Relay: Relay{Host: "relay.example", GroupKey: groupKey.String(), DisplayName: "Björn Ö"}}

	path := s.PublishPath()
	if got, want := path, s.Relay.Prefix()+"Bj_c3_b6rn_20_c3_96/monitor-0"; got != want {
		t.Errorf("a member called %q publishes to %q, want %q", s.Relay.DisplayName, got, want)
	}

	// A viewer's list reads the name back off the path, and asks for that path again.
	name, ok := group.NameOf(strings.TrimPrefix(path, s.Relay.Prefix()))
	if !ok || name != s.StreamName() {
		t.Errorf("the path reads back as (%q, %v), want %q", name, ok, s.StreamName())
	}
	if got := s.WatchPath(name); got != path {
		t.Errorf("watching that name opens %q, and it publishes to %q", got, path)
	}
}

// A stream a viewer named reaches the relay under the prefix this machine publishes under.
// The name comes off a viewer's list, inside that prefix (internal/app, insidePrefix),
// so one derivation puts it back on rather than each transport builder.
func TestAWatchPathPutsThePrefixBackOn(t *testing.T) {
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}
	s := Settings{Relay: Relay{Host: "relay.example", GroupKey: groupKey.String(), DisplayName: "bjoern"}}

	if got, want := s.WatchPath("alice/monitor-1"), s.Relay.Prefix()+"alice/monitor-1"; got != want {
		t.Errorf("a viewer opens %q, want %q", got, want)
	}

	// This machine's own stream is watched the way anybody else's is,
	// so the end-to-end preview reaches the path the publish writes to.
	if got, want := s.WatchPath(s.StreamName()), s.PublishPath(); got != want {
		t.Errorf("watching this machine's own stream opens %q, and it publishes to %q", got, want)
	}
}

// The SRT passphrase follows the group key the way the path does,
// one derivation answering both ends of the leg,
// so nothing about it is stored and nothing about it is typed.
// A stored value beside the key would be a second copy of a fact the key already carries,
// wrong the moment the group changes.
func TestTheSrtPassphraseFollowsTheGroupKey(t *testing.T) {
	groupKey, err := group.NewKey()
	if err != nil {
		t.Fatalf("drawing a group key: %v", err)
	}

	for deployment, want := range map[Relay]string{
		{Host: "relay.example", GroupKey: groupKey.String()}: groupKey.SrtPassphrase(),
		// A machine in no group, and one whose key is damaged, both publish to the bare name
		// every relay refuses (Path), so the leg either would have keyed opens nowhere.
		{Host: "relay.example"}:                              "",
		{Host: "192.168.1.9"}:                                "",
		{Host: "relay.example", GroupKey: "not a group key"}: "",
		{}: "",
	} {
		if got := deployment.SrtPassphrase(); got != want {
			t.Errorf("the relay %+v keys SRT with %q, want %q", deployment, got, want)
		}
	}

	encoded, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatalf("rendering the settings: %v", err)
	}
	if strings.Contains(string(encoded), "srtPassphrase") {
		t.Errorf("the stored settings carry an srtPassphrase key: %s", encoded)
	}
}

// A burst ceiling of zero is an answer and not an absence: the encode bounded by nothing,
// which the form offers as an entry of its own beside the ceiling's own band
// (api/proto/screenshare/v1/form.proto, CONTROL_KIND_NUMBER_SELECT).
// A store that replaced it with a default would take that answer back on the next start.
func TestAnUncappedBurstSurvivesTheStore(t *testing.T) {
	isolateConfig(t)

	want := Defaults()
	want.Publish.MaxrateM = 0
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t).Publish.MaxrateM; got != 0 {
		t.Errorf("the stored burst ceiling read back as %d Mbit/s, want the uncapped zero it was saved as", got)
	}
}

// A file written before the field carries no key for it, and an absent key is no answer anybody gave.
func TestAFileWithNoBurstCeilingKeepsTheDefault(t *testing.T) {
	isolateConfig(t)

	dir, err := configDir()
	if err != nil {
		t.Fatalf("resolving the config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, configFileName),
		[]byte(`{"publish":{"name":"old","bitrateM":40}}`), 0o600); err != nil {
		t.Fatalf("writing a file with no burst ceiling: %v", err)
	}

	if got := mustLoad(t).Publish.MaxrateM; got != Defaults().Publish.MaxrateM {
		t.Errorf("a file naming no burst ceiling read back as %d Mbit/s, want the default %d",
			got, Defaults().Publish.MaxrateM)
	}
}

// A file written while the encode was one field carries an encoder name under the old key,
// which the ordinary decode never reads:
// the pair replaced it and neither half spells a codec.
// The stored stream keeps encoding as it did, on the row that name addressed.
func TestLoadMigratesTheOneCodecKeyOntoThePair(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.Format, s.Publish.Encoder = "", ""
	s.Publish.FlatCodec = "libsvtav1"
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.Publish.Format != "av1" || got.Publish.Encoder != "svt-av1" {
		t.Errorf("format, encoder = %q, %q, want av1, svt-av1", got.Publish.Format, got.Publish.Encoder)
	}
	if got.Publish.Codec() != "libsvtav1" {
		t.Errorf("the migrated pair addresses %q, want libsvtav1", got.Publish.Codec())
	}
	if got.Publish.FlatCodec != "" {
		t.Errorf("an upgraded publish carries no pre-pair codec key, got %q", got.Publish.FlatCodec)
	}
}

// Every relay this app is pointed at runs a group service,
// and the address of one follows the address of the relay:
// the proxy's own name off a trusted network,
// and the port groupd binds where the relay is reached directly (deploy/relay.sh, cmd/groupd).
//
// A relay on this machine answering none is a development relay that issues no token,
// and the relay refuses every publisher that carries none.
func TestEveryNamedRelayNamesItsGroupService(t *testing.T) {
	for host, want := range map[string]string{
		"streamrelay.bjoernblessin.de": "https://streamrelay.bjoernblessin.de",
		"93.184.216.34":                "https://93.184.216.34",
		"127.0.0.1":                    "http://127.0.0.1:9443",
		"localhost":                    "http://localhost:9443",
		"192.168.1.9":                  "http://192.168.1.9:9443",
	} {
		base, ok := (Relay{Host: host}).GroupService()
		if !ok {
			t.Errorf("relay %q names no group service, so nothing there issues it a relay token", host)
			continue
		}
		if base != want {
			t.Errorf("relay %q asks %q, want %q", host, base, want)
		}
	}

	// A relay nobody named is not a deployment without a service: there is no host to ask at all.
	if base, ok := (Relay{}).GroupService(); ok {
		t.Errorf("an unnamed relay asks %q, want no service", base)
	}
}

// An empty display name is a state and not a gap:
// this machine has no name, and joining a group asks for one.
// A migration that filled it would join every group under whatever it filled with,
// a name the user never chose and one another member may already hold.
func TestAnUnnamedMachineKeepsItsEmptyDisplayName(t *testing.T) {
	isolateConfig(t)

	dir, err := configDir()
	if err != nil {
		t.Fatalf("resolving the config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, configFileName),
		[]byte(`{"relay":{"host":"relay.example"}}`), 0o600); err != nil {
		t.Fatalf("writing a file naming no display name: %v", err)
	}

	if got := mustLoad(t).Relay.DisplayName; got != "" {
		t.Errorf("a file naming no display name read back as %q, want the empty name it holds", got)
	}
}

// A stored name no row carries leaves the pair on what a fresh installation holds.
// Splitting it would write a format nothing produces and an encoder no family answers to,
// and the form would then grey both halves with no value to walk to.
func TestLoadMigratesAnUnknownCodecKeyOntoTheDefaults(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.Format, s.Publish.Encoder = "", ""
	s.Publish.FlatCodec = "h264_omx"
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, d := mustLoad(t), Defaults()
	if got.Publish.Format != d.Publish.Format || got.Publish.Encoder != d.Publish.Encoder {
		t.Errorf("format, encoder = %q, %q, want the defaults %q, %q",
			got.Publish.Format, got.Publish.Encoder, d.Publish.Format, d.Publish.Encoder)
	}
}

// Every relay terminates TLS on the RTSP and RTMP legs and binds no cleartext listener at all
// (deploy/mediamtx-groups.yml, rtspEncryption),
// and both builders spell the address rtsps and rtmps whatever port it carries (internal/transport).
// A file naming a cleartext port therefore addresses a listener nothing answers,
// and a publish over it waits out its timeout against a closed port instead of being refused.
func TestLoadMigratesTheCleartextRelayPorts(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Relay.RtspPort = 8554
	s.Relay.RtmpPort = 1935
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, d := mustLoad(t), Defaults()
	if got.Relay.RtspPort != d.Relay.RtspPort {
		t.Errorf("RTSP port = %d, want the TLS listener %d", got.Relay.RtspPort, d.Relay.RtspPort)
	}
	if got.Relay.RtmpPort != d.Relay.RtmpPort {
		t.Errorf("RTMP port = %d, want the TLS listener %d", got.Relay.RtmpPort, d.Relay.RtmpPort)
	}
}

// A relay binds its TLS listeners wherever it is told to,
// a second one on the same host taking numbers of its own (cmd/soak/scripts/start.sh).
// Those are ports somebody chose,
// so the move above reaches the two numbers the cleartext listeners had and no other.
func TestARelayOnItsOwnPortsKeepsThem(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Relay.RtspPort = 18554
	s.Relay.RtmpPort = 11936
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := mustLoad(t)
	if got.Relay.RtspPort != 18554 || got.Relay.RtmpPort != 11936 {
		t.Errorf("RTSP, RTMP ports = %d, %d, want the stored 18554, 11936",
			got.Relay.RtspPort, got.Relay.RtmpPort)
	}
}

// The relay negotiates the larger of its window and the stored one (SrtRelayFloorMs),
// so a stored figure below the floor names a window the hop does not run at.
func TestLoadRaisesAPublishWindowBelowTheRelayFloor(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.SrtPublishLatencyMs = 50
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t); got.Publish.SrtPublishLatencyMs != SrtRelayFloorMs {
		t.Errorf("publish latency = %d, want raised to the relay floor of %d",
			got.Publish.SrtPublishLatencyMs, SrtRelayFloorMs)
	}
}

func TestAFreshInstallationFollowsTheBalancedPreset(t *testing.T) {
	if got := Defaults().Publish.Preset; got != PresetBalanced {
		t.Errorf("a fresh installation follows %q, want %q", got, PresetBalanced)
	}
}

// The empty key is the detached state, so no migration fills it and no save drops it:
// either would put a machine back on a preset the user left.
func TestADetachedDraftStaysDetachedAcrossAReload(t *testing.T) {
	isolateConfig(t)

	s := Defaults()
	s.Publish.Preset = ""
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := mustLoad(t).Publish.Preset; got != "" {
		t.Errorf("a detached draft reloaded following %q", got)
	}
}

// A file written before the app group names neither flag.
// Load decodes over the defaults, so an install keeps reporting a crash
// and reading the published release the way it did before either was answerable.
func TestAFileFromBeforeTheAppGroupKeepsBothFlags(t *testing.T) {
	isolateConfig(t)

	path := mustSettingsPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"relay":{"host":"stored.example"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := mustLoad(t)
	if !got.App.SendCrashReports || !got.App.CheckUpdatesOnStart {
		t.Errorf("app settings = %+v, want what a fresh installation holds", got.App)
	}
}
