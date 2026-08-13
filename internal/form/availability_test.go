package form

import (
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// availabilityAllKeys is every field the form declares, spelled out a second time on purpose: the
// test reading it checks that the availability table and keys.go cover each other, which a list
// derived from either could not.
var availabilityAllKeys = []string{
	KeyName, KeyRelayHost, KeyRelayTls, KeyGroupKey, KeySrtPassphrase, KeySrtPort, KeyAPIPort, KeyRtspPort, KeyWebrtcPort,
	KeyRtmpPort, KeyHlsPort,
	KeyTransport, KeyCodec, KeyMode, KeyChroma, KeyColorRange, KeyFps, KeyCq,
	KeyBitrateM, KeyMaxrateM, KeyVbvMs, KeyGop, KeyBframes, KeyEffort, KeyTune,
	KeyCapture, KeyAudioSource, KeyAudioSourceDevice, KeyAudioSourceGain, KeyAudioSourceMute,
	KeyAudioCodec, KeyDrmMap, KeyMonitor, KeyCaptureMemory,
	KeyCursor,
	KeySrtPublishLatencyMs, KeySrtWatchLatencyMs,
	KeyRtspPublishProtocol, KeyRtspWatchProtocol,
	KeyUplinkMbps, KeyPlayerWatchTransport, KeyOutputResolution,
	KeyTileWatchTransport, KeyRtspWatchLatencyMs, KeyRenderChain,
}

// The value spaces the option tests walk, stated here rather than read off an option builder: the
// walk has to reach every value the settings can hold, including one no builder offers, since a
// stored settings file still reaches the greying rule for it.
var (
	availabilityChromas     = []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"}
	availabilityColorRanges = []string{"pc", "tv"}
)

// availabilityDraft is the defaults with the machine's own answers replaced by fixed ones, so a case
// reads the same on every platform the suite runs on.
// settings.Defaults picks its capture backend off runtime.GOOS, which would otherwise make half of
// these tests describe the developer.
func availabilityDraft(capture, codec, chroma, transportName string) settings.Settings {
	s := settings.Defaults()
	s.Publish.Name = "test"
	s.Publish.Capture = capture
	s.Publish.Codec = codec
	s.Publish.Chroma = chroma
	s.Publish.Transport = transportName
	s.Publish.Mode = capabilities.ModeCrf
	s.Publish.ColorRange = "tv"
	s.Viewer.PlayerWatchTransport = "srt"
	s.Publish.CaptureMemory = gpupath.MemoryAuto
	// The two ladder steps follow the codec this draft names, as the defaults, the migration and the
	// repair all set them.
	// Carrying the default codec's step onto another encoder would make every draft one the repair has
	// to move, which is not the combination a case is about.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(codec, s.Publish.Mode)
	return s
}

// availabilityCase is one machine and one draft, named for what it reaches.
type availabilityCase struct {
	name string
	deps Deps
	s    settings.Settings
}

// availabilityCases spread wide enough that every greying path in the table is taken by one of them:
// both engines, both colour verdicts of the pair table, a machine whose probe never ran, one whose
// engine has no tooling, and a settings file naming a capture backend this app has no publisher for.
func availabilityCases() []availabilityCase {
	linuxX11 := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	linuxWayland := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	windows := Deps{Platform: platform.Info{OS: "windows"}}
	macOS := Deps{Platform: platform.Info{OS: "darwin"}}

	encoderColour := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
	encoderColour.Publish.CaptureMemory = gpupath.MemoryGpuEncoderColor

	exactColour := availabilityDraft("kmsgrab", "hevc_vaapi", "yuv420p", "rtsp")
	exactColour.Publish.CaptureMemory = gpupath.MemoryGpu

	vaapiCeiling := availabilityDraft("portal", "hevc_vaapi", "yuv420p", "rtsp")
	vaapiCeiling.Publish.Mode = capabilities.ModeVbr
	vaapiCeiling.Publish.BitrateM = 20
	vaapiCeiling.Publish.MaxrateM = 200

	lossless := availabilityDraft("avfoundation", "libx264", "yuv444p", "rtsp")
	lossless.Publish.Mode = capabilities.ModeLossless

	noTooling := linuxX11
	noTooling.Encoders = encoders.Availability{
		Unprobed: map[string]string{capabilities.EngineFfmpeg: "ffmpeg not found on PATH"},
	}

	noNvenc := linuxX11
	noNvenc.Encoders = encoders.Availability{
		Usable: map[string]map[string]bool{capabilities.EngineFfmpeg: {"hevc_nvenc": false}},
	}

	return []availabilityCase{
		{"software encoding over SRT on an X11 session", linuxX11,
			availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
		{"the GStreamer engine on a Wayland session", linuxWayland,
			availabilityDraft("portal", "libx265", "gbrp", "rtsp")},
		{"the device path that converts nothing", windows, encoderColour},
		{"the device path that converts on the device", linuxX11, exactColour},
		{"a VBR ceiling the va elements cannot express", linuxWayland, vaapiCeiling},
		{"lossless on macOS", macOS, lossless},
		{"a machine whose ffmpeg is missing", noTooling,
			availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
		{"a machine with no NVIDIA encoder", noNvenc,
			availabilityDraft("x11grab", "hevc_nvenc", "yuv420p", "srt")},
		{"a capture backend from a hand-edited settings file", linuxX11,
			availabilityDraft("no-such-capture", "libx264", "yuv420p", "srt")},
		{"a codec from a hand-edited settings file", linuxX11,
			availabilityDraft("x11grab", "no-such-codec", "yuv420p", "srt")},
	}
}

// The contract form.go asserts on every field it renders: a greyed field with no sentence teaches
// nothing and reads as a bug.
func TestAGreyedFieldAlwaysCarriesAReason(t *testing.T) {
	for _, tc := range availabilityCases() {
		for _, key := range availabilityAllKeys {
			st := fieldState(tc.deps, tc.s, key, noEntry)
			if !st.enabled && st.reason == nil {
				t.Errorf("%s: %s is disabled with no reason", tc.name, key)
			}
			if st.enabled && st.reason != nil {
				t.Errorf("%s: %s is enabled and still carries a reason: %v", tc.name, key, st.reason)
			}
			if !st.enabled && st.note != nil {
				t.Errorf("%s: %s is disabled and carries a note: %v", tc.name, key, st.note)
			}
		}
	}
}

// The same contract on the option half: a greyed entry names the limit and which side has it, rather
// than saying only that the option is gone.
func TestAGreyedOptionAlwaysCarriesAReason(t *testing.T) {
	values := map[string][]string{
		KeyCapture:    publish.Captures(),
		KeyTransport:  transport.Names(),
		KeyCodec:      availabilityCodecNames(),
		KeyChroma:     availabilityChromas,
		KeyMode:       capabilities.Modes,
		KeyColorRange: availabilityColorRanges,
		// Every declared source, which is the list on every platform: the table marks the ones a session
		// here does not serve, so the Info this is asked with changes the verdicts and not the roster.
		KeyAudioSource:   platform.AudioSourceIDs(platform.Info{}),
		KeyAudioCodec:    capabilities.AudioNames(),
		KeyCaptureMemory: gpupath.Memories,
		// A watch leg is offered from the roster its receiver can reach, which is what the option list is
		// built from and what the reasons are stated against.
		KeyPlayerWatchTransport: transport.WatchNames(capabilities.EngineFfmpeg),
	}
	for _, tc := range availabilityCases() {
		for key, list := range values {
			for _, value := range list {
				enabled, reason := optionState(tc.deps, tc.s, key, value, noEntry)
				if !enabled && reason == nil {
					t.Errorf("%s: %s option %q is greyed with no reason", tc.name, key, value)
				}
				if enabled && reason != nil {
					t.Errorf("%s: %s option %q is offered and still carries a reason: %v",
						tc.name, key, value, reason)
				}
			}
		}
	}
}

// A control the key list declares and the table forgets would render as a plain enabled widget no
// rule governs and no repair moves.
func TestEveryFieldTheFormDeclaresHasAnAvailabilityRule(t *testing.T) {
	for _, key := range availabilityAllKeys {
		if _, ok := availabilityRules[key]; !ok {
			t.Errorf("field %q has no availability rule", key)
		}
	}
	for key := range availabilityRules {
		if !slices.Contains(availabilityAllKeys, key) {
			t.Errorf("availability rule for %q, which is not a field the form declares", key)
		}
	}
	for key := range availabilityOptionRules {
		if _, ok := availabilityRules[key]; !ok {
			t.Errorf("option rule for %q, which has no availability rule", key)
		}
	}
}

// The first treatment.
// A backend implementation knob with no meaning outside one selection is not rendered at all: the
// DRM download strategy belongs to the kmsgrab scanout path, and its tooltip would teach a user on
// any other backend nothing (docs/field-availability.md).
func TestABackendKnobIsHiddenAwayFromItsCaptureBackend(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	away := fieldState(deps, availabilityDraft("x11grab", "libx264", "yuv420p", "srt"), KeyDrmMap, noEntry)
	if away.visible {
		t.Error("the DRM download strategy is rendered on a capture backend that downloads no scanout buffer")
	}

	home := fieldState(deps, availabilityDraft("kmsgrab", "libx264", "yuv420p", "rtsp"), KeyDrmMap, noEntry)
	if !home.visible || !home.enabled {
		t.Errorf("under kmsgrab through system memory the DRM download strategy is %+v, want live", home)
	}
}

// The same field greys rather than hiding a second time where the run downloads nothing: it is
// already gated on the capture backend, and a second gate would make it appear and vanish while the
// user changes codecs.
func TestTheDrmDownloadStrategyIsGreyedWhereNothingIsDownloaded(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("kmsgrab", "hevc_vaapi", "yuv420p", "rtsp")
	s.Publish.CaptureMemory = gpupath.MemoryGpu

	st := fieldState(deps, s, KeyDrmMap, noEntry)
	if !st.visible {
		t.Fatal("the DRM download strategy is hidden under kmsgrab, where it belongs")
	}
	if st.enabled {
		t.Error("the DRM download strategy is live on a run that downloads nothing")
	}
}

// The third treatment on a value the machine rather than the settings rules out: a source no session
// here serves stays in the dropdown and greys with what the machine is missing.
//
// The second track is a general concept rather than one platform's implementation knob, so a user on
// Windows reads why the machine cannot hand them what it is playing instead of finding a one-entry
// control (docs/field-availability.md).
// The sentence is the platform table's rather than written here, which is what stops the form greying
// a source the catalog offered.
func TestAnUnservedAudioSourceIsOfferedAndGreyedWithThePlatformsReason(t *testing.T) {
	for _, info := range []platform.Info{{OS: "windows"}, {OS: "darwin"}} {
		deps := Deps{Platform: info}
		s := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")

		offered := false
		for _, o := range optionAudioSources(deps, s) {
			if o.GetValue() == platform.AudioSourceDesktop {
				offered = true
			}
		}
		if !offered {
			t.Errorf("%s is offered no desktop audio entry at all, so nothing on screen says why", info.OS)
		}

		enabled, reason := optionState(deps, s, KeyAudioSource, platform.AudioSourceDesktop, 0)
		_, want := platform.AudioSourceAvailable(platform.AudioSourceDesktop, info)
		if enabled {
			t.Errorf("%s offers desktop audio live, which neither publish engine can open there", info.OS)
		}
		if !proto.Equal(reason, want) {
			t.Errorf("%s greys desktop audio with %v, the platform table says %v", info.OS, reason, want)
		}
	}

	// The same entry on the platform that serves it, so the greying is a fact about the machine rather
	// than a control that is always dead.
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	s := availabilityDraft("portal", "libx264", "yuv420p", "rtsp")
	if enabled, reason := optionState(deps, s, KeyAudioSource, platform.AudioSourceDesktop, 0); !enabled {
		t.Errorf("a Linux session is refused desktop audio: %v", reason)
	}
}

// The second treatment.
// A general encoding concept the combination blocks stays rendered and greys, so a user hunting for
// the effort ladder under a VAAPI encoder reads why it is absent instead of finding a blank.
//
// The reason names the codec rather than a set of families, the ladder being the codec's own: two
// codecs of one family can declare different ones, and one declaring none says nothing about the
// other.
func TestTheEffortStepIsDisabledWhereTheCodecDeclaresNoLadder(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "h264_vaapi", "yuv420p", "srt")
	s.Publish.Mode = capabilities.ModeVbr

	st := fieldState(deps, s, KeyEffort, noEntry)
	if st.enabled {
		t.Fatal("the effort step is live under an encoder whose row declares no ladder")
	}
	if !st.visible {
		t.Error("the effort step is hidden, where a general encoding concept greys")
	}
	if codeOf(st.reason) != codecTakesNoEffortLadder {
		t.Errorf("the effort step greys with %v, want the statement naming the codec", codeOf(st.reason))
	}
	if codec := idOf(st.reason, screensharev1.TextArgName_TEXT_ARG_NAME_CODEC); codec != "h264_vaapi" {
		t.Errorf("the effort step greys naming %q, where the draft is on h264_vaapi", codec)
	}
}

// The other half of the same rule: a control greyed for a codec whose encoder takes the step would
// be the form withholding a knob that reaches the encoder, which docs/field-availability.md rules
// out.
func TestTheEffortStepIsLiveWhereTheCodecDeclaresALadder(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	s.Publish.Mode = capabilities.ModeVbr

	if st := fieldState(deps, s, KeyEffort, noEntry); !st.enabled {
		t.Errorf("x264 declares an effort ladder and its builder spends it, yet the control greys: %v",
			codeOf(st.reason))
	}
}

// A codec can declare either ladder without the other: libvpx takes an effort step and tunes for
// nothing, so its effort control is live while its tune control greys naming the codec.
func TestTheTwoLaddersAreAskedAboutSeparately(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libvpx-vp9", "yuv420p", "srt")

	c, ok := capabilities.Get(s.Publish.Codec)
	if !ok {
		t.Fatalf("no capability row for %s", s.Publish.Codec)
	}
	if len(c.Effort.Steps) == 0 || len(c.Tune.Steps) > 0 {
		t.Skipf("%s no longer declares one ladder and not the other", s.Publish.Codec)
	}

	if st := fieldState(deps, s, KeyEffort, noEntry); !st.enabled {
		t.Errorf("the effort step greys on a codec that declares a ladder: %v", codeOf(st.reason))
	}
	st := fieldState(deps, s, KeyTune, noEntry)
	if st.enabled {
		t.Fatal("the tune is live on a codec whose encoder tunes for nothing")
	}
	if codeOf(st.reason) != codecTakesNoTuneLadder {
		t.Errorf("the tune greys with %v, want the statement naming the codec", codeOf(st.reason))
	}
}

// Both engines forward both steps, so neither control greys for the engine alone: the nvcodec
// elements take the same p1-p7 ladder ffmpeg spends, so no engine rule withholds a step.
func TestBothLaddersReachBothEngines(t *testing.T) {
	linuxX11 := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	linuxWayland := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}

	for _, tc := range []availabilityCase{
		{"the ffmpeg engine", linuxX11, availabilityDraft("x11grab", "hevc_nvenc", "yuv420p", "srt")},
		{"the GStreamer engine", linuxWayland, availabilityDraft("portal", "hevc_nvenc", "yuv420p", "rtsp")},
	} {
		for _, key := range []string{KeyEffort, KeyTune} {
			if st := fieldState(tc.deps, tc.s, key, noEntry); !st.enabled {
				t.Errorf("%s: %s greys, where the encoder takes the step: %v", tc.name, key, codeOf(st.reason))
			}
		}
	}
}

// A mode that pins the step greys the control and names the step in force.
// The greying and the encode read one row, so the sentence cannot name a step the encoder is not
// running (ffmpeg.TestNvencCbrPinsTheDeclaredStep).
func TestTheEffortStepGreysWhereTheModePinsIt(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "hevc_nvenc", "yuv420p", "srt")
	s.Publish.Mode = capabilities.ModeCbr

	c, ok := capabilities.Get(s.Publish.Codec)
	if !ok {
		t.Fatalf("no capability row for %s", s.Publish.Codec)
	}
	want, _ := c.Effort.StepFor(capabilities.ModeCbr)

	st := fieldState(deps, s, KeyEffort, noEntry)
	if st.enabled {
		t.Fatal("the effort step is live in a mode that pins it")
	}
	if codeOf(st.reason) != effortPinnedByMode {
		t.Errorf("the pinned step greys with %v, want the statement naming the pin", codeOf(st.reason))
	}
	if step := idOf(st.reason, screensharev1.TextArgName_TEXT_ARG_NAME_EFFORT); step != want {
		t.Errorf("the pinned step greys naming %q, where the row declares %q", step, want)
	}
}

// The case docs/field-availability.md names outright: under software x264 in VBR the mode does use
// B-frames and the family has no property for them, so the reason is the family's and not the mode
// sentence, which would be a lie there.
func TestTheBframeCountNamesTheFamilyRatherThanTheMode(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	s.Publish.Mode = capabilities.ModeVbr

	st := fieldState(deps, s, KeyBframes, noEntry)
	if st.enabled {
		t.Fatal("the B-frame count is live under an encoder family that takes none")
	}
	if codeOf(st.reason) == bframesOffInMode {
		t.Errorf("the B-frame count greys with the mode's own statement in VBR: %v", st.reason)
	}
	if codeOf(st.reason) != bframesOnlyOnFamilies {
		t.Errorf("the B-frame count greys with %v, want the statement naming who takes one", codeOf(st.reason))
	}
	if families := idsOf(st.reason, screensharev1.TextArgName_TEXT_ARG_NAME_FAMILIES); !slices.Contains(families, capabilities.FamilyNvenc) {
		t.Errorf("the B-frame count greys naming %v, which leaves out the family that takes one", families)
	}
}

// The third treatment.
// A field that stays editable and means something its label does not describe gains a sentence and
// no greying.
// The pixel format's note is about somebody else's machine: every format has a software decoder, so
// a format no GPU takes is a viewer spending cores.
func TestThePixelFormatStaysLiveAndSaysWhatItCostsAViewer(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	hardware := fieldState(deps, availabilityDraft("x11grab", "libx264", "yuv420p", "srt"), KeyChroma, noEntry)
	if !hardware.enabled || hardware.reason != nil {
		t.Fatalf("4:2:0 H.264 greys the pixel format: %+v", hardware)
	}
	if codeOf(hardware.note) != decodesInHardware {
		t.Errorf("4:2:0 H.264 carries %v, which does not say viewers decode it on a GPU", codeOf(hardware.note))
	}

	// 4:4:4 H.264 is the case the decode table exists to state: no vendor put High 4:4:4 Predictive in
	// silicon, so every viewer decodes it on the CPU, and the control still does not grey.
	software := fieldState(deps, availabilityDraft("x11grab", "libx264", "yuv444p", "srt"), KeyChroma, noEntry)
	if !software.enabled {
		t.Fatalf("4:4:4 H.264 greys the pixel format: %+v", software)
	}
	if codeOf(software.note) != decodesOnCPU {
		t.Errorf("4:4:4 H.264 carries %v, which does not say what it costs a viewer", codeOf(software.note))
	}
}

// The other half of the note's purpose: a value the builder does send is never greyed, which would
// leave the encoder using a number the form refused to show.
func TestAForwardedKnobCarriesANoteRatherThanAGreying(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "windows"}}
	s := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
	s.Publish.Mode = capabilities.ModeCrf

	st := fieldState(deps, s, KeyBitrateM, noEntry)
	if !st.enabled {
		t.Fatalf("the bitrate greys in constant quality on NVENC, where the builder forwards it: %+v", st)
	}
	if st.note == nil {
		t.Error("the bitrate carries no note in constant quality on NVENC, where it becomes a burst ceiling")
	}
}

// The fourth treatment.
// A dropdown keeps the value a neighbouring combination allows and greys that entry alone: planar
// RGB greys where no encoder element takes it and stays selectable on the capture backends that run
// ffmpeg, which codes it.
func TestPlanarRGBIsOneOptionGreyedOnTheEngineThatCannotCodeIt(t *testing.T) {
	linuxX11 := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	linuxWayland := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}

	enabled, reason := optionState(linuxWayland,
		availabilityDraft("portal", "libx265", "yuv420p", "rtsp"), KeyChroma, "gbrp", noEntry)
	if enabled {
		t.Error("planar RGB is offered on the GStreamer engine, whose x265 element takes no GBR sink format")
	}
	if reason == nil {
		t.Error("planar RGB is greyed on the GStreamer engine with no reason")
	}

	enabled, _ = optionState(linuxX11,
		availabilityDraft("x11grab", "libx265", "yuv420p", "rtsp"), KeyChroma, "gbrp", noEntry)
	if !enabled {
		t.Error("planar RGB is greyed on the ffmpeg engine, which codes it")
	}
}

// A machine whose probe never ran is not a machine with nothing usable.
// The zero Availability is the state a form resolves in before Detect answers, so it leaves the
// codec list as the tables describe it: greying there would offer a roster that shrinks and comes
// back while the app starts.
func TestAnUnprobedMachineGreysNoCodec(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp")

	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		if _, gap := c.EngineGap(capabilities.EngineFfmpeg); gap {
			continue
		}
		if !transport.CanPublishFormat(s.Publish.Transport, capabilities.EngineFfmpeg, c.Format) {
			continue
		}
		if enabled, reason := optionState(deps, s, KeyCodec, c.Name, noEntry); !enabled {
			t.Errorf("%s is greyed on an unprobed machine: %v", c.Name, reason)
		}
	}
}

// The opposite case, and why the probe records it separately: no codec on such an engine was tested
// and none can run there, the encoders no probe is spent on included.
// Without it a missing ffmpeg would read as a machine with no encoder hardware.
func TestAnEngineWithNoToolingGreysEveryCodecWithItsOwnReason(t *testing.T) {
	const missing = "ffmpeg not found on PATH"
	deps := Deps{
		Platform: platform.Info{OS: "linux", Display: "x11"},
		Encoders: encoders.Availability{
			Unprobed: map[string]string{capabilities.EngineFfmpeg: missing},
		},
	}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp")

	for _, c := range capabilities.Codecs {
		enabled, reason := optionState(deps, s, KeyCodec, c.Name, noEntry)
		if enabled {
			t.Errorf("%s is offered on an engine whose tooling is missing", c.Name)
			continue
		}
		if codeOf(reason) != engineToolingMissing {
			t.Errorf("%s greys with %v, want the engine's own statement", c.Name, codeOf(reason))
		}
		if got := idOf(reason, screensharev1.TextArgName_TEXT_ARG_NAME_ENGINE); got != capabilities.EngineFfmpeg {
			t.Errorf("%s greys naming engine %q, want the one whose tooling is missing", c.Name, got)
		}
	}
}

// Which half a failed probe names follows the family rather than the engine: a device family's
// encoder is absent because the hardware or its driver is, a software one's because nobody compiled
// it in.
func TestAFailedProbeNamesTheHalfTheFamilyIsMissing(t *testing.T) {
	deps := Deps{
		Platform: platform.Info{OS: "linux", Display: "x11"},
		Encoders: encoders.Availability{Usable: map[string]map[string]bool{
			capabilities.EngineFfmpeg: {"hevc_nvenc": false, "libaom-av1": false},
		}},
	}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp")

	_, device := optionState(deps, s, KeyCodec, "hevc_nvenc", noEntry)
	if codeOf(device) != probeNoDevice {
		t.Errorf("a missing NVENC encoder greys with %v, want the no-device verdict", codeOf(device))
	}
	if got := idOf(device, screensharev1.TextArgName_TEXT_ARG_NAME_FAMILY); got != capabilities.FamilyNvenc {
		t.Errorf("a missing NVENC encoder greys naming family %q, which is not the hardware", got)
	}

	_, build := optionState(deps, s, KeyCodec, "libaom-av1", noEntry)
	if codeOf(build) != probeNoBuild {
		t.Errorf("a missing software encoder greys with %v, want the no-build verdict", codeOf(build))
	}
	if got := idOf(build, screensharev1.TextArgName_TEXT_ARG_NAME_CODEC); got != "libaom-av1" {
		t.Errorf("a missing software encoder greys naming codec %q, which is not the encoder", got)
	}
}

// The process either holds the privilege or the capture dies at launch, and no probe tells which in
// advance, so greying the backend would refuse a choice this app cannot know is wrong.
func TestACaptureBackendBehindAPrivilegeStaysSelectable(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")

	if publish.Grant("kmsgrab") == nil {
		t.Fatal("kmsgrab declares no privilege, so this test no longer covers the case it names")
	}
	if enabled, reason := optionState(deps, s, KeyCapture, "kmsgrab", noEntry); !enabled {
		t.Errorf("kmsgrab is greyed for a privilege nothing can establish: %v", reason)
	}
}

// The sentence for an unavailable capture backend is publish's own rather than a second one written
// here: the catalog shows the same list and has to give the same answer.
func TestAnUnavailableCaptureBackendCarriesPublishsOwnSentence(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	s := availabilityDraft("portal", "libx264", "yuv420p", "rtsp")

	for _, capture := range publish.Captures() {
		available, want := publish.Available(capture, deps.Platform)
		enabled, got := optionState(deps, s, KeyCapture, capture, noEntry)
		if enabled != available {
			t.Errorf("%s is offered = %v, where publish says available = %v", capture, enabled, available)
		}
		if !proto.Equal(got, want) {
			t.Errorf("%s greys with %v, where publish says %v", capture, got, want)
		}
	}
}

// Auto answers with whichever path the pair has and the system copy is the path every pair has, so a
// combination with no row leaves a working control rather than a dead one.
func TestAutoAndTheSystemCopyAreNeverGreyed(t *testing.T) {
	for _, tc := range availabilityCases() {
		for _, memory := range []string{gpupath.MemoryAuto, gpupath.MemorySystem} {
			if enabled, reason := optionState(tc.deps, tc.s, KeyCaptureMemory, memory, noEntry); !enabled {
				t.Errorf("%s: frame memory %q is greyed: %v", tc.name, memory, reason)
			}
		}
	}
}

// A pair whose device path converts nothing greys the value demanding the colour too, and names the
// capture backend that reaches both.
// That last half is the useful one, the same screen often being reachable on the other engine where
// the conversion does state its colour.
func TestTheDirectPathThatTradesColourNamesTheWayToTheExactOne(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "windows"}}
	s := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")

	enabled, reason := optionState(deps, s, KeyCaptureMemory, gpupath.MemoryGpu, noEntry)
	if enabled {
		t.Fatal("the exact-colour device value is offered on a pair whose path converts nothing")
	}
	reach := nestedOf(reason, screensharev1.TextArgName_TEXT_ARG_NAME_REACH)
	if got := idOf(reach, screensharev1.TextArgName_TEXT_ARG_NAME_CAPTURE); got != "d3d11screencapturesrc" {
		t.Errorf("the greying points at capture %q, not at the pair that reaches the same screen at the colour selected", got)
	}
	if trade, _ := optionState(deps, s, KeyCaptureMemory, gpupath.MemoryGpuEncoderColor, noEntry); !trade {
		t.Error("the value that pays the colour for the path is greyed on the one pair that offers that trade")
	}
}

// Under that trade neither colour field reaches the stream, so both grey with the row's cost rather
// than showing a value the run discards.
func TestTheColourFieldsGreyWhereTheEncoderConvertsOnItsOwnTerms(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "windows"}}
	s := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
	s.Publish.CaptureMemory = gpupath.MemoryGpuEncoderColor

	for _, key := range []string{KeyChroma, KeyColorRange} {
		st := fieldState(deps, s, key, noEntry)
		if st.enabled {
			t.Errorf("%s is live on a path whose encoder converts the frames itself", key)
			continue
		}
		if codeOf(st.reason) != screensharev1.TextCode_TEXT_CODE_COST_ENCODER_SIGNALS_ITS_OWN_COLOUR {
			t.Errorf("%s greys with %v, which is not the row's own cost", key, codeOf(st.reason))
		}
	}
}

// The output resolution is an ordinary live field on a path with a filter that resizes, and greys
// only where the frames never reach one.
func TestTheOutputResolutionIsLiveOnAPathThatCanScale(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	st := fieldState(deps, availabilityDraft("x11grab", "libx264", "yuv420p", "srt"), KeyOutputResolution, noEntry)

	if !st.visible {
		t.Error("the output resolution is hidden, where a general concept greys")
	}
	if !st.enabled {
		t.Errorf("the output resolution is greyed with %v on a software path, which scales", st.reason)
	}
}

// What refuses a scaled picture is the frame path rather than the control: an encoder reading
// captured surfaces with no filter between has nothing that resizes.
// The scaled entries grey and the source size stays, so the reader is told which pair to change
// instead of finding a dead field.
func TestAScaledResolutionGreysWhereTheDevicePathHasNoFilter(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "windows"}}
	s := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
	s.Publish.CaptureMemory = gpupath.MemoryGpuEncoderColor

	if enabled, _ := optionState(deps, s, KeyOutputResolution, "", noEntry); !enabled {
		t.Error("the source size is greyed, where it is what every pair does")
	}

	enabled, reason := optionState(deps, s, KeyOutputResolution, "1280x720", noEntry)
	if enabled {
		t.Error("a scaled picture is offered on a device path with nothing on it that resizes")
	}
	if got := idOf(reason, screensharev1.TextArgName_TEXT_ARG_NAME_MEMORY); got != gpupath.MemorySystem {
		t.Errorf("the greying names memory %q, which is not the way across", got)
	}
}

// The same pair scales as soon as the run leaves that path.
// The greying offers the system copy as the way across, so on a run that already downloads every
// frame the sentence would name a fix already applied and refuse a scale that path's CPU filter can
// perform.
// Auto is such a run, taking the round trip on exactly the rows this greys for.
func TestAScaledResolutionIsOfferedOnceTheRunLeavesTheDevicePath(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "windows"}}

	for _, memory := range []string{gpupath.MemoryAuto, gpupath.MemorySystem} {
		s := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
		s.Publish.CaptureMemory = memory

		if enabled, reason := optionState(deps, s, KeyOutputResolution, "1280x720", noEntry); !enabled {
			t.Errorf("frame memory %q downloads every frame and the scaled picture is still greyed: %s",
				memory, reason)
		}
	}
}

// A family with no row reaches no verdict of its own and would grey under the engine's name instead.
func TestEveryEncoderFamilyStatesWhatItsEncodersTake(t *testing.T) {
	for _, family := range capabilities.Families {
		if _, ok := availabilityFamilies[family]; !ok {
			t.Errorf("encoder family %q states nothing about what its encoders take", family)
		}
	}
	for family := range availabilityFamilies {
		if !slices.Contains(capabilities.Families, family) {
			t.Errorf("a row for encoder family %q, which the capability table does not declare", family)
		}
	}
}

// A per-protocol knob gated on a name the registry does not carry is gated on a protocol nothing
// publishes, and never appears.
func TestEveryGatedTransportIsRegistered(t *testing.T) {
	for _, name := range []string{
		availabilitySrt, availabilityRtsp, availabilityWebrtc, availabilityRtmp, availabilityHls,
	} {
		if _, ok := transport.Get(name); !ok {
			t.Errorf("a field is gated on transport %q, which the registry does not carry", name)
		}
	}
}

// The capability table's engine name is the identifier a statement carries and the one a surface
// looks its spelling up by, so an engine the table does not declare would cross as a name nothing on
// the other side can spell.
func TestEveryPublishEngineIsDeclared(t *testing.T) {
	for _, engine := range publish.Engines() {
		if !slices.Contains(capabilities.Engines, engine) {
			t.Errorf("publish engine %q is not one the capability table declares", engine)
		}
	}
}

// A knob a receiver reads is on the screen, whatever the legs are set to.
//
// The defect this locks out was a hidden control still in force.
// The two link knobs followed both leg settings, but PlayerWatchTransport does not decide which
// players run: one is opened per press, on whichever leg the reader picked.
// A player opened over RTSP while both settings said SRT read RtspWatchProtocol, and the control
// holding that value was on no screen.
//
// Hidden and greyed are both answers about a knob that does nothing here.
// One that does something is shown (docs/field-availability.md).
func TestAWatchKnobAReceiverReadsIsShown(t *testing.T) {
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	s.Viewer.PlayerWatchTransport = availabilitySrt
	s.Viewer.TileWatchTransport = availabilitySrt

	// Both link knobs, against legs neither setting names: a player can be opened on either protocol
	// from this machine, and each reads its own.
	for _, key := range []string{KeySrtWatchLatencyMs, KeyRtspWatchProtocol} {
		if st := fieldState(fieldTestDeps(), s, key, noEntry); !st.visible {
			t.Errorf("%s is hidden while a player can be opened on the leg that reads it", key)
		}
	}

	// The tile's own knob is the other half of the rule and does not move with it.
	// A tile receives over the leg its setting names and over no other, so a reorder window for a
	// protocol no tile is on is a control that genuinely does nothing here.
	if st := fieldState(fieldTestDeps(), s, KeyRtspWatchLatencyMs, noEntry); st.visible {
		t.Error("the tile's RTSP reorder window is shown while the tile receives over SRT")
	}

	s.Viewer.TileWatchTransport = availabilityRtsp
	if st := fieldState(fieldTestDeps(), s, KeyRtspWatchLatencyMs, noEntry); !st.visible {
		t.Error("the tile's RTSP reorder window is hidden while the tile receives over RTSP")
	}
}

// availabilityCodecNames is what a codec dropdown offers: every row of the table, greyed where this
// combination rules it out.
func availabilityCodecNames() []string {
	out := make([]string, 0, len(capabilities.Codecs))
	for _, c := range capabilities.Codecs {
		out = append(out, c.Name)
	}
	return out
}
