package form

import (
	"reflect"
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// presetCases are the machines a preset is resolved against: the two Linux sessions, Windows,
// a machine whose settings name nothing this app carries, and one whose probe found a single
// encoder family, which is where a preset no candidate reaches is.
func presetCases() []availabilityCase {
	linuxX11 := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	linuxWayland := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	windows := Deps{Platform: platform.Info{OS: "windows"}}

	vaapiOnly := linuxWayland
	vaapiOnly.Encoders = presetOnlyFamilies(capabilities.FamilyVaapi)

	// Nothing here names anything the tables carry, which is a settings file somebody edited by hand.
	handEdited := availabilityDraft("no-such-capture", "libx264", "no-such-chroma", "srt")
	handEdited.Publish.Encoder = "no-such-encoder"

	return []availabilityCase{
		{"a VAAPI-only Wayland session", vaapiOnly,
			availabilityDraft("portal", "hevc_vaapi", "yuv420p", "srt")},
		{"an X11 session", linuxX11, availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
		{"a Wayland session", linuxWayland, availabilityDraft("portal", "hevc_vaapi", "yuv420p", "srt")},
		{"a Windows desktop", windows, availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")},
		{"a hand-edited settings file", linuxX11, handEdited},
	}
}

func presetOf(t *testing.T, key string) preset {
	t.Helper()

	for _, p := range presetTable {
		if p.key == key {
			return p
		}
	}
	t.Fatalf("no preset named %q", key)
	return preset{}
}

// presetOnlyFamilies is a probe result where the named encoder families work and every other codec
// was tested and refused, on both publish engines.
func presetOnlyFamilies(families ...string) encoders.Availability {
	usable := make(map[string]map[string]bool, len(capabilities.Engines))
	for _, engine := range capabilities.Engines {
		verdicts := make(map[string]bool, len(capabilities.Codecs))
		for _, c := range capabilities.Codecs {
			verdicts[c.Name] = slices.Contains(families, c.Family)
		}
		usable[engine] = verdicts
	}
	return encoders.Availability{Usable: usable}
}

// presetOnlyCodecs is a probe result where the named codecs work and every other one was tested and
// refused, on both publish engines.
func presetOnlyCodecs(names ...string) encoders.Availability {
	usable := make(map[string]map[string]bool, len(capabilities.Engines))
	for _, engine := range capabilities.Engines {
		verdicts := make(map[string]bool, len(capabilities.Codecs))
		for _, c := range capabilities.Codecs {
			verdicts[c.Name] = slices.Contains(names, c.Name)
		}
		usable[engine] = verdicts
	}
	return encoders.Availability{Usable: usable}
}

// A search answering with settings outside the claim would hand a surface a preset it had to stop
// marking as selected the moment it was applied.
func TestAResolvedPresetDeliversItsOwnPromise(t *testing.T) {
	for _, tc := range presetCases() {
		s, _ := Repair(tc.deps, tc.s)
		for _, p := range presetTable {
			reached, ok := presetResolve(tc.deps, p, s)
			if !ok {
				continue
			}
			if !presetHolds(reached.Publish, p.claim) {
				t.Errorf("%s: %s resolved to settings its own promise does not cover: %+v",
					tc.name, p.key, reached.Publish)
			}
		}
	}
}

// A preset is an idempotent operation rather than a step: the settings a search returns are
// themselves the candidate the next search reaches first (docs/development-principles.md).
func TestApplyingAPresetTwiceEqualsApplyingItOnce(t *testing.T) {
	for _, tc := range presetCases() {
		s, _ := Repair(tc.deps, tc.s)
		for _, p := range presetTable {
			once, ok := presetResolve(tc.deps, p, s)
			if !ok {
				continue
			}
			twice, again := presetResolve(tc.deps, p, once)
			if !again {
				t.Errorf("%s: %s stopped being reachable from the settings it had just produced", tc.name, p.key)
				continue
			}
			if !reflect.DeepEqual(twice, once) {
				t.Errorf("%s: %s applied twice moved %v", tc.name, p.key, repairChanged(once, twice))
			}
		}
	}
}

// The search answers with what the repair the form runs returned, so a preset landing on a value
// that same repair still moves is this package disagreeing with itself.
func TestAResolvedPresetNeedsNoRepair(t *testing.T) {
	for _, tc := range presetCases() {
		s, _ := Repair(tc.deps, tc.s)
		for _, p := range presetTable {
			reached, ok := presetResolve(tc.deps, p, s)
			if !ok {
				continue
			}
			if _, repaired := Repair(tc.deps, reached); len(repaired) > 0 {
				t.Errorf("%s: %s resolved to a draft the repair still moves: %v", tc.name, p.key, repaired)
			}
		}
	}
}

// A preset writes figures the controls under them can return to.
// Landing a slider between two of its stops leaves a value a sweep cannot reproduce, so the reader
// who nudges it loses the preset's own figure.
func TestAPresetLandsOnValuesItsSlidersStopOn(t *testing.T) {
	for _, tc := range presetCases() {
		s, _ := Repair(tc.deps, tc.s)
		for _, p := range presetTable {
			reached, ok := presetResolve(tc.deps, p, s)
			if !ok {
				continue
			}
			for _, off := range fieldsOffTheirStep(tc.deps, reached) {
				t.Errorf("%s: %s applied and %s", tc.name, p.key, off)
			}
		}
	}
}

// The publish transport is how viewers are reached rather than a property of the picture, and the
// sentence on an unreachable preset names it as what the search worked within.
// A preset that moved it would answer a request about the picture by changing who can watch.
func TestAPresetLeavesThePublishTransportWhereItIs(t *testing.T) {
	for _, tc := range presetCases() {
		s, _ := Repair(tc.deps, tc.s)
		for _, p := range presetTable {
			reached, ok := presetResolve(tc.deps, p, s)
			if !ok {
				continue
			}
			if reached.Publish.Transport != s.Publish.Transport {
				t.Errorf("%s: %s moved the publish leg from %s to %s",
					tc.name, p.key, s.Publish.Transport, reached.Publish.Transport)
			}
		}
	}
}

// The relay is per site and the viewer's own fields are per driver, a render chain this machine
// registers being one the machine it is copied to may not, so a preset carrying either would be the
// thing that breaks on the next machine (docs/presets.md).
func TestAPresetTouchesNothingOutsideThePublishGroup(t *testing.T) {
	for _, tc := range presetCases() {
		s, _ := Repair(tc.deps, tc.s)
		for _, p := range presetTable {
			reached, ok := presetResolve(tc.deps, p, s)
			if !ok {
				continue
			}
			if reached.Relay != s.Relay {
				t.Errorf("%s: %s moved the relay coordinates", tc.name, p.key)
			}
			if reached.Viewer != s.Viewer {
				t.Errorf("%s: %s moved this machine's own watching", tc.name, p.key)
			}
		}
	}
}

// The selection is derived from the settings and never remembered, so a preset applied is a preset
// marked: the ladder produces settings, and the claim reads them back.
func TestApplyingAPresetSelectsIt(t *testing.T) {
	for _, tc := range presetCases() {
		s, _ := Repair(tc.deps, tc.s)
		for _, p := range presetTable {
			reached, ok := presetResolve(tc.deps, p, s)
			if !ok {
				continue
			}
			for _, entry := range resolvePresets(tc.deps, reached) {
				if entry.GetKey() == p.key && !entry.GetSelected() {
					t.Errorf("%s: applying %s left it unselected", tc.name, p.key)
				}
				if entry.GetKey() != p.key && entry.GetSelected() {
					t.Errorf("%s: applying %s selected %s as well", tc.name, p.key, entry.GetKey())
				}
			}
		}
	}
}

// A field edited to a value the promise still covers is not a different way of publishing,
// however the settings got there, which is the whole of what a derived selection buys.
func TestTheSelectionFollowsTheSettingsRatherThanTheApply(t *testing.T) {
	gaming := presetOf(t, "gaming")

	inside := settings.Defaults().Publish
	inside = gaming.base(inside)
	inside.Fps = 120
	if !presetHolds(inside, gaming.claim) {
		t.Error("a frame rate above the one the promise asks for left the preset")
	}

	outside := gaming.base(settings.Defaults().Publish)
	outside.Bframes = 2
	if presetHolds(outside, gaming.claim) {
		t.Error("a reorder delay the promise rules out kept the preset")
	}
}

// A preset no candidate satisfies is stated as unreachable with the reason and nothing is applied:
// a repaired near-miss would be a configuration the user did not ask for carrying the name of one
// they did.
//
// Lossless is the case that exists, no VA profile coding bit-exact, so a machine whose only
// encoders are VAAPI reaches no candidate for it.
func TestAnUnreachablePresetCarriesTheReasonAndNoSettings(t *testing.T) {
	if _, gap := mustCodec(t, "hevc_vaapi").OptionGap(
		capabilities.EngineGst, capabilities.OptionMode, capabilities.ModeLossless); !gap {
		t.Fatal("the VA encoders code lossless, so this test no longer names an unreachable preset")
	}

	// A Wayland session reaches the portal alone, that backend runs the GStreamer engine, and the
	// probe found only the VA elements there.
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	deps.Encoders = presetOnlyFamilies(capabilities.FamilyVaapi)

	s, _ := Repair(deps, availabilityDraft("portal", "hevc_vaapi", "yuv420p", "srt"))

	for _, entry := range resolvePresets(deps, s) {
		if entry.GetKey() != "lossless" {
			continue
		}
		if entry.GetSettings() != nil {
			t.Errorf("lossless resolved on a machine whose encoders code no lossless: %+v", entry.GetSettings())
		}
		if entry.GetReason() == nil {
			t.Error("an unreachable preset carries no reason")
		}
	}
}

// A field outside a preset's claim is the machine's to answer, so the repair walking it is not a
// rejected candidate.
//
// The quantization range is that field for every preset but lossless, and the va elements signal no
// colour description, so a draft holding full range would put every VA encoder out of reach.
// What the search then reaches is a software encoder, and a 60 fps desktop lands on the CPU on a
// machine whose silicon codes the rung.
func TestAPresetTakesTheDeviceEncoderOverAFieldItPromisesNothingAbout(t *testing.T) {
	if _, gap := mustCodec(t, "hevc_vaapi").OptionGap(
		capabilities.EngineGst, capabilities.OptionColorRange, capabilities.ColorRangeFull); !gap {
		t.Fatal("the va elements carry the colour range, so this test names no field a preset leaves standing")
	}

	// A Wayland session reaches the portal alone and that backend runs the GStreamer engine, where the
	// gap is. The software encoders are what the settings can hold full range on.
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	deps.Encoders = presetOnlyFamilies(capabilities.FamilyVaapi, capabilities.FamilySoftware)

	draft := availabilityDraft("portal", "libx264", "yuv420p", "srt")
	draft.Publish.ColorRange = capabilities.ColorRangeFull
	s, moved := Repair(deps, draft)
	if s.Publish.ColorRange != capabilities.ColorRangeFull {
		t.Fatalf("the draft this test starts from is not on full range: the repair moved %v", moved)
	}

	reached, ok := presetResolve(deps, presetOf(t, "gaming"), s)
	if !ok {
		t.Fatal("gaming reached nothing on a machine whose VA encoders code its rung")
	}
	if family := mustCodec(t, reached.Publish.Codec()).Family; family != capabilities.FamilyVaapi {
		t.Errorf("gaming resolved to %s, a %s encoder, beside VA encoders that code the same rung",
			reached.Publish.Codec(), family)
	}
}

// The ladder step follows the encoder the machine has rather than the preset.
//
// A preset naming a step would carry that encoder's identifier onto every candidate, the repair
// would move it, a repaired candidate is a rejected one, and the table would resolve on the family
// the step came from and on no other.
// What each mode is worth running at is the codec row's answer, the same one a fresh installation
// gets.
func TestAPresetReachesAMachineWithNoNvidiaEncoder(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	deps.Encoders = presetOnlyFamilies(capabilities.FamilySoftware)

	s, _ := Repair(deps, availabilityDraft("x11grab", "libx264", "yuv420p", "srt"))

	for _, entry := range resolvePresets(deps, s) {
		if entry.GetKey() != "gaming" {
			continue
		}
		reached := entry.GetSettings()
		if reached == nil {
			t.Fatalf("gaming reached no software encoder: %v", entry.GetReason())
		}
		codec := wire.ToPublish(reached).Codec()
		c := mustCodec(t, codec)
		if step := reached.GetEffort(); !c.Effort.Has(step) {
			t.Errorf("gaming resolved on %s carrying the step %q, which that encoder does not take",
				codec, step)
		}
	}
}

// A preset carries a configuration this machine encodes, so what it produces is what an encoder can
// be handed.
// The pair that broke it: a preset states its own target and leaves the burst ceiling the draft
// arrived with, and the va elements express a VBR target as a percentage of that ceiling and take
// 50% at the lowest, so a ceiling far above the target has no form on them.
func TestAPresetProducesSettingsTheEncodersCanExpress(t *testing.T) {
	deps := Deps{
		Platform: platform.Info{OS: "linux", Display: "x11"},
		// The driver whose defect withholds constant bitrate from this codec, which is what sends the
		// repair looking for another rate control and lands the candidate on VBR.
		Device: capabilities.Device{Driver: "radeonsi", Model: "AMD Radeon 780M Graphics"},
	}
	// The one encoder this machine has is the one the defect names, so the search cannot step around
	// it onto a sibling that holds constant bitrate.
	deps.Encoders = presetOnlyCodecs("av1_vaapi")

	draft, _ := Repair(deps, availabilityDraft("ximagesrc", "av1_vaapi", "yuv420p", "rtsp"))
	// A ceiling many times the target, which is what a walk over the burst control leaves behind and
	// what no preset states anything about.
	draft.Publish.Mode = capabilities.ModeVbr
	draft.Publish.BitrateM, draft.Publish.MaxrateM = 40, 757

	for _, p := range presetTable {
		reached, ok := presetResolve(deps, p, draft)
		if !ok {
			continue
		}
		if err := publish.EncoderRefusal(reached); err != nil {
			t.Errorf("%s resolved to settings no encoder takes: %v", p.key, err)
		}
	}
}
