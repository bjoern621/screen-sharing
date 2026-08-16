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

// repairChanged names every settings field that reads differently between two drafts, as the
// contract names them.
//
// It walks the wire message for the reason Repair does: a field key is that message's own field
// name, so the answer derives from the contract rather than from a list beside it that could
// disagree with the one under test.
func repairChanged(before, after settings.Settings) []string {
	from, to := wire.Settings(before).ProtoReflect(), wire.Settings(after).ProtoReflect()

	// Two levels, since a field key is two names, the group and the field in it.
	// Comparing groups alone would name a whole group as the thing that moved.
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

// repairCases are the drafts worth walking: a legal one, one stranded on every dimension the
// cascade runs through, the pair whose device path converts nothing, and a machine whose engine can
// run no encoder at all.
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

	// Nothing here names anything the tables carry, which is a settings file somebody edited by hand.
	handEdited := availabilityDraft("no-such-capture", "libx264", "no-such-chroma", "no-such-transport")
	handEdited.Publish.Encoder = "no-such-encoder"

	return []availabilityCase{
		{"a draft the tables already accept", linuxX11,
			availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
		{"a capture backend this session cannot run", linuxWayland, stranded},
		{"a rate-control mode this encoder has no form of", linuxX11, losslessAv1},
		{"the device path that converts nothing", windows, encoderColour},
		{"a device path demanded of a pair that has none", linuxX11, demandsDevice},
		{"a hand-edited settings file", linuxX11, handEdited},
		{"an engine that can run no encoder here", noTooling,
			availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
	}
}

// A normalize step has to be a fixed point: a shell calls ResolveForm on every keystroke and adopts
// the settings it answers with, so a second pass that moved something would resolve to a different
// draft each time the form was asked about the one it just returned.
//
// It also says the walk cannot spin.
// A dimension repaired against a value a later dimension replaces shows up here as a second pass
// with work left to do.
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

// The pointer walks the same way, and its availability is a rule rather than a converted gap.
// A scanout capture draws no pointer at all, so a draft carrying the default arrives on a backend
// with no form of it.
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

// One evaluation decides both ends of this: a repair landing on a value the form would grey is the
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

// The list a shell is handed is exactly what changed.
// Naming a field that did not move says a choice was overridden when it was not, and leaving one
// out rewrites what the user typed in silence.
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

// The whole cascade in one call.
// The capture backend fixes the publish engine, the engine decides which transports serialize,
// the transport decides which bitstream formats reach the relay, and the codec decides which pixel
// formats reach the encoder.
// A capture backend this session cannot run therefore strands the three below it, and one
// ResolveForm settles all four: a shell that had to call twice would draw the intermediate state.
func TestACaptureChangeCascadesThroughTransportCodecAndChroma(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	// ddagrab runs on Windows alone, rtmp has no GStreamer publish sink, the AMF family has no
	// GStreamer element at all, and 4:2:2 belongs to the two software H.26x encoders.
	// Each is legal where the one above it left off and stranded once that one moves.
	draft := availabilityDraft("ddagrab", "hevc_amf", "yuv422p", "rtmp")

	s, repaired := Repair(deps, draft)

	for _, key := range []string{KeyCapture, KeyTransport, KeyEncoder, KeyChroma} {
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
	for _, key := range []string{KeyEncoder, KeyChroma} {
		value := s.Publish.Encoder
		if key == KeyChroma {
			value = s.Publish.Chroma
		}
		if enabled, reason := optionState(deps, s, key, value, noEntry); !enabled {
			t.Errorf("the repair landed on %s %q, which the same evaluation greys: %s", key, value, reason)
		}
	}
}

// A draft the tables accept is left as it was.
// The repair moves values the tables forbid, and moving a legal one overrides a choice the user was
// entitled to make.
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

// The one case the contract allows a control to show a value its own evaluation refuses.
// A dimension with nothing legal left keeps what it has, so the field stays disabled with its
// reason rather than holding a value picked out of the set that greys it.
func TestADimensionWithNothingLegalLeftKeepsTheValueItHas(t *testing.T) {
	deps := Deps{
		Platform: platform.Info{OS: "linux", Display: "x11"},
		Encoders: encoders.Availability{
			Unprobed: map[string]string{capabilities.EngineFfmpeg: "ffmpeg not found on PATH"},
		},
	}
	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")

	s, repaired := Repair(deps, draft)

	if s.Publish.Codec() != draft.Publish.Codec() {
		t.Errorf("the codec moved to %q on an engine that can run none of them", s.Publish.Codec())
	}
	if slices.Contains(repaired, KeyEncoder) {
		t.Errorf("the repaired list %v names the encoder, which had nowhere to go", repaired)
	}
}

// mustCodec reads the row a test states a fact about.
// A name the table does not carry fails here rather than reaching the assertion as a zero row.
func mustCodec(t *testing.T, name string) capabilities.Codec {
	t.Helper()
	c, ok := capabilities.Get(name)
	if !ok {
		t.Fatalf("the capability table carries no codec %q", name)
	}
	return c
}

// A field the walk can reach resolves to a group and a field in it, or the lookup that writes the
// repair back finds nothing.
//
// A repeated control resolves through an entry rather than its template: the template names the
// control and an index names the value, which is what a shell binds and what the walk writes
// through.
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

// A codec whose bitrate ceiling sits under the settings' own default is the case the numeric repair
// exists for.
// libsvtav1's ceiling is under what settings.Defaults asks for, so selecting it leaves a draft
// capabilities.Validate refuses, and a number has no entry to grey.
func TestABitrateAboveTheCodecCeilingIsBroughtDownToIt(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	// RTSP rather than SRT: MPEG-TS carries no AV1 mapping, so an SRT leg would repair the codec out
	// from under the ceiling this test is about.
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

	if out.Publish.Codec() != "libsvtav1" {
		t.Fatalf("the draft was repaired off the codec under test, onto %q", out.Publish.Codec())
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

// A quantizer scale is the codec's own, so changing codec moves the ceiling under a value that was
// legal where it was set.
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

// A burst ceiling under its own target is not a ceiling.
// The target is the figure the user chose, so the ceiling follows it.
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

// The audio codec is read only where a source is picked.
// On a silent stream it reaches no encoder and no transport, so a leg that carries none of it makes
// the stored choice wrong about nothing.
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

// A settings file names a source the machine it was moved to does not serve, and the repair walks
// it onto one that machine does.
//
// The platform table decides both ends: the rows the form greys the entry with are the rows this
// walks over, so a repair lands on a value the same evaluation leaves offered.
// Reaching for the absent source by name would put an opinion about which source is safe in
// repair.go, where the table carries the rule (docs/domain-model.md, "The second-track capture
// sources").
func TestASourceThisMachineDoesNotServeWalksToOneItDoes(t *testing.T) {
	entry := indexedKey(KeyAudioSource, 0)
	for _, info := range []platform.Info{{OS: "windows"}, {OS: "darwin"}} {
		d := Deps{Platform: info}
		draft := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
		draft.Publish.AudioSources = settings.Recording(platform.AudioSourceDesktop)

		out, repaired := Repair(d, draft)

		// The kind walks to the absent one, which takes the entry off the list: an entry with no kind
		// records nothing, and a machine serving none of them has nothing to record.
		if len(out.Publish.AudioSources) != 0 {
			t.Errorf("%s: audio sources = %+v, want the unserved entry taken off",
				info.OS, out.Publish.AudioSources)
		}
		if !slices.Contains(repaired, entry) {
			t.Errorf("%s: repaired = %v, want it to name %s", info.OS, repaired, entry)
		}
	}

	// The same draft on a platform that serves the source is left alone, or the walk is a control that
	// never keeps what it is given rather than a repair.
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

// The numeric repairs are fixed points too, or a form asked about the draft it just returned
// announces a repair on every keystroke.
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

// Constant quality has no target for a ceiling to sit above, so the two figures cannot disagree
// there.
// Raising the ceiling to a bitrate belonging to another mode would hand a bounded quality encode
// several times the rate it was bounded to.
func TestAConstantQualityCeilingIsNotRaisedToTheBitrate(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	draft.Publish.Mode = capabilities.ModeCrf
	draft.Publish.BitrateM = 40
	draft.Publish.MaxrateM = 10

	out, repaired := Repair(d, draft)

	if out.Publish.MaxrateM != 10 {
		t.Errorf("maxrate = %d, want the stated ceiling kept at 10", out.Publish.MaxrateM)
	}
	if slices.Contains(repaired, KeyMaxrateM) {
		t.Errorf("repaired = %v, want the ceiling left alone", repaired)
	}
}

// A stored draft can name a keyframe interval no encoder on the newly chosen codec takes, and a
// draft the form would not offer is one the repair brings back inside the scale.
func TestAKeyframeIntervalAboveTheCodecCeilingIsBroughtDownToIt(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "h264_amf", "yuv420p", "rtsp")
	draft.Publish.Gop = fieldGopCeiling

	codec, ok := capabilities.Get("h264_amf")
	if !ok {
		t.Fatal("h264_amf is a row of the codec table")
	}
	ceiling := codec.GopLimitOn(capabilities.EngineFfmpeg)
	if ceiling == 0 || ceiling >= draft.Publish.Gop {
		t.Fatalf("this test needs a ceiling under the draft: ceiling %d, draft %d", ceiling, draft.Publish.Gop)
	}

	out, repaired := Repair(d, draft)

	if out.Publish.Codec() != "h264_amf" {
		t.Fatalf("the draft was repaired off the codec under test, onto %q", out.Publish.Codec())
	}
	if out.Publish.Gop != ceiling {
		t.Errorf("gop = %d, want the codec's ceiling %d", out.Publish.Gop, ceiling)
	}
	if !slices.Contains(repaired, KeyGop) {
		t.Errorf("repaired = %v, want it to name %s", repaired, KeyGop)
	}
}

// The rate buffer is stated to the encoder as the rate times the window, and a draft can hold a pair
// whose product no encoder's field takes.
// The form offers the window inside what the rate leaves, and the repair is the same limit on a
// draft that arrived from somewhere else.
func TestARateBufferAboveWhatTheEncoderHoldsIsBroughtDown(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp")
	draft.Publish.Mode = capabilities.ModeCbr
	draft.Publish.BitrateM = 2000
	draft.Publish.MaxrateM = 2000
	draft.Publish.VbvMs = fieldVbvCeiling

	out, repaired := Repair(d, draft)

	const int32Max = 2147483647
	if bits := int64(out.Publish.BitrateM) * int64(out.Publish.VbvMs) * 1000; bits > int32Max {
		t.Errorf("the repaired draft states a %d bit buffer, past the %d an encoder holds", bits, int32Max)
	}
	if out.Publish.VbvMs == draft.Publish.VbvMs {
		t.Errorf("the window is still %d ms, want it brought inside what the rate leaves", out.Publish.VbvMs)
	}
	if !slices.Contains(repaired, KeyVbvMs) {
		t.Errorf("repaired = %v, want it to name %s", repaired, KeyVbvMs)
	}
}

// A target of zero is what the control stopped offering in the modes that send one, so a draft
// carrying it arrived from a mode that sends none or from a file somebody edited.
// The walk raises it rather than leaving a stream at no rate, and names the move.
func TestATargetOfZeroIsRaisedInTheModesThatSendOne(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	for _, mode := range capabilities.Modes {
		if !capabilities.TargetsBitrate(mode) {
			continue
		}
		draft := availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp")
		draft.Publish.Mode = mode
		draft.Publish.BitrateM = 0

		out, repaired := Repair(d, draft)

		if out.Publish.BitrateM <= 0 {
			t.Errorf("in %s the repaired draft still targets %d Mbit/s", mode, out.Publish.BitrateM)
		}
		if !slices.Contains(repaired, KeyBitrateM) {
			t.Errorf("in %s repaired = %v, want it to name %s", mode, repaired, KeyBitrateM)
		}
	}
}

// Constant quality sends the encoder no target, so the field there carries whatever another mode
// left on it and a zero is not a draft to repair.
func TestATargetOfZeroStandsWhereNothingSendsIt(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp")
	draft.Publish.Mode = capabilities.ModeCrf
	draft.Publish.BitrateM = 0

	out, repaired := Repair(d, draft)

	if out.Publish.BitrateM != 0 {
		t.Errorf("constant quality moved the target to %d Mbit/s", out.Publish.BitrateM)
	}
	if slices.Contains(repaired, KeyBitrateM) {
		t.Errorf("repaired = %v, want no move of %s", repaired, KeyBitrateM)
	}
}

// The format is what every viewer has to decode, so an encoder this machine cannot run moves the
// encoder alone: the same bitstream goes out through whatever produces it here.
//
// The two controls are what makes that hold. One field naming the whole encode has a single list to
// walk, so the first entry that runs decides the bitstream as a side effect, and a machine whose
// software encoders are missing publishes a format nobody asked for.
//
// The case is the striking one: an x264 draft on a machine where NVENC is all there is.
func TestAMissingEncoderMovesTheEncoderAndNotTheFormat(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	deps.Encoders = presetOnlyFamilies(capabilities.FamilyNvenc)
	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")

	s, repaired := Repair(deps, draft)

	if s.Publish.Format != draft.Publish.Format {
		t.Errorf("the format moved from %q to %q on a machine that codes it, only elsewhere",
			draft.Publish.Format, s.Publish.Format)
	}
	if slices.Contains(repaired, KeyFormat) {
		t.Errorf("the repaired list %v names the format, which had somewhere to stay", repaired)
	}
	if s.Publish.Encoder == draft.Publish.Encoder {
		t.Errorf("the encoder stayed on %q, which this machine cannot run", s.Publish.Encoder)
	}
	if _, ok := capabilities.Row(s.Publish.Format, s.Publish.Encoder); !ok {
		t.Errorf("the repair landed on %s/%s, which addresses no row", s.Publish.Format, s.Publish.Encoder)
	}
}
