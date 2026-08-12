package form

import (
	"reflect"
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// repairChanged names every settings field that reads differently between two drafts, as
// the contract names them.
//
// It walks the wire message rather than the Go struct for the same reason Repair does: a
// field key is that message's own field name, so the answer is derived from the contract
// instead of from a list beside it that could disagree with the one under test.
func repairChanged(before, after settings.Settings) []string {
	from, to := wire.Settings(before).ProtoReflect(), wire.Settings(after).ProtoReflect()

	// Two levels, because a field key is two names: the group, then the field in it.
	// Comparing the groups alone would name a whole group as the thing that moved.
	var changed []string
	groups := from.Descriptor().Fields()
	for i := range groups.Len() {
		g := groups.Get(i)
		fromGroup, toGroup := from.Get(g).Message(), to.Get(g).Message()
		fields := g.Message().Fields()
		for j := range fields.Len() {
			f := fields.Get(j)
			if !fromGroup.Get(f).Equal(toGroup.Get(f)) {
				changed = append(changed, string(g.Name())+"."+string(f.Name()))
			}
		}
	}
	return changed
}

// repairCases are drafts worth walking: a legal one, one stranded on every dimension the
// cascade runs through, the pair whose device path converts nothing, and a machine whose
// engine can run no encoder at all.
func repairCases() []availabilityCase {
	linuxX11 := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	linuxWayland := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	windows := Deps{Platform: platform.Info{OS: "windows"}}

	stranded := availabilityDraft("ddagrab", "hevc_amf", "yuv422p", "rtmp")

	losslessAv1 := availabilityDraft("x11grab", "av1_nvenc", "yuv420p", "rtsp")
	losslessAv1.Publish.Mode = capabilities.ModeLossless

	encoderColour := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
	encoderColour.Publish.CaptureMemory = gpupath.MemoryGpuEncoderColor

	demandsDevice := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	demandsDevice.Publish.CaptureMemory = gpupath.MemoryGpu

	noTooling := linuxX11
	noTooling.Encoders = encoders.Availability{
		Unprobed: map[string]string{capabilities.EngineFfmpeg: "ffmpeg not found on PATH"},
	}

	return []availabilityCase{
		{"a draft the tables already accept", linuxX11,
			availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
		{"a capture backend this session cannot run", linuxWayland, stranded},
		{"a rate-control mode this encoder has no form of", linuxX11, losslessAv1},
		{"the device path that converts nothing", windows, encoderColour},
		{"a device path demanded of a pair that has none", linuxX11, demandsDevice},
		{"a hand-edited settings file", linuxX11,
			availabilityDraft("no-such-capture", "no-such-codec", "no-such-chroma", "no-such-transport")},
		{"an engine that can run no encoder here", noTooling,
			availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
	}
}

// The repair is a normalize step, so it has to be a fixed point: a shell calls ResolveForm
// on every keystroke and adopts the settings it answers with, and a walk whose second pass
// moved something again would leave the form resolving to a different draft each time it
// was asked about the one it just returned.
//
// It is also what says the walk cannot spin. A dimension repaired against a value a later
// dimension then replaces would show up here as a second pass with work left to do.
func TestARepairedDraftRepairsToItself(t *testing.T) {
	for _, tc := range repairCases() {
		once, _ := Repair(tc.deps, tc.s)
		twice, again := Repair(tc.deps, once)

		if !reflect.DeepEqual(twice, once) {
			t.Errorf("%s: repairing a repaired draft moved %v", tc.name, repairChanged(once, twice))
		}
		if len(again) != 0 {
			t.Errorf("%s: repairing a repaired draft named %v as repaired", tc.name, again)
		}
	}
}

// The pointer walks the same way, and it is the first field whose whole availability is a
// rule rather than a converted gap. A scanout capture cannot draw the pointer at all, so a
// draft carrying the default arrives on a backend that has no form of it.
func TestThePointerWalksOffABackendThatCannotDrawIt(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	draft := availabilityDraft("kmsgrab", "hevc_nvenc", "yuv420p", "rtsp")
	draft.Publish.Cursor = cursor.Embedded

	s, repaired := Repair(deps, draft)

	if s.Publish.Cursor == cursor.Embedded {
		t.Error("a pointer the capture cannot draw survived the repair")
	}
	if enabled, reason := optionState(deps, s, KeyCursor, s.Publish.Cursor, noEntry); !enabled {
		t.Errorf("the repair landed on pointer mode %q, which the same evaluation greys: %s", s.Publish.Cursor, reason)
	}
	if !slices.Contains(repaired, KeyCursor) {
		t.Errorf("the repaired list is %v, which does not name the field that moved", repaired)
	}
}

// A value the tables forbid comes back as one they accept, and it is the same evaluation
// that decides both: a repair landing on a value the form would grey is the one
// disagreement between the form and the publish this package exists to prevent.
func TestAStrandedValueWalksToALegalOne(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	draft := availabilityDraft("x11grab", "av1_nvenc", "yuv420p", "rtsp")
	draft.Publish.Mode = capabilities.ModeLossless

	if _, gap := mustCodec(t, "av1_nvenc").OptionGap(
		capabilities.EngineFfmpeg, capabilities.OptionMode, capabilities.ModeLossless); !gap {
		t.Fatal("av1_nvenc codes lossless, so this draft is no longer the stranded one this test names")
	}

	s, repaired := Repair(deps, draft)

	if s.Publish.Mode == capabilities.ModeLossless {
		t.Error("a rate-control mode the encoder has no form of survived the repair")
	}
	if enabled, reason := optionState(deps, s, KeyMode, s.Publish.Mode, noEntry); !enabled {
		t.Errorf("the repair landed on rate control %q, which the same evaluation greys: %s", s.Publish.Mode, reason)
	}
	if !slices.Contains(repaired, KeyMode) {
		t.Errorf("the repaired list is %v, which does not name the field that moved", repaired)
	}
}

// The list a shell is handed has to be exactly what changed: naming a field that did not
// move tells the user their choice was overridden when it was not, and leaving one out
// rewrites what they typed in silence.
func TestTheRepairedListNamesExactlyTheFieldsThatMoved(t *testing.T) {
	for _, tc := range repairCases() {
		s, repaired := Repair(tc.deps, tc.s)

		want := repairChanged(tc.s, s)
		got := slices.Clone(repaired)
		slices.Sort(want)
		slices.Sort(got)

		if !slices.Equal(got, want) {
			t.Errorf("%s: the repair named %v and moved %v", tc.name, got, want)
		}
	}
}

// The cascade in one call. The capture backend fixes the publish engine, the engine
// decides which transports can be serialized, the transport decides which bitstream
// formats reach the relay, and the codec decides which pixel formats reach the encoder.
// So a capture backend this session cannot run strands the three below it, and one
// ResolveForm has to settle all four: a shell that had to call twice would draw the
// intermediate state once.
func TestACaptureChangeCascadesThroughTransportCodecAndChroma(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	// ddagrab is Windows-only, rtmp has no GStreamer publish sink, the AMF family has no
	// GStreamer element at all, and 4:2:2 is the two software H.26x encoders' alone. Each
	// is legal where the one above it left off and stranded once it moves.
	draft := availabilityDraft("ddagrab", "hevc_amf", "yuv422p", "rtmp")

	s, repaired := Repair(deps, draft)

	for _, key := range []string{KeyCapture, KeyTransport, KeyCodec, KeyChroma} {
		if !slices.Contains(repaired, key) {
			t.Errorf("the repaired list %v does not name %s, which the cascade had to move", repaired, key)
		}
	}

	if available, reason := publish.Available(s.Publish.Capture, deps.Platform); !available {
		t.Errorf("the repair landed on capture backend %q, which this session cannot run: %s", s.Publish.Capture, reason)
	}
	allowed, err := publish.TransportsFor(s.Publish.Capture)
	if err != nil {
		t.Fatalf("the repair landed on capture backend %q, which has no publisher: %v", s.Publish.Capture, err)
	}
	if !slices.Contains(allowed, s.Publish.Transport) {
		t.Errorf("the repair landed on transport %q, which %s cannot carry", s.Publish.Transport, s.Publish.Capture)
	}
	for _, key := range []string{KeyCodec, KeyChroma} {
		value := s.Publish.Codec
		if key == KeyChroma {
			value = s.Publish.Chroma
		}
		if enabled, reason := optionState(deps, s, key, value, noEntry); !enabled {
			t.Errorf("the repair landed on %s %q, which the same evaluation greys: %s", key, value, reason)
		}
	}
}

// A draft the tables accept is left exactly as it was. The repair exists to move a value
// the tables forbid, and one that moves a legal value is one that overrides a choice the
// user was entitled to make.
func TestADraftTheTablesAcceptIsNotRepaired(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")

	s, repaired := Repair(deps, draft)

	if !reflect.DeepEqual(s, draft) {
		t.Errorf("a legal draft was repaired on %v", repairChanged(draft, s))
	}
	if len(repaired) != 0 {
		t.Errorf("a legal draft was reported repaired on %v", repaired)
	}
}

// The one case the contract explicitly allows a control to show a value its own
// evaluation would refuse: a dimension with nothing legal left keeps what it has, so the
// field stays disabled with its reason rather than holding a value picked out of the
// same set that greys it.
func TestADimensionWithNothingLegalLeftKeepsTheValueItHas(t *testing.T) {
	deps := Deps{
		Platform: platform.Info{OS: "linux", Display: "x11"},
		Encoders: encoders.Availability{
			Unprobed: map[string]string{capabilities.EngineFfmpeg: "ffmpeg not found on PATH"},
		},
	}
	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")

	s, repaired := Repair(deps, draft)

	if s.Publish.Codec != draft.Publish.Codec {
		t.Errorf("the codec moved to %q on an engine that can run none of them", s.Publish.Codec)
	}
	if slices.Contains(repaired, KeyCodec) {
		t.Errorf("the repaired list %v names the codec, which had nowhere to go", repaired)
	}
}

// mustCodec reads a row the test states a fact about, and fails on a name the table no
// longer carries rather than carrying a zero row into the assertion.
func mustCodec(t *testing.T, name string) capabilities.Codec {
	t.Helper()
	c, ok := capabilities.Get(name)
	if !ok {
		t.Fatalf("the capability table carries no codec %q", name)
	}
	return c
}

// A settings field the walk can reach has to resolve to a group and a field in it, or
// the lookup that writes the repair back would find nothing.
//
// A repeated control resolves through an entry rather than through its template: the
// template names the control and an index names the value, which is what a shell binds and
// what the walk writes through.
func TestEveryRepairableFieldNamesASettingsField(t *testing.T) {
	m := wire.Settings(settings.Defaults())

	for _, key := range availabilityAllKeys {
		if keyRepeats(key) {
			key = indexedKey(key, 0)
		}
		if _, _, ok := settingsField(m, key); !ok {
			t.Errorf("field key %q names no group and field of the settings message", key)
		}
	}
}

// A codec whose bitrate ceiling sits under the settings' own default is the case a
// numeric repair exists for. libsvtav1 accepts 100 Mbit/s and settings.Defaults asks for
// more, so selecting it leaves a draft capabilities.Validate refuses - and a number has no
// entry to grey, so nothing on the form would say so.
func TestABitrateAboveTheCodecCeilingIsBroughtDownToIt(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	// RTSP rather than SRT: MPEG-TS has no mapping for AV1, so an SRT publish leg would
	// have the codec repaired out from under the ceiling this test is about.
	draft := availabilityDraft("x11grab", "libsvtav1", "yuv420p", "rtsp")
	draft.Publish.Mode = capabilities.ModeCbr
	draft.Publish.BitrateM = 150
	draft.Publish.MaxrateM = 200

	codec, ok := capabilities.Get("libsvtav1")
	if !ok {
		t.Fatal("libsvtav1 is a row of the codec table")
	}
	ceiling := codec.BitrateLimitOn(capabilities.EngineFfmpeg)
	if ceiling == 0 || ceiling >= draft.Publish.BitrateM {
		t.Fatalf("this test needs a ceiling under the draft: ceiling %d, draft %d", ceiling, draft.Publish.BitrateM)
	}

	out, repaired := Repair(d, draft)

	if out.Publish.Codec != "libsvtav1" {
		t.Fatalf("the draft was repaired off the codec under test, onto %q", out.Publish.Codec)
	}
	if out.Publish.BitrateM != ceiling {
		t.Errorf("bitrate = %d, want the codec's ceiling %d", out.Publish.BitrateM, ceiling)
	}
	if out.Publish.MaxrateM > ceiling {
		t.Errorf("maxrate = %d, want no more than the codec's ceiling %d", out.Publish.MaxrateM, ceiling)
	}
	if !slices.Contains(repaired, KeyBitrateM) {
		t.Errorf("repaired = %v, want it to name %s", repaired, KeyBitrateM)
	}
}

// The quantizer scale is the codec's own, so moving between codecs moves the ceiling
// under a value that was legal where it was set.
func TestAQuantizerOffTheTopOfTheScaleIsBroughtDownToIt(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	draft.Publish.Cq = 200

	codec, ok := capabilities.Get("libx264")
	if !ok {
		t.Fatal("libx264 is a row of the codec table")
	}
	ceiling := codec.CqMaxOn(capabilities.EngineFfmpeg)

	out, repaired := Repair(d, draft)

	if out.Publish.Cq != ceiling {
		t.Errorf("cq = %d, want the top of the codec's scale %d", out.Publish.Cq, ceiling)
	}
	if !slices.Contains(repaired, KeyCq) {
		t.Errorf("repaired = %v, want it to name %s", repaired, KeyCq)
	}
}

// A burst ceiling under the target it is a ceiling for is not a ceiling. The target is
// what the user chose, so the ceiling is what follows it.
func TestABurstCeilingUnderItsTargetIsRaisedToIt(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	draft.Publish.Mode = capabilities.ModeVbr
	draft.Publish.BitrateM = 40
	draft.Publish.MaxrateM = 10

	out, repaired := Repair(d, draft)

	if out.Publish.BitrateM != 40 {
		t.Errorf("bitrate = %d, want the target left alone at 40", out.Publish.BitrateM)
	}
	if out.Publish.MaxrateM != 40 {
		t.Errorf("maxrate = %d, want it raised to the target", out.Publish.MaxrateM)
	}
	if !slices.Contains(repaired, KeyMaxrateM) {
		t.Errorf("repaired = %v, want it to name %s", repaired, KeyMaxrateM)
	}
}

// The audio codec is read only where a source is selected. On a silent stream it reaches
// no encoder and no transport, so a leg that cannot carry the stored codec does not make
// the stored choice wrong - there is no track for it to be wrong about.
func TestASilentStreamKeepsItsStoredAudioCodec(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "rtmp")
	draft.Publish.AudioSources = nil
	draft.Publish.AudioCodec = "opus"

	out, repaired := Repair(d, draft)

	if out.Publish.AudioCodec != "opus" {
		t.Errorf("audioCodec = %q, want the stored choice kept on a stream with no track", out.Publish.AudioCodec)
	}
	if slices.Contains(repaired, KeyAudioCodec) {
		t.Errorf("repaired = %v, want it not to name %s on a silent stream", repaired, KeyAudioCodec)
	}
}

// A settings file names a source the machine it was moved to does not serve, and the
// repair walks it onto one that machine does.
//
// It is the platform table that decides both ends: the same rows the form greys the
// entry with are the rows this walks over, so the value a repair lands on is a value the
// same evaluation leaves offered. A repair reaching for the absent source by name would
// be repair.go holding an opinion about which source is safe, which is the rule the table
// exists to carry (docs/domain-model.md, "The second-track capture sources").
func TestASourceThisMachineDoesNotServeWalksToOneItDoes(t *testing.T) {
	entry := indexedKey(KeyAudioSource, 0)
	for _, info := range []platform.Info{{OS: "windows"}, {OS: "darwin"}} {
		d := Deps{Platform: info}
		draft := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
		draft.Publish.AudioSources = settings.Recording(platform.AudioSourceDesktop)

		out, repaired := Repair(d, draft)

		// The kind walks to the absent one, which takes the entry off the list: an entry
		// naming no kind records nothing, and a machine that serves none of them is a
		// machine with nothing to record.
		if len(out.Publish.AudioSources) != 0 {
			t.Errorf("%s: audio sources = %+v, want the unserved entry taken off",
				info.OS, out.Publish.AudioSources)
		}
		if !slices.Contains(repaired, entry) {
			t.Errorf("%s: repaired = %v, want it to name %s", info.OS, repaired, entry)
		}
	}

	// The same draft on the platform that serves it is left alone, so the walk is a
	// repair rather than a control that never keeps what it is given.
	d := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	draft := availabilityDraft("portal", "libx264", "yuv420p", "rtsp")
	draft.Publish.AudioSources = settings.Recording(platform.AudioSourceDesktop)

	out, repaired := Repair(d, draft)
	if len(out.Publish.AudioSources) != 1 || out.Publish.AudioSources[0].Source != platform.AudioSourceDesktop {
		t.Errorf("audio sources = %+v on a session that serves desktop audio, want the entry kept",
			out.Publish.AudioSources)
	}
	if slices.Contains(repaired, entry) {
		t.Errorf("repaired = %v, want it not to name %s where the platform serves it", repaired, entry)
	}
}

// Every repair above has to be a fixed point too, or a form asked about the draft it has
// just returned would announce a repair on every keystroke.
func TestAClampedDraftClampsToItself(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libsvtav1", "yuv420p", "rtsp")
	draft.Publish.Mode = capabilities.ModeCbr
	draft.Publish.BitrateM = 150
	draft.Publish.MaxrateM = 200
	draft.Publish.Cq = 200

	once, _ := Repair(d, draft)
	twice, repaired := Repair(d, once)

	if len(repaired) != 0 {
		t.Errorf("repaired = %v, want nothing left to move", repaired)
	}
	if changed := repairChanged(once, twice); len(changed) != 0 {
		t.Errorf("a second repair moved %v", changed)
	}
}
