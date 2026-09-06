package form

import (
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// availabilityAllKeys is every field the form declares, spelled out a second time:
// the test reading it checks that the availability table and keys.go cover each other,
// which a list derived from either could not.
var availabilityAllKeys = []string{
	KeyRelayHost, KeyRelayTls, KeyDiscordMode, KeyDiscordRichPresence, KeyGroupKey, KeyDisplayName,
	KeySrtPort, KeyRtspPort, KeyWebrtcPort,
	KeyRtmpPort, KeyHlsPort, KeyMoqPort,
	KeyTransport, KeyFormat, KeyEncoder, KeyMode, KeyChroma, KeyColorRange, KeyFps, KeyCq,
	KeyBitrateM, KeyMaxrateM, KeyVbvMs, KeyGop, KeyBframes, KeyEffort, KeyTune,
	KeyCapture, KeyAudioSource, KeyAudioSourceDevice, KeyAudioSourceGain, KeyAudioSourceMute,
	KeyAudioCodec, KeyDrmMap, KeyMonitor, KeyCaptureMemory,
	KeyCursor,
	KeySrtPublishLatencyMs, KeySrtWatchLatencyMs,
	KeyRtspPublishProtocol, KeyRtspWatchProtocol,
	KeyUplinkMbps, KeyOutputResolution,
	KeyTileWatchTransport, KeyRtspWatchLatencyMs, KeyRenderChain,
	KeySendCrashReports, KeyCheckUpdatesOnStart,
}

// The value spaces the option tests walk, stated here rather than read off an option builder:
// the walk has to reach every value the settings can hold, including one no builder offers,
// since a stored settings file still reaches the greying rule for it.
var (
	availabilityChromas     = []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"}
	availabilityColorRanges = []string{"pc", "tv"}
)

// availabilityDraft is the defaults with the machine's own answers replaced by fixed ones,
// so a case reads the same on every platform the suite runs on.
// settings.Defaults picks its capture backend off runtime.GOOS,
// which would otherwise make half of these tests describe the developer.
func availabilityDraft(capture, codec, chroma, transportName string) settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = capture
	s.Publish.UseCodec(codec)
	s.Publish.Chroma = chroma
	s.Publish.Transport = transportName
	s.Publish.Mode = capabilities.ModeCrf
	s.Publish.ColorRange = "tv"
	s.Publish.CaptureMemory = gpupath.MemoryAuto
	// The two ladder steps follow the codec this draft names, as the defaults, the migration
	// and the repair all set them.
	// Carrying the default codec's step onto another encoder would make every draft one the repair
	// has to move, which is not the combination a case is about.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(codec, s.Publish.Mode)
	return s
}

// availabilityCase is one machine and one draft, named for what it reaches.
type availabilityCase struct {
	name string
	deps Deps
	s    settings.Settings
}

// availabilityCases spread wide enough that every greying path in the table is taken
// by one of them: both engines, both colour verdicts of the pair table,
// a machine whose probe never ran, one whose engine has no tooling,
// and a settings file naming a capture backend this app has no publisher for.
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

	noPortalMetadata := linuxWayland
	noPortalMetadata.Portal = portal.Capabilities{CursorModes: portal.CursorHidden | portal.CursorEmbedded}

	// A pair no row carries, as a hand-edited file holds: the format is one the table produces
	// and the encoder answers to nothing, so the draft names an encode that does not exist.
	handEditedEncode := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	handEditedEncode.Publish.Encoder = "no-such-encoder"

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
		{"a compositor whose portal reports no pointer position", noPortalMetadata,
			availabilityDraft("portal", "libx264", "yuv420p", "rtsp")},
		{"a capture backend from a hand-edited settings file", linuxX11,
			availabilityDraft("no-such-capture", "libx264", "yuv420p", "srt")},
		{"an encode from a hand-edited settings file", linuxX11, handEditedEncode},
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

// The same contract on the option half: a greyed entry names the limit and which side has it,
// rather than saying only that the option is gone.
func TestAGreyedOptionAlwaysCarriesAReason(t *testing.T) {
	values := map[string][]string{
		KeyCapture:    publish.Captures(),
		KeyTransport:  transport.Names(),
		KeyFormat:     optionCodedFormats(),
		KeyEncoder:    capabilities.Encoders(),
		KeyChroma:     availabilityChromas,
		KeyMode:       capabilities.Modes,
		KeyColorRange: availabilityColorRanges,
		// Every declared source, which is the list on every platform:
		// the table marks the ones a session here does not serve, so the Info this is asked with changes
		// the verdicts and not the roster.
		KeyAudioSource:   platform.AudioSourceIDs(platform.Info{}),
		KeyAudioCodec:    capabilities.AudioNames(),
		KeyCaptureMemory: gpupath.Memories,
		KeyCursor:        cursor.Modes,
		// A watch leg is offered from the roster its receiver can reach,
		// what the option list is built from and what the reasons are stated against.
		KeyTileWatchTransport: transport.WatchNames(capabilities.EngineGst),
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

// The portal's pointer modes belong to the compositor behind it, so the option follows what this
// machine answered rather than what the capture backend's row can express.
func TestThePointerModeFollowsWhatThePortalServes(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	s := availabilityDraft("portal", "libx264", "yuv420p", "rtsp")

	if enabled, _ := optionState(deps, s, KeyCursor, cursor.Metadata, noEntry); !enabled {
		t.Fatal("a machine nothing asked greys the mode")
	}

	deps.Portal = portal.Capabilities{CursorModes: portal.CursorHidden | portal.CursorEmbedded}
	enabled, reason := optionState(deps, s, KeyCursor, cursor.Metadata, noEntry)
	if enabled {
		t.Fatal("a portal serving hidden and embedded alone still offers the metadata mode")
	}
	if got := reason.GetCode(); got != screensharev1.TextCode_TEXT_CODE_PORTAL_SERVES_NO_CURSOR_MODE {
		t.Errorf("the refusal states %v, want the portal's own list", got)
	}
	if enabled, _ := optionState(deps, s, KeyCursor, cursor.Embedded, noEntry); !enabled {
		t.Error("a mode the portal serves is greyed along with the one it does not")
	}

	// The X11 backends read the position off the display server, so nothing the portal answered
	// reaches them.
	x11 := deps
	if enabled, _ := optionState(x11, availabilityDraft("ximagesrc", "libx264", "yuv420p", "rtsp"),
		KeyCursor, cursor.Metadata, noEntry); !enabled {
		t.Error("the X11 backend is greyed by what the desktop portal serves")
	}
}

// A control the key list declares and the table forgets would render as a plain enabled widget
// no rule governs and no repair moves.
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
// A backend implementation knob with no meaning outside one selection is not rendered at all:
// the DRM download strategy belongs to the kmsgrab scanout path,
// and its tooltip would teach a user on any other backend nothing (docs/field-availability.md).
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

// The same field greys rather than hiding a second time where the run downloads nothing:
// it is already gated on the capture backend, and a second gate would make it appear
// and vanish while the user changes codecs.
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

// The third treatment on a value the machine rather than the settings rules out:
// a source no session here serves stays in the dropdown and greys with what the machine is missing.
//
// The second track is a general concept rather than one platform's implementation knob,
// so a user on Windows reads why the machine cannot hand them what it is playing instead of finding
// a one-entry control (docs/field-availability.md).
// The sentence is the platform table's rather than written here,
// so the form never greys a source the catalog offered.
func TestAnUnservedAudioSourceIsOfferedAndGreyedWithThePlatformsReason(t *testing.T) {
	for _, info := range []platform.Info{{OS: "darwin"}} {
		deps := Deps{Platform: info}
		s := availabilityDraft("avfoundation", "libx264", "yuv420p", "srt")

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
	linux := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	onLinux := availabilityDraft("portal", "libx264", "yuv420p", "rtsp")
	if enabled, reason := optionState(linux, onLinux, KeyAudioSource, platform.AudioSourceDesktop, 0); !enabled {
		t.Errorf("a Linux session is refused desktop audio: %v", reason)
	}
}

// A source one engine opens and the other does not greys on the capture backends that run
// the other, and the reason names the engine rather than the machine.
//
// Windows is the case: wasapi2src reads what the machine plays in loopback
// and ffmpeg has no WASAPI input, so the same Windows session serves the source
// or not depending on which backend is selected.
// A platform reason there would send a user looking for a sound server they already have.
func TestAWindowsSourceFollowsTheEngineTheBackendRuns(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "windows"}}

	onFfmpeg := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")
	enabled, reason := optionState(deps, onFfmpeg, KeyAudioSource, platform.AudioSourceDesktop, 0)
	if enabled {
		t.Error("desktop audio is live under a capture backend whose engine has no WASAPI input")
	}
	if codeOf(reason) != screensharev1.TextCode_TEXT_CODE_AUDIO_SOURCE_UNSERVED_BY_ENGINE {
		t.Errorf("desktop audio greys with %v, want the statement naming the engine", codeOf(reason))
	}

	onGst := availabilityDraft("d3d11screencapturesrc", "hevc_nvenc", "yuv420p", "srt")
	if enabled, reason := optionState(deps, onGst, KeyAudioSource, platform.AudioSourceDesktop, 0); !enabled {
		t.Errorf("desktop audio greys on the engine that opens it: %v", codeOf(reason))
	}
}

// The second treatment.
// A general encoding concept the combination blocks stays rendered and greys,
// so a user hunting for the effort ladder under a Vulkan
// encoder reads why it is absent instead of finding a blank.
//
// The reason names the codec rather than a set of families, the ladder being the codec's own:
// two codecs of one family can declare different ones,
// and one declaring none says nothing about the other.
func TestTheEffortStepIsDisabledWhereTheCodecDeclaresNoLadder(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "h264_vulkan", "yuv420p", "srt")
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
	if codec := idOf(st.reason, screensharev1.TextArgName_TEXT_ARG_NAME_CODEC); codec != "h264_vulkan" {
		t.Errorf("the effort step greys naming %q, where the draft is on h264_vulkan", codec)
	}
}

// A ladder one engine spends and the other does not greys on the engine spending nothing,
// and with the departure's own reason rather than the codec's.
//
// The VAAPI rows are the case: the va elements take the seven target usages,
// where ffmpeg's VAAPI encoders count over the range the installed driver reports,
// so one step would mean a different amount of work per engine and per card.
// It is the same control on both, so this is a departure and not a gap.
func TestTheEffortStepIsDisabledWhereTheEngineSpendsNone(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	onFfmpeg := availabilityDraft("x11grab", "h264_vaapi", "yuv420p", "srt")
	st := fieldState(deps, onFfmpeg, KeyEffort, noEntry)
	if st.enabled {
		t.Error("the effort step is live on the engine whose builder spends none of it")
	}
	if codeOf(st.reason) != screensharev1.TextCode_TEXT_CODE_FFMPEG_VAAPI_QUALITY_IS_THE_DRIVERS_SCALE {
		t.Errorf("the effort step greys with %v, want the departure naming the driver's scale",
			codeOf(st.reason))
	}

	onGst := availabilityDraft("ximagesrc", "h264_vaapi", "yuv420p", "srt")
	if st := fieldState(deps, onGst, KeyEffort, noEntry); !st.enabled {
		t.Errorf("the effort step greys on the engine that spends it: %v", codeOf(st.reason))
	}
}

// The other half of the same rule: a control greyed for a codec whose encoder takes the step
// would be the form withholding a knob that reaches the encoder,
// which docs/field-availability.md rules out.
func TestTheEffortStepIsLiveWhereTheCodecDeclaresALadder(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	s.Publish.Mode = capabilities.ModeVbr

	if st := fieldState(deps, s, KeyEffort, noEntry); !st.enabled {
		t.Errorf("x264 declares an effort ladder and its builder spends it, yet the control greys: %v",
			codeOf(st.reason))
	}
}

// A codec can declare either ladder without the other: libvpx takes an effort step
// and tunes for nothing, so its effort control is live
// while its tune control greys naming the codec.
func TestTheTwoLaddersAreAskedAboutSeparately(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libvpx-vp9", "yuv420p", "srt")

	c, ok := capabilities.Get(s.Publish.Codec())
	if !ok {
		t.Fatalf("no capability row for %s", s.Publish.Codec())
	}
	if len(c.Effort.Steps) == 0 || len(c.Tune.Steps) > 0 {
		t.Skipf("%s declares no effort ladder, or a tune ladder too", s.Publish.Codec())
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

// Both engines forward both steps, so neither control greys for the engine alone:
// the nvcodec elements take the same p1-p7 ladder ffmpeg spends,
// so no engine rule withholds a step.
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
// The greying and the encode read one row, so the sentence cannot name a step the encoder
// is not running (ffmpeg.TestNvencCbrPinsTheDeclaredStep).
func TestTheEffortStepGreysWhereTheModePinsIt(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "hevc_nvenc", "yuv420p", "srt")
	s.Publish.Mode = capabilities.ModeCbr

	c, ok := capabilities.Get(s.Publish.Codec())
	if !ok {
		t.Fatalf("no capability row for %s", s.Publish.Codec())
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
// B-frames and the family has no property for them, so the reason is the family's
// and not the mode sentence, which would be a lie there.
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
// A field that stays editable and means something its label
// does not describe gains a sentence and no greying.
// The pixel format's note is about somebody else's machine: every format has a software decoder,
// so a format no GPU takes is a viewer spending cores.
func TestThePixelFormatStaysLiveAndSaysWhatItCostsAViewer(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	hardware := fieldState(deps, availabilityDraft("x11grab", "libx264", "yuv420p", "srt"), KeyChroma, noEntry)
	if !hardware.enabled || hardware.reason != nil {
		t.Fatalf("4:2:0 H.264 greys the pixel format: %+v", hardware)
	}
	if codeOf(hardware.note) != decodesInHardware {
		t.Errorf("4:2:0 H.264 carries %v, which does not say viewers decode it on a GPU", codeOf(hardware.note))
	}

	// 4:4:4 H.264 is the case the decode table exists to state: no vendor put High 4:4:4 Predictive
	// in silicon, so every viewer decodes it on the CPU, and the control still does not grey.
	software := fieldState(deps, availabilityDraft("x11grab", "libx264", "yuv444p", "srt"), KeyChroma, noEntry)
	if !software.enabled {
		t.Fatalf("4:4:4 H.264 greys the pixel format: %+v", software)
	}
	if codeOf(software.note) != decodesOnCPU {
		t.Errorf("4:4:4 H.264 carries %v, which does not say what it costs a viewer", codeOf(software.note))
	}
}

// The other half of the note's purpose: a value the builder does send is never greyed,
// which would leave the encoder using a number the form refused to show.
func TestAForwardedKnobCarriesANoteRatherThanAGreying(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("kmsgrab", "hevc_vaapi", "yuv420p", "rtsp")
	s.Publish.Mode = capabilities.ModeAbr

	st := fieldState(deps, s, KeyBitrateM, noEntry)
	if !st.enabled {
		t.Fatalf("the bitrate greys in average bitrate on VAAPI, where the builder forwards it: %+v", st)
	}
	if st.note == nil {
		t.Error("the bitrate carries no note on VAAPI, where the ceiling the elements code against derives from it")
	}
}

// The fourth treatment.
// A dropdown keeps the value a neighbouring combination allows and greys that entry alone:
// planar RGB greys where no encoder element takes it and stays selectable on the capture backends
// that run ffmpeg, which codes it.
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
// The zero Availability is the state a form resolves in before Detect answers,
// so it leaves the codec list as the tables describe it: greying there would offer a roster
// that shrinks and comes back while the app starts.
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
		if enabled, reason := availabilityRowState(deps, s, c); !enabled {
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
		enabled, reason := availabilityRowState(deps, s, c)
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

// Which half a failed probe names follows the family rather than the engine:
// a device family's encoder is absent because the hardware or its driver is,
// a software one's because nobody compiled it in.
func TestAFailedProbeNamesTheHalfTheFamilyIsMissing(t *testing.T) {
	deps := Deps{
		Platform: platform.Info{OS: "linux", Display: "x11"},
		Encoders: encoders.Availability{Usable: map[string]map[string]bool{
			capabilities.EngineFfmpeg: {"hevc_nvenc": false, "libaom-av1": false},
		}},
	}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp")

	s.Publish.Format = "hevc"
	_, device := optionState(deps, s, KeyEncoder, capabilities.FamilyNvenc, noEntry)
	if codeOf(device) != probeNoDevice {
		t.Errorf("a missing NVENC encoder greys with %v, want the no-device verdict", codeOf(device))
	}
	if got := idOf(device, screensharev1.TextArgName_TEXT_ARG_NAME_FAMILY); got != capabilities.FamilyNvenc {
		t.Errorf("a missing NVENC encoder greys naming family %q, which is not the hardware", got)
	}

	s.Publish.Format = "av1"
	_, build := optionState(deps, s, KeyEncoder, "libaom", noEntry)
	if codeOf(build) != probeNoBuild {
		t.Errorf("a missing software encoder greys with %v, want the no-build verdict", codeOf(build))
	}
	if got := idOf(build, screensharev1.TextArgName_TEXT_ARG_NAME_CODEC); got != "libaom-av1" {
		t.Errorf("a missing software encoder greys naming codec %q, which is not the encoder", got)
	}
}

// The process either holds the privilege or the capture dies at launch,
// and no probe tells which in advance,
// so greying the backend would refuse a choice this app cannot know is wrong.
func TestACaptureBackendBehindAPrivilegeStaysSelectable(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")

	if publish.Grant("kmsgrab") == nil {
		t.Fatal("kmsgrab declares no privilege, so this test does not cover the case it names")
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

// Auto answers with whichever path the pair has and the system copy is the path every pair has,
// so a combination with no row leaves a working control rather than a dead one.
func TestAutoAndTheSystemCopyAreNeverGreyed(t *testing.T) {
	for _, tc := range availabilityCases() {
		for _, memory := range []string{gpupath.MemoryAuto, gpupath.MemorySystem} {
			if enabled, reason := optionState(tc.deps, tc.s, KeyCaptureMemory, memory, noEntry); !enabled {
				t.Errorf("%s: frame memory %q is greyed: %v", tc.name, memory, reason)
			}
		}
	}
}

// A pair whose device path converts nothing greys the value demanding the colour too,
// and names the capture backend that reaches both.
// That last half is the useful one, the same screen often being reachable on the other engine
// where the conversion does state its colour.
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

// The output resolution is an ordinary live field on a path with a filter that resizes,
// and greys only where the frames never reach one.
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

// What refuses a scaled picture is the frame path rather than the control:
// an encoder reading captured surfaces with no filter between has nothing that resizes.
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
// The greying offers the system copy as the way across, so on a run that already downloads
// every frame the sentence would name a fix already applied
// and refuse a scale that path's CPU filter can perform.
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

// A family with no row reaches no verdict
// of its own and would grey under the engine's name instead.
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

// The capability table's engine name is the identifier a statement carries
// and the one a surface looks its spelling up by, so an engine the table does not declare
// would cross as a name nothing on the other side can spell.
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
// The two link knobs followed a stored player leg as well as the tile's,
// but no setting decides which players run: one is opened per press,
// on whichever leg the reader picked.
// A player opened over RTSP while both settings said SRT read RtspWatchProtocol,
// and the control holding that value was on no screen.
//
// Hidden and greyed are both answers about a knob that does nothing here.
// One that does something is shown (docs/field-availability.md).
func TestAWatchKnobAReceiverReadsIsShown(t *testing.T) {
	s := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	s.Viewer.TileWatchTransport = availabilitySrt

	// Both link knobs, against a leg the tile's setting does not name:
	// a player can be opened on either protocol from this machine, and each reads its own.
	for _, key := range []string{KeySrtWatchLatencyMs, KeyRtspWatchProtocol} {
		if st := fieldState(fieldTestDeps(), s, key, noEntry); !st.visible {
			t.Errorf("%s is hidden while a player can be opened on the leg that reads it", key)
		}
	}

	// The tile's own knob is the other half of the rule and does not move with it.
	// A tile receives over the leg its setting names and over no other,
	// so a reorder window for a protocol no tile is on is a control that genuinely does nothing here.
	if st := fieldState(fieldTestDeps(), s, KeyRtspWatchLatencyMs, noEntry); st.visible {
		t.Error("the tile's RTSP reorder window is shown while the tile receives over SRT")
	}

	s.Viewer.TileWatchTransport = availabilityRtsp
	if st := fieldState(fieldTestDeps(), s, KeyRtspWatchLatencyMs, noEntry); !st.visible {
		t.Error("the tile's RTSP reorder window is hidden while the tile receives over RTSP")
	}
}

// RTSPS wraps the control connection alone, so RTP over UDP travels beside it in the clear
// whichever end negotiated the session.
// Both legs refuse it once asked (transport.RTSP, ValidatePublishSettings and SetWatchOption),
// so a control offering the value on either leg offers a refusal.
//
// The defect this locks out was the watch field carrying no option rule:
// on one encrypted relay the publish dropdown greyed "udp"
// and the watch dropdown beside it offered it.
func TestAnEncryptedRelayGreysUdpOnBothRtspLegs(t *testing.T) {
	legs := []string{KeyRtspPublishProtocol, KeyRtspWatchProtocol}

	s := availabilityDraft("x11grab", "libx264", "yuv420p", availabilityRtsp)
	s.Relay.Host = "relay.example"
	if !s.Relay.Tls() {
		t.Fatal("the relay this case names is reached over TLS")
	}

	for _, key := range legs {
		if enabled, _ := optionState(fieldTestDeps(), s, key, "udp", noEntry); enabled {
			t.Errorf("%s offers udp on an encrypted relay, which puts the picture on the wire in the clear", key)
		}
		if enabled, reason := optionState(
			fieldTestDeps(), s, key, transport.EncryptedRtspProtocol, noEntry); !enabled {
			t.Errorf("%s greys %s, the one lower transport an encrypted session carries media on: %s",
				key, transport.EncryptedRtspProtocol, reason)
		}
	}

	// A stranded value is walked off rather than left decoding in the clear,
	// the greying being what the repair reads.
	s.Viewer.RtspWatchProtocol, s.Publish.RtspPublishProtocol = "udp", "udp"
	repaired, moved := Repair(fieldTestDeps(), s)
	if repaired.Viewer.RtspWatchProtocol != transport.EncryptedRtspProtocol {
		t.Errorf("the watch leg survived the repair on %q", repaired.Viewer.RtspWatchProtocol)
	}
	if repaired.Publish.RtspPublishProtocol != transport.EncryptedRtspProtocol {
		t.Errorf("the publish leg survived the repair on %q", repaired.Publish.RtspPublishProtocol)
	}
	for _, key := range legs {
		if !slices.Contains(moved, key) {
			t.Errorf("the repaired list is %v, which does not name %s", moved, key)
		}
	}

	// A relay this network reaches directly serves RTSPS too, so it narrows both legs the same way.
	s.Relay.Host = "192.168.1.9"
	for _, key := range legs {
		if enabled, _ := optionState(fieldTestDeps(), s, key, "udp", noEntry); enabled {
			t.Errorf("%s offers udp on a relay reached directly, which serves RTSPS alone", key)
		}
	}
}

// Nothing a draft holds greys the name this machine goes by.
// A name is claimed per group and reaches no capture backend, encoder or leg,
// so no combination on this screen rules it out and a stream in force blocks
// it no more than any other field does (docs/field-availability.md,
// "A live stream blocks no field").
//
// An empty one is a state and not a refusal.
// A machine without a name states no presence in the group its key names (app.membershipFor),
// and greying the control here would leave the reader holding that with nowhere to answer it.
func TestTheDisplayNameIsEditableWhateverTheDraftHolds(t *testing.T) {
	for _, tc := range availabilityCases() {
		for _, name := range []string{"", "Björn"} {
			s := tc.s
			s.Relay.DisplayName = name

			st := fieldState(tc.deps, s, KeyDisplayName, noEntry)
			if !st.visible || !st.enabled {
				t.Errorf("%s: a machine named %q draws the display name visible=%v enabled=%v",
					tc.name, name, st.visible, st.enabled)
			}
			if st.reason != nil {
				t.Errorf("%s: a machine named %q carries a reason on the display name: %v", tc.name, name, st.reason)
			}
			if st.note != nil {
				t.Errorf("%s: a machine named %q carries a note on the display name: %v", tc.name, name, st.note)
			}
		}
	}
}

// The toggle asks for a check the channel already refuses, whatever the machine underneath it,
// a preference over a check that never runs being a control with nothing behind it.
func TestCheckingOnStartGreysWhereTheUpdateChannelIsOff(t *testing.T) {
	for _, tc := range availabilityCases() {
		live := tc.deps
		live.UpdateCheckOff = false
		if st := fieldState(live, tc.s, KeyCheckUpdatesOnStart, noEntry); !st.visible || !st.enabled {
			t.Errorf("%s: a channel taking checks draws the toggle visible=%v enabled=%v", tc.name, st.visible, st.enabled)
		}

		off := tc.deps
		off.UpdateCheckOff = true
		st := fieldState(off, tc.s, KeyCheckUpdatesOnStart, noEntry)
		if !st.visible || st.enabled {
			t.Errorf("%s: a channel refusing every check draws the toggle visible=%v enabled=%v", tc.name, st.visible, st.enabled)
		}
		if got := st.reason.GetCode(); got != screensharev1.TextCode_TEXT_CODE_UPDATE_CHECK_OFF {
			t.Errorf("%s: the toggle's reason is not the channel's own code: %v", tc.name, got)
		}
	}
}

// availabilityRowState is what the encoder control says about one row of the capability table.
//
// The draft is pointed at that row's format first, the pair being what a greying is about:
// asking about an encoder under another format answers whether the two go together,
// which is a different question from whether this machine runs the row.
func availabilityRowState(deps Deps, s settings.Settings, c capabilities.Codec) (bool, *screensharev1.Text) {
	s.Publish.Format = c.Format
	return optionState(deps, s, KeyEncoder, c.Encoder(), noEntry)
}

// A quality target spends what the picture costs, so the ceiling is the one control that bounds it,
// and it is offered exactly where the encoder holds the target inside a VBV.
// An encoder with no form of one says so rather than taking a figure it would ignore.
func TestTheCeilingIsOfferedInConstantQualityWhereTheEncoderBoundsIt(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	bounded := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	bounded.Publish.Mode = capabilities.ModeCrf
	if st := fieldState(d, bounded, KeyMaxrateM, noEntry); !st.enabled {
		t.Errorf("libx264 crf: the ceiling is greyed with %v, want it offered", st.reason)
	}

	free := availabilityDraft("x11grab", "librav1e", "yuv420p", "srt")
	free.Publish.Mode = capabilities.ModeCrf
	if st := fieldState(d, free, KeyMaxrateM, noEntry); st.enabled {
		t.Error("librav1e crf: the ceiling is offered, want it greyed on an encoder that bounds nothing")
	}
}

// The window sizes a ceiling, so it follows the ceiling it would hold:
// an unbounded quality target has none, and a control that took a number there would size nothing.
func TestTheRateBufferFollowsTheCeilingItHolds(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	draft := availabilityDraft("x11grab", "libx264", "yuv420p", "srt")
	draft.Publish.Mode = capabilities.ModeCrf

	draft.Publish.MaxrateM = 0
	if st := fieldState(d, draft, KeyVbvMs, noEntry); st.enabled {
		t.Error("crf with no ceiling: the window is offered, want it greyed")
	}

	draft.Publish.MaxrateM = 20
	if st := fieldState(d, draft, KeyVbvMs, noEntry); !st.enabled {
		t.Errorf("crf under a ceiling: the window is greyed with %v, want it offered", st.reason)
	}
}

// A recommendation is a hint about this combination, the same combination the greying answers for,
// so the two cannot both hold of one entry.
// The mark is a builder's, which states it against the codec or the platform rather
// than against the draft: the pointer modes recommend the embedded one,
// and the scanout capture backend cannot draw a pointer into the picture at all.
func TestARuledOutEntryCarriesNoRecommendation(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}

	for _, capture := range []string{"x11grab", "kmsgrab", "ximagesrc"} {
		draft := availabilityDraft(capture, "libx264", "yuv420p", "rtsp")
		for _, group := range Resolve(d, draft).GetGroups() {
			for _, field := range group.GetFields() {
				for _, option := range field.GetOptions() {
					if option.GetRecommended() && !option.GetEnabled() {
						t.Errorf("on %s, %s recommends %q and rules it out",
							capture, field.GetKey(), option.GetValue())
					}
				}
			}
		}
	}
}

// The encoder probe answers for encoders, and a leg is the sink after them.
// An install carrying an older WHIP element than the one this app builds passes every codec probe
// there is, and the leg then takes a start, dies at launch and spends its retry budget
// on an element that was never there.
func TestALegGreysWhereItsSinkElementIsMissing(t *testing.T) {
	d := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	d.Encoders = encoders.Availability{Legs: map[string]bool{"webrtc": false, "rtsp": true}}

	// The GStreamer engine is the one whose sink is a named element, which ximagesrc selects.
	av := availabilityOf(d, availabilityDraft("ximagesrc", "libx264", "yuv420p", "rtsp"))

	reason := av.transportReason("webrtc")
	if reason == nil {
		t.Fatal("a leg whose sink element is missing is offered as one this machine publishes over")
	}
	if reason.GetCode() != screensharev1.TextCode_TEXT_CODE_PUBLISH_SINK_ELEMENT_MISSING {
		t.Errorf("the greying reads %v, want it to name the missing element", reason.GetCode())
	}
	if av.transportReason("rtsp") != nil {
		t.Errorf("a leg whose elements all register greys: %v", av.transportReason("rtsp"))
	}

	// The ffmpeg engine builds its own muxers, so a GStreamer element says nothing about it.
	ff := availabilityOf(d, availabilityDraft("x11grab", "libx264", "yuv420p", "rtsp"))
	if ff.transportReason("webrtc") != nil {
		t.Errorf("the ffmpeg engine greys a leg over a GStreamer element: %v", ff.transportReason("webrtc"))
	}
}

// The tile's leg covers every stream the window watches, anyone's as well as this machine's,
// so what this machine publishes rules out none of it.
// A publisher on HEVC still watches a neighbour's H.264 over WHEP,
// the leg with the shortest way back and no mapping for HEVC.
//
// A stream a leg cannot carry is refused when the tile opens,
// per stream and against the format the relay reports for that path (internal/app, carriesStream),
// which is the authority a draft cannot stand in for.
//
// What stays greyed is a protocol this engine has no receiver for at all.
func TestTheTileLegIgnoresWhatThisMachinePublishes(t *testing.T) {
	deps := fieldTestDeps()
	s := availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", availabilitySrt)

	for _, name := range transport.WatchNames(capabilities.EngineGst) {
		enabled, reason := optionState(deps, s, KeyTileWatchTransport, name, noEntry)
		if !enabled {
			t.Errorf("%s is greyed on a machine publishing HEVC: %v", name, codeOf(reason))
		}
	}
}
