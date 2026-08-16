package form

import (
	"google.golang.org/protobuf/proto"

	"fmt"
	"slices"
	"strconv"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// fieldDeclaredKeys is every constant in keys.go, written out because Go has no reflection over a
// const block to derive it from.
// It is the only other copy of that list, which is what makes the bijection below a check rather
// than a tautology: a key added to keys.go and to no table fails here.
var fieldDeclaredKeys = []string{
	KeyName, KeyRelayHost, KeyRelayTls, KeyGroupKey, KeySrtPassphrase, KeySrtPort, KeyAPIPort, KeyRtspPort, KeyWebrtcPort,
	KeyRtmpPort, KeyHlsPort, KeyMoqPort,
	KeyTransport, KeyCodec, KeyMode, KeyChroma, KeyColorRange, KeyFps, KeyCq,
	KeyBitrateM, KeyMaxrateM, KeyVbvMs, KeyGop, KeyBframes, KeyEffort, KeyTune,
	KeyCapture, KeyAudioSource, KeyAudioSourceDevice, KeyAudioSourceGain, KeyAudioSourceMute,
	KeyAudioCodec, KeyDrmMap, KeyMonitor, KeyCaptureMemory,
	KeyCursor,
	KeySrtPublishLatencyMs, KeySrtWatchLatencyMs,
	KeyRtspPublishProtocol, KeyRtspWatchProtocol,
	KeyUplinkMbps,
	KeyOutputResolution,
	KeyTileWatchTransport, KeyRtspWatchLatencyMs, KeyRenderChain,
}

// fieldDeclaredGroups is every group key, written out for the same reason.
var fieldDeclaredGroups = []string{
	GroupStream, GroupSource, GroupQuality, GroupAudio,
	GroupTransport, GroupWatch, GroupRelay,
}

// fieldTestDeps is a machine with two monitors, which is what Deps exists for: a form resolves
// against hardware the test is not running on.
func fieldTestDeps() Deps {
	return Deps{
		Monitors: []display.Monitor{
			{Index: 0, Width: 2560, Height: 1440, RefreshHz: 144, Primary: true},
			{Index: 1, Width: 1920, Height: 1080, RefreshHz: 60},
		},
	}
}

func fieldRowFor(t *testing.T, key string) *field {
	t.Helper()
	for i := range fieldTable {
		if fieldTable[i].key == key {
			return &fieldTable[i]
		}
	}
	t.Fatalf("no row for key %q", key)
	return nil
}

// fieldOptionValues is what one row's builder offers, in the builder's own order.
func fieldOptionValues(t *testing.T, key string) []string {
	t.Helper()
	f := fieldRowFor(t, key)
	d, s := fieldTestDeps(), settings.Defaults()

	built := []*screensharev1.FieldOption(nil)
	switch {
	case f.options != nil:
		built = f.options(d, s)
	case f.itemOptions != nil:
		// The row past the end of an empty list, which is the only entry a fresh installation draws.
		built = f.itemOptions(d, s, settings.DefaultAudioSource())
	default:
		t.Fatalf("%s offers no options", key)
	}

	var out []string
	for _, o := range built {
		out = append(out, o.GetValue())
	}
	return out
}

// A shell binds its widgets by key and a capability gap greys by the same key, so a key with no row
// is a gap pointing at nothing and a row under a key no constant declares is a control nothing can
// point at.
// The two lists are one list, and this is where they are held to it.
func TestEveryDeclaredKeyHasExactlyOneRow(t *testing.T) {
	for _, key := range fieldDeclaredKeys {
		rows := 0
		for _, f := range fieldTable {
			if f.key == key {
				rows++
			}
		}
		if rows != 1 {
			t.Errorf("key %q has %d rows, want exactly one", key, rows)
		}
	}
	if len(fieldTable) != len(fieldDeclaredKeys) {
		t.Errorf("the table has %d rows for %d declared keys", len(fieldTable), len(fieldDeclaredKeys))
	}
}

func TestEveryRowNamesADeclaredKey(t *testing.T) {
	for _, f := range fieldTable {
		if !slices.Contains(fieldDeclaredKeys, f.key) {
			t.Errorf("row %q names no declared key", f.key)
		}
	}
}

// resolveField calls the value function on every row it renders, so a row carrying none panics on
// the first resolve rather than rendering an empty control.
func TestEveryRowShowsAValue(t *testing.T) {
	s := settings.Defaults()
	for i := range fieldTable {
		f := &fieldTable[i]
		if (f.value == nil) == (f.itemValue == nil) {
			t.Errorf("%s reads its value either off the draft or off one entry, and states neither or both", f.key)
			continue
		}
		// Entry 0 of an empty list is the row that grows it, which is the one a fresh installation draws.
		entry := noEntry
		if f.repeat {
			entry = 0
		}
		if v := fieldValue(f, s, entry); v == nil || v.GetKind() == nil {
			t.Errorf("%s reads no value out of the defaults", f.key)
		}
	}
}

// A control's default comes from the defaults and not from the draft in front of it.
// A shell offers putting a group of settings back with it, and a default that followed the draft
// would put a changed value back to what it was just changed to.
func TestEveryFieldStatesWhatAFreshInstallationHolds(t *testing.T) {
	fresh := settings.Defaults()

	draft := settings.Defaults()
	draft.Relay.Host = "elsewhere.example"
	draft.Relay.SrtPort = 9001

	deps := fieldTestDeps()
	for _, g := range resolveGroups(availabilityOf(deps, draft), deps, draft, fresh) {
		for _, f := range g.GetFields() {
			if f.GetDefaultValue().GetKind() == nil {
				t.Errorf("%s states no default", f.GetKey())
				continue
			}

			switch f.GetKey() {
			case KeyRelayHost:
				if got := f.GetValue().GetText(); got != draft.Relay.Host {
					t.Errorf("%s holds %q, want the draft's %q", f.GetKey(), got, draft.Relay.Host)
				}
				if got := f.GetDefaultValue().GetText(); got != fresh.Relay.Host {
					t.Errorf("%s defaults to %q, want %q", f.GetKey(), got, fresh.Relay.Host)
				}
			case KeySrtPort:
				if got := f.GetValue().GetNumber(); got != int64(draft.Relay.SrtPort) {
					t.Errorf("%s holds %d, want the draft's %d", f.GetKey(), got, draft.Relay.SrtPort)
				}
				if got := f.GetDefaultValue().GetNumber(); got != int64(fresh.Relay.SrtPort) {
					t.Errorf("%s defaults to %d, want %d", f.GetKey(), got, fresh.Relay.SrtPort)
				}
			}
		}
	}
}

// resolveGroups walks the groups and picks the rows naming each, so a row assigned to a group no
// groups entry carries is silently absent from every screen.
func TestEveryRowBelongsToADeclaredGroup(t *testing.T) {
	for _, f := range fieldTable {
		if !slices.Contains(fieldDeclaredGroups, f.group) {
			t.Errorf("%s belongs to group %q, which is not one of %v", f.key, f.group, fieldDeclaredGroups)
		}
	}
	for _, g := range groups {
		if !slices.Contains(fieldDeclaredGroups, g.key) {
			t.Errorf("group %q is not one of %v", g.key, fieldDeclaredGroups)
		}
	}
}

// resolveGroups drops a group with no fields, so a heading no row names never appears.
// Declaring one is either a forgotten row or a group that should not exist, and both are worth
// failing on.
func TestEveryGroupDrawsAtLeastOneField(t *testing.T) {
	for _, g := range groups {
		rows := 0
		for _, f := range fieldTable {
			if f.group == g.key {
				rows++
			}
		}
		if rows == 0 {
			t.Errorf("group %q has no fields", g.key)
		}
	}
	for _, key := range fieldDeclaredGroups {
		if !slices.ContainsFunc(groups, func(g group) bool { return g.key == key }) {
			t.Errorf("group key %q has no entry in the groups table", key)
		}
	}
}

// The key is the whole of a group on the wire: the heading over it and the paragraph under it are
// looked up by that key on the surface that draws it, so a key the surface has never heard of
// renders as an unnamed run of fields.
func TestEveryGroupIsDeclaredOnceUnderADeclaredKey(t *testing.T) {
	declared := []string{
		GroupStream, GroupSource, GroupQuality, GroupAudio,
		GroupTransport, GroupWatch, GroupRelay,
	}
	seen := make(map[string]bool, len(groups))
	for _, g := range groups {
		if !slices.Contains(declared, g.key) {
			t.Errorf("group %q is not one keys.go declares", g.key)
		}
		if seen[g.key] {
			t.Errorf("group %q is declared twice", g.key)
		}
		seen[g.key] = true
	}
	for _, key := range declared {
		if !seen[key] {
			t.Errorf("group %q is declared and never rendered", key)
		}
	}
}

// Which groups are applied rather than staged, written out rather than read off the table it
// checks, which is what makes it a check: a group that gains or loses the flag fails here.
//
// Applied where it should be staged persists a half-configured stream on every keystroke.
// Staged where it should be applied is the deadlock form.proto describes: the relay's address then
// reaches the backend only through a publish, and that publish is refused because the relay it
// would replace cannot be reached.
func TestOnlyTheStandingSettingsAreApplied(t *testing.T) {
	applied := map[string]bool{
		GroupRelay: true,
	}

	for _, g := range groups {
		if want := applied[g.key]; g.applied != want {
			t.Errorf("group %q is applied=%v, want %v", g.key, g.applied, want)
		}
	}

	// A shell reads the flag off the rendered group and not off this table, so it has to survive the
	// render.
	deps, draft := fieldTestDeps(), settings.Defaults()
	for _, g := range resolveGroups(availabilityOf(deps, draft), deps, draft, settings.Defaults()) {
		if want := applied[g.GetKey()]; g.GetApplied() != want {
			t.Errorf("resolved group %q is applied=%v, want %v", g.GetKey(), g.GetApplied(), want)
		}
	}
}

// A select and a radio carry options, a number and a slider a range, the number that carries a
// ladder both, and every other control neither.
// A select with no options is a dropdown a shell cannot open, a number with no range is a field
// whose ends the contract says to read as unbounded rather than as zero, and a number-select
// missing either half is an ordinary control mislabelled as the combined one.
func TestASelectOffersOptionsAndANumberStatesARange(t *testing.T) {
	for _, f := range fieldTable {
		switch f.control {
		case screensharev1.ControlKind_CONTROL_KIND_SELECT,
			screensharev1.ControlKind_CONTROL_KIND_RADIO:
			if f.options == nil && f.itemOptions == nil {
				t.Errorf("%s is a select or radio with no options", f.key)
			}
			if f.bounds != nil {
				t.Errorf("%s is a select or radio carrying a range", f.key)
			}
		case screensharev1.ControlKind_CONTROL_KIND_NUMBER,
			screensharev1.ControlKind_CONTROL_KIND_SLIDER:
			if f.bounds == nil {
				t.Errorf("%s is a number or slider with no range", f.key)
			}
			if f.options != nil {
				t.Errorf("%s is a number or slider carrying options", f.key)
			}
		case screensharev1.ControlKind_CONTROL_KIND_NUMBER_SELECT:
			if f.options == nil && f.itemOptions == nil {
				t.Errorf("%s is a number-select with no ladder", f.key)
			}
			if f.bounds == nil {
				t.Errorf("%s is a number-select with no range", f.key)
			}
		default:
			if f.options != nil || f.bounds != nil {
				t.Errorf("%s is a %v carrying options or a range", f.key, f.control)
			}
		}
	}
}

// The ladder is a shortcut and not the domain: every step is a rate the range admits, so picking
// one can never write a value the same form refuses.
// The combined control rests on that, and its two halves are stated in two places.
func TestTheFrameRateLadderStaysInsideItsRange(t *testing.T) {
	d := fieldTestDeps()
	s := settings.Defaults()

	bounds := fieldFpsBounds(d, s)
	for _, o := range optionFpsPresets(d, s) {
		fps, err := strconv.Atoi(o.GetValue())
		if err != nil {
			t.Fatalf("frame rate ladder offers %q, which is not a number", o.GetValue())
		}
		if int64(fps) < bounds.GetMin() || int64(fps) > bounds.GetMax() {
			t.Errorf("frame rate ladder offers %d, outside the %d-%d range the field states",
				fps, bounds.GetMin(), bounds.GetMax())
		}
	}
}

// A saved rate off the ladder is offered all the same, so the closed control shows the rate the
// stream is captured at rather than the nearest step to it.
func TestTheFrameRateLadderCarriesASavedRateOffIt(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Fps = 37

	if !slices.ContainsFunc(optionFpsPresets(fieldTestDeps(), s), func(o *screensharev1.FieldOption) bool {
		return o.GetValue() == "37"
	}) {
		t.Error("the frame rate ladder drops a saved rate that is not one of its steps")
	}
}

// A unit says what a number means, and it crosses as an enum rather than a spelling: how "Mbit/s"
// sits beside its figure is typography, and one string could not tell a surface which half of it
// was which.
func TestEveryQuantityStatesItsUnit(t *testing.T) {
	units := map[string]screensharev1.Unit{
		KeyFps:                 screensharev1.Unit_UNIT_FRAMES_PER_SECOND,
		KeyBitrateM:            screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		KeyMaxrateM:            screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		KeyUplinkMbps:          screensharev1.Unit_UNIT_MEGABITS_PER_SECOND,
		KeyVbvMs:               screensharev1.Unit_UNIT_MILLISECONDS,
		KeySrtPublishLatencyMs: screensharev1.Unit_UNIT_MILLISECONDS,
		KeySrtWatchLatencyMs:   screensharev1.Unit_UNIT_MILLISECONDS,
		KeyRtspWatchLatencyMs:  screensharev1.Unit_UNIT_MILLISECONDS,
		KeyGop:                 screensharev1.Unit_UNIT_FRAMES,
		KeyBframes:             screensharev1.Unit_UNIT_FRAMES,
		KeyAudioSourceGain:     screensharev1.Unit_UNIT_PERCENT,
	}
	for key, want := range units {
		if got := fieldRowFor(t, key).unit; got != want {
			t.Errorf("%s carries unit %v, want %v", key, got, want)
		}
	}
	// A field that is not a quantity states none, so no surface draws a unit beside a stream name or a
	// codec.
	for _, f := range fieldTable {
		if _, quantity := units[f.key]; quantity {
			continue
		}
		if f.unit != screensharev1.Unit_UNIT_UNSPECIFIED {
			t.Errorf("%s is not a quantity and carries unit %v", f.key, f.unit)
		}
	}
}

// An option's verdict is availability's alone.
// A builder pre-enabling an entry would be a second place deciding what is greyed, and resolveField
// overwrites both fields anyway, so a value set there is either ignored or a disagreement waiting
// to be read as the truth.
func TestAnOptionLeavesItsVerdictToAvailability(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	for _, f := range fieldTable {
		if f.options == nil {
			continue
		}
		for _, o := range f.options(d, s) {
			if o.GetEnabled() || o.GetReason() != nil {
				t.Errorf("%s offers %q already judged (enabled=%v reason=%v)",
					f.key, o.GetValue(), o.GetEnabled(), o.GetReason())
			}
		}
	}
}

// A shell names an entry by its value and sends that value back, so two entries sharing one are two
// ways to mean the same thing and a repair that can never settle.
// The empty value is legal on one control, the output resolution, where it means the capture
// reaches the encoder unscaled.
func TestEveryOptionCarriesADistinctValue(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	for _, f := range fieldTable {
		if f.options == nil {
			continue
		}
		options := f.options(d, s)
		if len(options) == 0 {
			t.Errorf("%s offers nothing", f.key)
			continue
		}
		seen := make(map[string]bool, len(options))
		for _, o := range options {
			if o.GetValue() == "" && f.key != KeyOutputResolution {
				t.Errorf("%s offers an entry with no value at all", f.key)
			}
			if seen[o.GetValue()] {
				t.Errorf("%s offers %q twice", f.key, o.GetValue())
			}
			seen[o.GetValue()] = true
		}
	}
}

// Every value a control offers comes off a domain table, so the form cannot offer what the encoder
// refuses and cannot withhold what it accepts.
// A list typed into this package passes every other test here and fails this one.
func TestOptionValuesComeFromTheDomainTables(t *testing.T) {
	var codecNames, drmNames []string
	for _, c := range capabilities.Codecs {
		codecNames = append(codecNames, c.Name)
	}
	for _, m := range ffmpeg.DrmMaps {
		drmNames = append(drmNames, m.Name)
	}

	cases := []struct {
		key   string
		table []string
	}{
		{KeyCapture, publish.Captures()},
		{KeyCaptureMemory, gpupath.Memories},
		{KeyDrmMap, drmNames},
		{KeyCodec, codecNames},
		{KeyMode, capabilities.Modes},
		{KeyAudioCodec, capabilities.AudioNames()},
		// fieldTestDeps names no platform, which the table answers with every source offered.
		// A machine that serves fewer greys the rest rather than leaving them out, so those platforms are
		// the greying test's.
		{KeyAudioSource, platform.AudioSourceIDs(platform.Info{})},
		{KeyTileWatchTransport, transport.WatchNames(capabilities.EngineGst)},
	}
	for _, c := range cases {
		if got := fieldOptionValues(t, c.key); !slices.Equal(got, c.table) {
			t.Errorf("%s offers %v, want the table's %v", c.key, got, c.table)
		}
	}
}

// An audio source's note is what serves it on this machine, read off the platform table rather than
// written into the paragraph beside it.
// The mechanism differs per platform, so a paragraph naming one platform's would be read elsewhere
// as a description of what that machine is doing.
// A machine that does not serve the source notes nothing and carries the greying's sentence
// instead.
func TestAnAudioSourcesNoteIsWhatServesItHere(t *testing.T) {
	for _, info := range []platform.Info{
		{OS: "linux", Display: "wayland"}, {OS: "windows"}, {OS: "darwin"},
	} {
		notes := map[string]*screensharev1.Text{}
		for _, o := range optionAudioSources(Deps{Platform: info}, settings.Defaults()) {
			notes[o.GetValue()] = o.GetNote()
		}
		for _, s := range platform.AudioSources(info) {
			if !proto.Equal(notes[s.ID], s.Server) {
				t.Errorf("%s notes %v beside %q, the platform table serves it with %v",
					info.OS, notes[s.ID], s.ID, s.Server)
			}
		}
	}
}

// The pixel formats have no table of their own: a chroma is a fact about a codec, so the union of
// the rows is the value space.
// A format some codec codes and the form withholds is unreachable, and one the form offers and no
// codec codes greys for every selection, which is a dead entry rather than a teaching one.
func TestThePixelFormatsAreTheUnionOfWhatTheCodecsCode(t *testing.T) {
	offered := fieldOptionValues(t, KeyChroma)
	coded := optionCodedChromas()
	for _, chroma := range coded {
		if !slices.Contains(offered, chroma) {
			t.Errorf("some codec codes %q and the form does not offer it", chroma)
		}
	}
	for _, chroma := range offered {
		if !slices.Contains(coded, chroma) {
			t.Errorf("the form offers %q and no codec codes it", chroma)
		}
	}
}

// The publish leg offers what either engine serializes rather than what the selected one does.
// A transport this capture backend's engine lacks is one a neighbouring backend has, so the entry
// stays and greys with the engine named.
// A protocol no engine ingests is absent, since no choice on this screen could lift the reason.
func TestThePublishLegOffersWhatEitherEngineSerializes(t *testing.T) {
	offered := fieldOptionValues(t, KeyTransport)
	var union []string
	for _, engine := range capabilities.Engines {
		for _, name := range transport.PublishNames(engine) {
			if !slices.Contains(union, name) {
				union = append(union, name)
			}
		}
	}
	slices.Sort(union)
	if !slices.Equal(offered, union) {
		t.Errorf("the publish leg offers %v, want %v", offered, union)
	}
	for _, name := range offered {
		if _, ok := transport.Get(name); !ok {
			t.Errorf("the publish leg offers %q, which the registry does not carry", name)
		}
	}
}

// The monitor list is the enumeration plus whatever the settings name.
// A selection the machine no longer reports stays on the list, because seeing it is what lets a
// reader move off it.
func TestTheMonitorListKeepsAStaleSelection(t *testing.T) {
	d := fieldTestDeps()
	s := settings.Defaults()
	s.Publish.Monitor = 7

	var values []string
	for _, o := range fieldRowFor(t, KeyMonitor).options(d, s) {
		values = append(values, o.GetValue())
	}
	want := []string{"0", "1", "7"}
	if !slices.Equal(values, want) {
		t.Errorf("the monitor list is %v, want %v", values, want)
	}

	s.Publish.Monitor = 1
	values = values[:0]
	for _, o := range fieldRowFor(t, KeyMonitor).options(d, s) {
		values = append(values, o.GetValue())
	}
	if want := []string{"0", "1"}; !slices.Equal(values, want) {
		t.Errorf("the monitor list is %v with a live selection, want %v", values, want)
	}
}

// The resolution ladder is derived from the captured monitor rather than listed, so another screen
// produces another ladder and no entry is an upscale.
func TestTheOutputResolutionLadderFollowsTheCapturedMonitor(t *testing.T) {
	d := fieldTestDeps()
	s := settings.Defaults()
	s.Publish.Monitor = 0

	options := fieldRowFor(t, KeyOutputResolution).options(d, s)
	var values []string
	for _, o := range options {
		values = append(values, o.GetValue())
	}
	want := []string{"", "1920x1080", "1280x720"}
	if !slices.Equal(values, want) {
		t.Errorf("the ladder off a 2560x1440 monitor is %v, want %v", values, want)
	}
	// A scaled entry carries the size it was derived from, so a reader is never left working out where
	// it came from.
	// The unscaled entry carries none: it is the source size, and the monitor's own catalog row says
	// what that is.
	for i, o := range options {
		note := o.GetNote()
		if i == 0 {
			if note != nil {
				t.Errorf("the unscaled entry is noted %v, where the source size is the monitor's own row", note)
			}
			continue
		}
		if codeOf(note) != scaledFromSource {
			t.Errorf("entry %q is noted %v, want the size it was scaled from", o.GetValue(), codeOf(note))
		}
	}
	if !options[0].GetRecommended() {
		t.Error("the unscaled entry is the one this backend delivers, so it is the recommended one")
	}

	// The second monitor is shorter, so its ladder is shorter: a step at or above the source's own
	// height would be an upscale.
	s.Publish.Monitor = 1
	values = values[:0]
	for _, o := range fieldRowFor(t, KeyOutputResolution).options(d, s) {
		values = append(values, o.GetValue())
	}
	if want := []string{"", "1280x720"}; !slices.Equal(values, want) {
		t.Errorf("the ladder off a 1920x1080 monitor is %v, want %v", values, want)
	}

	// A monitor the enumeration reported nothing for leaves the unscaled entry alone: there is no
	// source size to scale from, and absolute sizes would be a claim about a screen this machine
	// cannot measure.
	s.Publish.Monitor = 9
	values = values[:0]
	for _, o := range fieldRowFor(t, KeyOutputResolution).options(d, s) {
		values = append(values, o.GetValue())
	}
	if want := []string{""}; !slices.Equal(values, want) {
		t.Errorf("the ladder off an unenumerated monitor is %v, want %v", values, want)
	}
}

// Every chroma subsampling this app encodes in needs an even width, so an odd step is a scaler
// failure rather than a picture.
func TestTheOutputResolutionLadderOffersEvenWidths(t *testing.T) {
	d := Deps{Monitors: []display.Monitor{{Index: 0, Width: 1366, Height: 768}}}
	for _, o := range fieldRowFor(t, KeyOutputResolution).options(d, settings.Defaults()) {
		if o.GetValue() == "" {
			continue
		}
		var width, height int
		if _, err := fmt.Sscanf(o.GetValue(), "%dx%d", &width, &height); err != nil {
			t.Errorf("the ladder offers %q, which is not a WIDTHxHEIGHT", o.GetValue())
			continue
		}
		if width%2 != 0 {
			t.Errorf("the ladder offers %q, whose width is odd", o.GetValue())
		}
	}
}

// The chroma ladder is the one presentation decision this package keeps: which order the pixel
// formats are offered in, most colour detail first.
// A step naming a format no codec codes drops out of the list it is meant to order, and a coded
// format the ladder forgets lands after the ones it names.
func TestTheChromaLadderOrdersExactlyWhatTheCodecsCode(t *testing.T) {
	coded := optionCodedChromas()
	for _, chroma := range optionChromaOrder {
		if !slices.Contains(coded, chroma) {
			t.Errorf("the chroma ladder orders %q, which no codec codes", chroma)
		}
	}
	for _, chroma := range coded {
		if !slices.Contains(optionChromaOrder, chroma) {
			t.Errorf("pixel format %q is coded and unordered, so it lands after the ladder", chroma)
		}
	}
}

// No Go table exports the colour ranges, so this is what holds the list built here against the
// domain: a codec that cannot encode at a range declares a gap on it, and a gap on a value the form
// never offers states a reason for an option nobody was offered.
func TestEveryGappedColourRangeIsOffered(t *testing.T) {
	offered := fieldOptionValues(t, KeyColorRange)
	for _, c := range capabilities.Codecs {
		for _, g := range c.Gaps {
			if g.Option != capabilities.OptionColorRange {
				continue
			}
			if !slices.Contains(offered, g.Value) {
				t.Errorf("%s gaps on colour range %q, which the form does not offer", c.Name, g.Value)
			}
		}
	}
}

// The RTP lower transports are the same case, and the transport package declares them as the watch
// leg's knob choices, the one place they cross a package boundary.
// Neither leg's list is built from that declaration, so both are held against it here.
func TestEveryRtspProtocolTheTransportDeclaresIsOffered(t *testing.T) {
	var declared []string
	for _, o := range transport.WatchOptions("rtsp", settings.Defaults()) {
		if o.Kind == transport.OptionChoice {
			declared = append(declared, o.Choices...)
		}
	}
	if len(declared) == 0 {
		t.Fatal("the RTSP transport declares a lower-transport choice for its watch leg")
	}
	for _, key := range []string{KeyRtspPublishProtocol, KeyRtspWatchProtocol} {
		offered := fieldOptionValues(t, key)
		for _, value := range declared {
			if !slices.Contains(offered, value) {
				t.Errorf("%s does not offer %q, which the transport declares", key, value)
			}
		}
		if len(offered) != len(declared) {
			t.Errorf("%s offers %v against the transport's %v", key, offered, declared)
		}
	}
}

// The effort control offers the selected codec's own ladder in the table's declared order, most
// effort first, whatever direction that encoder's numbering runs.
//
// The step a fresh installation starts on has to be a rung of the default codec's ladder, or the
// control opens on a value it does not offer.
func TestTheEffortLadderIsTheCodecsOwnMostEffortFirst(t *testing.T) {
	fresh := settings.Defaults()
	c, ok := capabilities.Get(fresh.Publish.Codec)
	if !ok {
		t.Fatalf("the default codec %q is not in the table", fresh.Publish.Codec)
	}

	offered := fieldOptionValues(t, KeyEffort)
	if !slices.Equal(offered, c.Effort.Steps) {
		t.Errorf("the control offers %v against the row's own %v", offered, c.Effort.Steps)
	}
	if !slices.Contains(offered, fresh.Publish.Effort) {
		t.Errorf("the ladder %v does not carry the step %q a fresh installation starts on",
			offered, fresh.Publish.Effort)
	}
}

// Every closed set the settings start on is one the form offers.
// A default the form withholds is a first launch opening on a value the user cannot pick again,
// which is the disagreement between the tables and the screen read from the other end.
func TestTheDefaultsAreValuesTheFormOffers(t *testing.T) {
	s := settings.Defaults()
	cases := map[string]string{
		KeyCapture:             s.Publish.Capture,
		KeyCaptureMemory:       s.Publish.CaptureMemory,
		KeyDrmMap:              s.Publish.DrmMap,
		KeyCodec:               s.Publish.Codec,
		KeyChroma:              s.Publish.Chroma,
		KeyColorRange:          s.Publish.ColorRange,
		KeyMode:                s.Publish.Mode,
		KeyEffort:              s.Publish.Effort,
		KeyAudioSource:         platform.AudioSourceNone,
		KeyAudioCodec:          s.Publish.AudioCodec,
		KeyTransport:           s.Publish.Transport,
		KeyTileWatchTransport:  s.Viewer.TileWatchTransport,
		KeyRtspPublishProtocol: s.Publish.RtspPublishProtocol,
		KeyRtspWatchProtocol:   s.Viewer.RtspWatchProtocol,
	}
	for key, value := range cases {
		if offered := fieldOptionValues(t, key); !slices.Contains(offered, value) {
			t.Errorf("%s starts on %q, which the form does not offer: %v", key, value, offered)
		}
	}
}

// A default outside its own control's ends is a slider that opens pinned to one end, or a number
// field reporting an untouched setting as out of range.
func TestEveryRangeAdmitsTheDefaultSettings(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	for i := range fieldTable {
		f := &fieldTable[i]
		if f.bounds == nil {
			continue
		}
		r := f.bounds(d, s)
		if r == nil {
			t.Errorf("%s states no range", f.key)
			continue
		}
		if r.GetMin() > r.GetMax() {
			t.Errorf("%s ranges %d..%d", f.key, r.GetMin(), r.GetMax())
			continue
		}
		entry := noEntry
		if f.repeat {
			entry = 0
		}
		v := fieldValue(f, s, entry)
		n, ok := v.GetKind().(*screensharev1.FieldValue_Number)
		if !ok {
			t.Errorf("%s carries a range and reads a value that is not a number", f.key)
			continue
		}
		if n.Number < r.GetMin() || n.Number > r.GetMax() {
			t.Errorf("%s starts on %d, outside its own %d..%d", f.key, n.Number, r.GetMin(), r.GetMax())
		}
	}
}

// fieldsOffTheirStep names every slider holding a value no stop of its own sits on, one message per
// finding.
//
// A sweep stops on the multiples of its step and on the two ends of its range, so a floor of 20 with
// a step of 50 offers 20, 50, 100 and the shortest window stays reachable.
// Shared with the preset tests, which put the same question to the settings a search lands on.
func fieldsOffTheirStep(d Deps, s settings.Settings) []string {
	var off []string
	for i := range fieldTable {
		f := &fieldTable[i]
		if f.control != screensharev1.ControlKind_CONTROL_KIND_SLIDER || f.bounds == nil {
			continue
		}
		r := f.bounds(d, s)
		step := r.GetStep()
		if step <= 0 {
			step = 1
		}
		entry := noEntry
		if f.repeat {
			entry = 0
		}
		n, ok := fieldValue(f, s, entry).GetKind().(*screensharev1.FieldValue_Number)
		if !ok {
			continue
		}
		if n.Number != r.GetMin() && n.Number != r.GetMax() && n.Number%step != 0 {
			off = append(off, fmt.Sprintf("%s holds %d, which is neither an end of %d..%d nor a multiple of its step of %d",
				f.key, n.Number, r.GetMin(), r.GetMax(), step))
		}
	}
	return off
}

// A value between two stops is one the reader can leave and not get back: the sweep that moved it
// lands on the stops beside it and never on the figure it started from.
func TestEverySliderStopsOnItsOwnStep(t *testing.T) {
	for _, off := range fieldsOffTheirStep(fieldTestDeps(), settings.Defaults()) {
		t.Error(off)
	}
}

// The quantizer range follows the codec and the engine driving it, because the scales differ: one
// number is a different quality per codec, so a fixed range would clamp a wide scale to a fraction
// of itself and offer a narrow one values it refuses.
func TestTheQuantizerRangeFollowsTheCodecsOwnScale(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	s.Publish.Mode = capabilities.ModeCrf
	for _, c := range capabilities.Codecs {
		s.Publish.Codec = c.Name
		want := c.CqMaxOn(optionEngineOf(s))
		if want == 0 {
			// A row declaring no scale narrows nothing: an unwired family runs on whatever its builder sets,
			// and no number here is one the table would honour.
			want = capabilities.WidestCqScale()
		}
		if got := fieldCqBounds(d, s).GetMax(); got != int64(want) {
			t.Errorf("%s is offered a 0-%d quantizer, want 0-%d", c.Name, got, want)
		}
	}
}

// A scale binds in the mode that reads the knob and nowhere else.
// The control is greyed in the modes that send the encoder no quantizer anyway, so what this pins
// is that the range offered and the range accepted are one answer.
func TestTheQuantizerRangeIsWholeWhereTheKnobIsUnread(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	s.Publish.Codec = "libx264"

	s.Publish.Mode = capabilities.ModeCrf
	if got := fieldCqBounds(d, s).GetMax(); got != 51 {
		t.Errorf("the quantizer is offered to %d in constant quality, want x264's own 51", got)
	}
	s.Publish.Mode = capabilities.ModeLossless
	if got := fieldCqBounds(d, s).GetMax(); got != int64(capabilities.WidestCqScale()) {
		t.Errorf("the quantizer narrowed to %d in a mode that sends none", got)
	}
}

// The bitrate range takes the codec's own ceiling where it declares one.
// Such an encoder refuses the encode rather than clamping, so a target above the ceiling kills the
// publish at launch.
func TestTheBitrateRangeFollowsTheCodecsOwnCeiling(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	s.Publish.Mode = capabilities.ModeAbr
	for _, c := range capabilities.Codecs {
		s.Publish.Codec = c.Name
		want := c.BitrateLimitOn(optionEngineOf(s))
		if want == 0 {
			want = fieldRateCeiling
		}
		if got := fieldBitrateBounds(d, s).GetMax(); got != int64(want) {
			t.Errorf("%s is offered up to %d Mbit/s, want %d", c.Name, got, want)
		}
	}
}

// The ceiling is on the target the encoder is given, so it binds in the modes that aim at one.
// A constant-quality encode sends none, and narrowing there would state a limit on a number nothing
// reads.
func TestTheBitrateRangeIsWholeWhereNoTargetIsSent(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	s.Publish.Codec = "libsvtav1"

	s.Publish.Mode = capabilities.ModeAbr
	if got := fieldBitrateBounds(d, s).GetMax(); got != 100 {
		t.Errorf("the bitrate is offered to %d in a mode that aims at one, want 100", got)
	}
	s.Publish.Mode = capabilities.ModeCrf
	if got := fieldBitrateBounds(d, s).GetMax(); got != fieldRateCeiling {
		t.Errorf("the bitrate narrowed to %d in a mode that sends no target", got)
	}
}

// A capture backend that needs a privilege notes it on its own entry, and one that needs none notes
// nothing.
// The entry stays selectable either way, since the process holds the privilege or the capture dies
// at launch and nothing can tell which in advance, so the note is what makes it honest about what
// it is asking for.
//
// Which publish engine a backend runs is deliberately not noted here.
// It is a column of the backend's own catalog row, and repeating it would be a second answer to a
// question one table already answers.
func TestACaptureBackendBehindAPrivilegeSaysSoOnItsEntry(t *testing.T) {
	for _, o := range fieldRowFor(t, KeyCapture).options(fieldTestDeps(), settings.Defaults()) {
		if _, err := publish.EngineFor(o.GetValue()); err != nil {
			t.Errorf("the form offers capture backend %q, which has no publisher", o.GetValue())
			continue
		}
		grant := publish.Grant(o.GetValue())
		if !proto.Equal(o.GetNote(), grant) {
			t.Errorf("%s is noted %v, where publish says the privilege is %v", o.GetValue(), o.GetNote(), grant)
		}
	}
}

// The contract reserves the radio for a closed set whose entries carry a paragraph each, which the
// rate-control mode is and no other field here is.
// A second radio would be a decision about layout made in the wrong place.
func TestTheRateControlModeIsTheOnlyRadio(t *testing.T) {
	for _, f := range fieldTable {
		radio := f.control == screensharev1.ControlKind_CONTROL_KIND_RADIO
		if radio != (f.key == KeyMode) {
			t.Errorf("%s is drawn as %v", f.key, f.control)
		}
	}
	// What each mode's card says is the surface's, keyed by the value, so what is checked here is that
	// every mode the capability table declares reaches the control at all: a mode missing from the
	// radio is one no card can be written for.
	offered := fieldRowFor(t, KeyMode).options(fieldTestDeps(), settings.Defaults())
	if len(offered) != len(capabilities.Modes) {
		t.Errorf("the rate-control radio offers %d entries, and the table declares %d",
			len(offered), len(capabilities.Modes))
	}
	for i, o := range offered {
		if o.GetValue() != capabilities.Modes[i] {
			t.Errorf("entry %d is %q, and the table declares %q", i, o.GetValue(), capabilities.Modes[i])
		}
	}
}

// A resolved control offers everything the reader can pick before everything they cannot, so a list
// is answerable from the top on whatever machine it is drawn on.
//
// The check runs over every case availabilityCases states, because the partition is visible only
// where something is greyed.
func TestAResolvedControlOffersTheReachableEntriesFirst(t *testing.T) {
	for _, tc := range availabilityCases() {
		for _, g := range Resolve(tc.deps, tc.s).GetGroups() {
			for _, f := range g.GetFields() {
				ruledOut := ""
				for _, o := range f.GetOptions() {
					if !o.GetEnabled() {
						ruledOut = o.GetValue()
						continue
					}
					if ruledOut != "" {
						t.Errorf("%s: %s offers %q after the ruled-out %q",
							tc.name, f.GetKey(), o.GetValue(), ruledOut)
					}
				}
			}
		}
	}
}

// The partition keeps every entry and reorders nothing inside either half, which is what separates
// it from a sort: each builder's own order survives it, and an entry this combination rules out is
// still on the list with its reason (docs/field-availability.md).
func TestOrderingTheEntriesDropsNoneAndReordersNeither(t *testing.T) {
	for _, tc := range availabilityCases() {
		for i := range fieldTable {
			f := &fieldTable[i]
			if f.options == nil && f.itemOptions == nil {
				continue
			}

			// No draft here holds an audio source, so entry 0 is the row that grows the list.
			entry := noEntry
			if f.repeat {
				entry = 0
			}

			var built []string
			for _, o := range fieldOptions(f, tc.deps, tc.s, entry) {
				built = append(built, o.GetValue())
			}

			var reachable, ruledOut, offered []string
			for _, o := range resolveOptions(availabilityOf(tc.deps, tc.s), tc.deps, tc.s, f, entry) {
				offered = append(offered, o.GetValue())
				if o.GetEnabled() {
					reachable = append(reachable, o.GetValue())
					continue
				}
				ruledOut = append(ruledOut, o.GetValue())
			}

			if len(offered) != len(built) {
				t.Errorf("%s: %s offers %d of %d entries", tc.name, f.key, len(offered), len(built))
				continue
			}
			for _, half := range [][]string{reachable, ruledOut} {
				var order []int
				for _, value := range half {
					order = append(order, slices.Index(built, value))
				}
				if !slices.IsSorted(order) {
					t.Errorf("%s: %s reorders its entries: %v out of %v", tc.name, f.key, half, built)
				}
			}
		}
	}
}

func fieldOptions(f *field, d Deps, s settings.Settings, entry int) []*screensharev1.FieldOption {
	if f.repeat {
		return f.itemOptions(d, s, audioEntry(s, entry))
	}
	return f.options(d, s)
}
