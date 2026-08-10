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

// fieldDeclaredKeys is every constant in keys.go, written out rather than derived, because
// there is nothing to derive it from: Go has no reflection over a const block. It is the
// second copy of that list and the only one, which is what makes the bijection below a
// real check rather than a tautology - a key added to keys.go and to no table fails here.
var fieldDeclaredKeys = []string{
	KeyName, KeyRelayHost, KeySrtPort, KeyAPIPort, KeyRtspPort, KeyWebrtcPort,
	KeyRtmpPort, KeyHlsPort,
	KeyTransport, KeyCodec, KeyMode, KeyChroma, KeyColorRange, KeyFps, KeyCq,
	KeyBitrateM, KeyMaxrateM, KeyVbvMs, KeyGop, KeyBframes, KeyEncPreset,
	KeyCapture, KeyAudio, KeyAudioCodec, KeyDrmMap, KeyMonitor, KeyCaptureMemory,
	KeySrtPublishLatencyMs, KeySrtWatchLatencyMs,
	KeyRtspPublishProtocol, KeyRtspWatchProtocol,
	KeyUplinkMbps,
	KeyPlayerWatchTransport,
	KeyOutputResolution,
	KeyTileWatchTransport, KeyRtspWatchLatencyMs, KeyRenderChain,
}

// fieldDeclaredGroups is every group key, for the same reason.
var fieldDeclaredGroups = []string{
	GroupStream, GroupSource, GroupQuality, GroupAudio,
	GroupTransport, GroupWatch, GroupRelay,
}

// fieldTestDeps is a machine with two monitors, so a test resolves a form for hardware it
// is not running on. That is the whole reason Deps exists rather than a probe read at
// call time.
func fieldTestDeps() Deps {
	return Deps{
		Monitors: []display.Monitor{
			{Index: 0, Width: 2560, Height: 1440, RefreshHz: 144, Primary: true},
			{Index: 1, Width: 1920, Height: 1080, RefreshHz: 60},
		},
	}
}

// fieldRowFor finds one row of the table, failing the test where none carries the key.
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

// fieldOptionValues is the values one row's builder offers, in the order it offers them.
func fieldOptionValues(t *testing.T, key string) []string {
	t.Helper()
	f := fieldRowFor(t, key)
	if f.options == nil {
		t.Fatalf("%s offers no options", key)
	}
	var out []string
	for _, o := range f.options(fieldTestDeps(), settings.Defaults()) {
		out = append(out, o.GetValue())
	}
	return out
}

// A shell binds its widgets by key and a capability gap names the control it greys by the
// same key, so a key with no row is a gap pointing at nothing and a row with a key no
// constant declares is a control nothing can ever point at. The two lists are therefore
// one list, and this is where they are held to it.
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

// resolveField calls the value function on every row it renders, so a row without one is
// a panic on the first resolve rather than a control that renders empty.
func TestEveryRowShowsAValue(t *testing.T) {
	s := settings.Defaults()
	for _, f := range fieldTable {
		if f.value == nil {
			t.Errorf("%s has no value function", f.key)
			continue
		}
		if v := f.value(s); v == nil || v.GetKind() == nil {
			t.Errorf("%s reads no value out of the defaults", f.key)
		}
	}
}

// A row assigned to a group no groups entry carries renders nowhere: resolveGroups walks
// the groups and picks the rows naming each, so the field would be silently absent from
// every screen.
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

// resolveGroups drops a group with no fields, so a heading no row names is a heading that
// never appears. Declaring one is then either a row that was forgotten or a group that
// should not exist, and both are worth failing on.
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

// Every group is declared once and under a key the form spells, since the key is the
// whole of a group on the wire: the heading over it and the paragraph under it are
// looked up by that key on the surface that draws it, and one the surface has never
// heard of would render as an unnamed run of fields.
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

// The contract fills options for a select and a radio, a range for a number and a
// slider, both for the number that carries a ladder, and leaves each empty on the
// controls it does not apply to. A select with no options is a dropdown a shell cannot
// open; a number with no range is a field with no ends, which the contract says a shell
// must read as unbounded rather than as zero; and a number-select missing either half is
// one of the two ordinary controls mislabelled as the combined one.
func TestASelectOffersOptionsAndANumberStatesARange(t *testing.T) {
	for _, f := range fieldTable {
		switch f.control {
		case screensharev1.ControlKind_CONTROL_KIND_SELECT,
			screensharev1.ControlKind_CONTROL_KIND_RADIO:
			if f.options == nil {
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
			if f.options == nil {
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

// The ladder is a shortcut and not the domain: every step is a rate the range admits, so
// picking one can never write a value the same form would refuse. It is the claim the
// combined control rests on, and the two halves are stated in two places.
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

// A saved rate the ladder does not carry is offered all the same, so the closed control
// shows the rate the stream is captured at rather than the nearest step to it.
func TestTheFrameRateLadderCarriesASavedRateOffIt(t *testing.T) {
	s := settings.Defaults()
	s.Publish.Fps = 37

	if !slices.ContainsFunc(optionFpsPresets(fieldTestDeps(), s), func(o *screensharev1.FieldOption) bool {
		return o.GetValue() == "37"
	}) {
		t.Error("the frame rate ladder drops a saved rate that is not one of its steps")
	}
}

// A unit says what a number means, and every quantity here is one. It is an enum and
// not a spelling: how "Mbit/s" is set beside its figure is typography, and a field
// carrying one string could not tell a surface which half was which.
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
	}
	for key, want := range units {
		if got := fieldRowFor(t, key).unit; got != want {
			t.Errorf("%s carries unit %v, want %v", key, got, want)
		}
	}
	// A field that is not a quantity states no unit, so a surface never draws one beside
	// a stream name or a codec.
	for _, f := range fieldTable {
		if _, quantity := units[f.key]; quantity {
			continue
		}
		if f.unit != screensharev1.Unit_UNIT_UNSPECIFIED {
			t.Errorf("%s is not a quantity and carries unit %v", f.key, f.unit)
		}
	}
}

// The verdict on an option is availability's alone. A builder that pre-enabled an entry
// would be a second place deciding what is greyed, and resolveField overwrites both
// fields anyway, so a value set here is either ignored or a disagreement waiting to be
// read as the truth.
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

// A shell names an entry by its value and sends the same value back, so two entries
// sharing one are a control with two ways to mean the same thing - a repair that can
// never settle. The empty value is legal on exactly one control, the output resolution,
// where it means the capture reaches the encoder unscaled.
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

// The point of the whole package: every value a control offers comes off a domain table,
// so the form cannot offer what the encoder refuses and cannot withhold what it accepts.
// A list typed into this package would pass every other test here and fail this one.
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
		// fieldTestDeps names no platform, which is what the table answers with every
		// source offered; the platforms that serve fewer are the greying test's, since a
		// source a machine cannot serve is greyed here rather than left out.
		{KeyAudio, platform.AudioSourceIDs(platform.Info{})},
		{KeyPlayerWatchTransport, transport.WatchNames(capabilities.EngineFfmpeg)},
	}
	for _, c := range cases {
		if got := fieldOptionValues(t, c.key); !slices.Equal(got, c.table) {
			t.Errorf("%s offers %v, want the table's %v", c.key, got, c.table)
		}
	}
}

// An audio source's note is what serves it here, read off the platform table rather than
// written into the paragraph beside it.
//
// The mechanism is the part that differs per platform, so a paragraph naming one
// platform's would be read on the other two as a description of what their machine is
// doing. A machine that serves the source therefore names what serves it, and one that
// does not names nothing and carries the greying's sentence instead.
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

// The pixel formats have no table of their own: a chroma is a fact about a codec, so the
// union of the rows is the value space. A format some codec codes and the form does not
// offer is unreachable; one the form offers and no codec codes is greyed for every
// selection, which is a dead entry rather than a teaching one.
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

// The publish leg offers what either engine can serialize, not what the running one can.
// A transport this capture backend's engine lacks is one the neighbouring backend has, so
// the entry stays and is greyed with the engine named; a protocol no engine ingests is
// absent, since no choice on this screen could lift the reason.
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

// The monitor list is the enumeration plus whatever the settings already name. A stale
// selection is what the user has to see in order to move off it, so it is present even
// when the machine no longer reports that output.
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

// The resolution ladder is derived from the captured monitor rather than listed, so
// selecting another screen produces another ladder, every entry names what it was scaled
// from, and no entry is an upscale.
func TestTheOutputResolutionLadderFollowsTheCapturedMonitor(t *testing.T) {
	d := fieldTestDeps()
	s := settings.Defaults()
	s.Publish.Monitor = 0

	options := fieldRowFor(t, KeyOutputResolution).options(d, s)
	var values []string
	for _, o := range options {
		values = append(values, o.GetValue())
	}
	want := []string{"", "1920x1080", "1600x900", "1280x720", "960x540"}
	if !slices.Equal(values, want) {
		t.Errorf("the ladder off a 2560x1440 monitor is %v, want %v", values, want)
	}
	// Every scaled entry carries the size it was derived from, so a reader is never
	// asked to work out where 1600x900 came from. The unscaled entry carries none: it
	// is the source size, and the monitor's own catalog row is what says what that is.
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

	// The second monitor is shorter, so the ladder off it is shorter too: a step at or
	// above the source's own height would be an upscale.
	s.Publish.Monitor = 1
	values = values[:0]
	for _, o := range fieldRowFor(t, KeyOutputResolution).options(d, s) {
		values = append(values, o.GetValue())
	}
	if want := []string{"", "1600x900", "1280x720", "960x540"}; !slices.Equal(values, want) {
		t.Errorf("the ladder off a 1920x1080 monitor is %v, want %v", values, want)
	}

	// A monitor enumeration reported nothing for leaves the unscaled entry alone: there is
	// no source size to scale from, and absolute sizes would be a claim about a screen this
	// machine cannot measure.
	s.Publish.Monitor = 9
	values = values[:0]
	for _, o := range fieldRowFor(t, KeyOutputResolution).options(d, s) {
		values = append(values, o.GetValue())
	}
	if want := []string{""}; !slices.Equal(values, want) {
		t.Errorf("the ladder off an unenumerated monitor is %v, want %v", values, want)
	}
}

// Every width the ladder offers is even. Every chroma subsampling this app encodes in
// needs one, so an odd width is a scaler failure rather than a picture.
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

// The chroma ladder is the one presentation decision left in this package: which order
// the pixel formats are offered in, most colour detail kept first. It is an argument
// about the trade rather than about wording, which is why it survived the move - but a
// step naming a format no codec codes would silently drop out of the list it is meant to
// order, and a coded format the ladder forgets lands after the ones it names.
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

// The colour range is one of the four value sets no Go table exports, so this is what
// holds the list here against the domain: a codec that cannot encode at a range declares
// a gap on it, and a gap on a value the form never offers states a reason for an option
// that was never on offer.
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

// The RTP lower transports are another of the four. The transport package declares them
// as the watch leg's knob choices, which is the one place they cross a package boundary,
// so both legs' lists are held against it here even though neither is built from it.
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

// The NVENC ladder is the last of the four. Nothing exports it, so what is checked is its
// shape - seven contiguous steps from p1 - and that the value a fresh installation starts
// on is one of them.
func TestTheEncoderPresetLadderIsContiguousFromTheFirstStep(t *testing.T) {
	offered := fieldOptionValues(t, KeyEncPreset)
	for i, value := range offered {
		if want := "p" + strconv.Itoa(i+1); value != want {
			t.Errorf("ladder step %d is %q, want %q", i, value, want)
		}
	}
	if !slices.Contains(offered, settings.Defaults().Publish.EncPreset) {
		t.Errorf("the ladder %v does not carry the default preset %q", offered, settings.Defaults().Publish.EncPreset)
	}
}

// Every closed set the settings start on is a set the form offers. A default the form
// cannot show is a first launch that opens on a value the user cannot pick again, which
// is the same disagreement between the tables and the screen the package exists to
// prevent - read from the other end.
func TestTheDefaultsAreValuesTheFormOffers(t *testing.T) {
	s := settings.Defaults()
	cases := map[string]string{
		KeyCapture:              s.Publish.Capture,
		KeyCaptureMemory:        s.Publish.CaptureMemory,
		KeyDrmMap:               s.Publish.DrmMap,
		KeyCodec:                s.Publish.Codec,
		KeyChroma:               s.Publish.Chroma,
		KeyColorRange:           s.Publish.ColorRange,
		KeyMode:                 s.Publish.Mode,
		KeyEncPreset:            s.Publish.EncPreset,
		KeyAudio:                s.Publish.Audio,
		KeyAudioCodec:           s.Publish.AudioCodec,
		KeyTransport:            s.Publish.Transport,
		KeyPlayerWatchTransport: s.Viewer.PlayerWatchTransport,
		KeyRtspPublishProtocol:  s.Publish.RtspPublishProtocol,
		KeyRtspWatchProtocol:    s.Viewer.RtspWatchProtocol,
	}
	for key, value := range cases {
		if offered := fieldOptionValues(t, key); !slices.Contains(offered, value) {
			t.Errorf("%s starts on %q, which the form does not offer: %v", key, value, offered)
		}
	}
}

// Every range admits the value a fresh installation starts on. A default outside its own
// control's ends is a slider that opens pinned to one end, or a number field that reports
// the user's untouched setting as out of range.
func TestEveryRangeAdmitsTheDefaultSettings(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	for _, f := range fieldTable {
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
		v := f.value(s)
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

// The quantizer range follows the encoder and the engine that drives it, because the
// scales differ: the same number is a different quality per codec, so a fixed range would
// clamp a wide scale to a fraction of itself and offer a narrow one values it refuses.
func TestTheQuantizerRangeFollowsTheCodecsOwnScale(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
	for _, c := range capabilities.Codecs {
		s.Publish.Codec = c.Name
		want := c.CqMaxOn(optionEngineOf(s))
		if want == 0 {
			want = fieldAnchorCq
		}
		if got := fieldCqBounds(d, s).GetMax(); got != int64(want) {
			t.Errorf("%s is offered a 0-%d quantizer, want 0-%d", c.Name, got, want)
		}
	}
}

// The bitrate range narrows to the codec's own ceiling where it declares one. An encoder
// with a ceiling refuses the encode rather than clamping, so a target above it is a
// publish that dies at launch.
func TestTheBitrateRangeFollowsTheCodecsOwnCeiling(t *testing.T) {
	d, s := fieldTestDeps(), settings.Defaults()
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

// A capture backend behind a privilege says so on its own entry, and one behind none
// says nothing. The entry stays selectable either way - the process holds the privilege
// or the capture dies at launch, and nothing can tell which in advance - so the note is
// what makes it honest about what it is asking for.
//
// Which publish engine a backend runs is deliberately not here. It is a column of the
// backend's own catalog row, and a note repeating it would be a second answer to a
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

// The one radio is the rate-control mode, which is what the contract reserves that
// control for: the few choices carrying a paragraph each. Every other closed set is a
// select, so a second radio is a decision about layout made in the wrong place.
func TestTheRateControlModeIsTheOnlyRadio(t *testing.T) {
	for _, f := range fieldTable {
		radio := f.control == screensharev1.ControlKind_CONTROL_KIND_RADIO
		if radio != (f.key == KeyMode) {
			t.Errorf("%s is drawn as %v", f.key, f.control)
		}
	}
	// What each mode's card says is the surface's, keyed by the value below, so what is
	// checked here is that every mode the capability table declares reaches the control
	// at all: a mode missing from the radio is one no card can be written for.
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

// A resolved control offers everything the reader can pick before everything they
// cannot, so a list is answerable from the top on whatever machine it is drawn on.
//
// The check runs over every case availabilityCases states, because the partition is only
// visible where something is greyed: on a Linux session the Windows capture backends
// sink, on a Windows one the macOS backend does, and on a machine with no NVIDIA encoder
// the NVENC codecs do.
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

// The partition keeps every entry and reorders nothing inside either half, which is what
// separates it from a sort: the chroma ladder, the codec table's order and the capture
// registry's order all survive it, and an entry this combination rules out is still on
// the list with its reason (docs/field-availability.md).
func TestOrderingTheEntriesDropsNoneAndReordersNeither(t *testing.T) {
	for _, tc := range availabilityCases() {
		for i := range fieldTable {
			f := &fieldTable[i]
			if f.options == nil {
				continue
			}

			var built []string
			for _, o := range f.options(tc.deps, tc.s) {
				built = append(built, o.GetValue())
			}

			var reachable, ruledOut, offered []string
			for _, o := range resolveOptions(tc.deps, tc.s, f) {
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
