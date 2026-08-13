package form

import (
	"reflect"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// presetCases are the machines a preset is resolved against: the two Linux sessions, Windows,
// a machine whose settings name nothing this app carries, and one whose probe found a single
// encoder family - which is where a preset that no candidate reaches is.
func presetCases() []availabilityCase {
	linuxX11 := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	linuxWayland := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	windows := Deps{Platform: platform.Info{OS: "windows"}}

	vaapiOnly := linuxWayland
	vaapiOnly.Encoders = presetOnlyFamily(capabilities.FamilyVaapi)

	return []availabilityCase{
		{"a VAAPI-only Wayland session", vaapiOnly,
			availabilityDraft("portal", "hevc_vaapi", "yuv420p", "srt")},
		{"an X11 session", linuxX11, availabilityDraft("x11grab", "libx264", "yuv420p", "srt")},
		{"a Wayland session", linuxWayland, availabilityDraft("portal", "hevc_vaapi", "yuv420p", "srt")},
		{"a Windows desktop", windows, availabilityDraft("ddagrab", "hevc_nvenc", "yuv420p", "srt")},
		{"a hand-edited settings file", linuxX11,
			availabilityDraft("no-such-capture", "no-such-codec", "no-such-chroma", "srt")},
	}
}

// presetOf reads one row of the table, so a test names the preset it is about rather than an index
// into it.
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

// presetOnlyFamily is a probe result on which one encoder family works and every other codec was
// tested and refused, on both publish engines.
func presetOnlyFamily(family string) encoders.Availability {
	usable := make(map[string]map[string]bool, len(capabilities.Engines))
	for _, engine := range capabilities.Engines {
		verdicts := make(map[string]bool, len(capabilities.Codecs))
		for _, c := range capabilities.Codecs {
			verdicts[c.Name] = c.Family == family
		}
		usable[engine] = verdicts
	}
	return encoders.Availability{Usable: usable}
}

// A preset states a goal, and the settings that reach it are this machine's answer.
// So the one thing every reachable preset owes is that its own promise holds of what it returns:
// a search that answered with settings outside the claim would hand a surface a preset it had to
// stop marking as selected the moment it was applied.
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

// Applying twice equals applying once, which is what makes a preset an idempotent operation rather
// than a step: the settings a search returns are themselves the candidate the next search reaches
// first (docs/development-principles.md).
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

// A preset is only ever offered as a configuration the rest of the form already accepts.
// The search puts every candidate through the same repair the form runs and keeps it only if it
// comes back untouched, so a preset that landed on a value the form would grey would be this
// package disagreeing with itself.
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

// The publish transport is not searched: it is how viewers are reached rather than a property of
// the picture, and the sentence on an unreachable preset names it as the thing the search worked
// within.
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

// A preset is a way of publishing and nothing else.
// The relay is per site and the viewer's own fields are per driver - a render chain this machine
// registers is one the machine it is copied to may not - so a preset that carried either would be
// the thing that breaks on the next machine (docs/presets.md).
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
// marked.
// The two halves of a preset meet here: the ladder produces settings, and the claim reads them
// back.
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

// Settings inside a claim keep the preset selected however they got there,
// and settings outside it drop out of it.
// That is the whole of what a derived selection buys: a field edited to a value the promise still
// covers is not a different way of publishing.
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

// A preset no candidate satisfies is stated as unreachable with the reason, and nothing is applied:
// a repaired near-miss would be a configuration the user did not ask for carrying the name of one
// they did.
//
// The lossless preset is the case that exists.
// No VA profile codes bit-exact, so a machine whose only encoders are VAAPI reaches no candidate
// for it.
func TestAnUnreachablePresetCarriesTheReasonAndNoSettings(t *testing.T) {
	if _, gap := mustCodec(t, "hevc_vaapi").OptionGap(
		capabilities.EngineGst, capabilities.OptionMode, capabilities.ModeLossless); !gap {
		t.Fatal("the VA encoders code lossless, so this test no longer names an unreachable preset")
	}

	// A Wayland session reaches the portal alone, which runs the GStreamer engine,
	// and the probe found nothing but the VA elements there.
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "wayland"}}
	deps.Encoders = presetOnlyFamily(capabilities.FamilyVaapi)

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

// A preset reaches whichever encoder the machine has, and the ladder step follows that encoder
// rather than the preset.
//
// The step is the encoder's own identifier, so a preset that named one would carry it onto every
// candidate: the repair moves a step off the candidate codec's ladder, a repaired candidate is a
// rejected one, and the whole table would quietly resolve on the family whose ladder the step came
// from and on no other.
// What each mode is worth running at is the codec row's, which is the same answer a fresh
// installation gets.
func TestAPresetReachesAMachineWithNoNvidiaEncoder(t *testing.T) {
	deps := Deps{Platform: platform.Info{OS: "linux", Display: "x11"}}
	deps.Encoders = presetOnlyFamily(capabilities.FamilySoftware)

	s, _ := Repair(deps, availabilityDraft("x11grab", "libx264", "yuv420p", "srt"))

	for _, entry := range resolvePresets(deps, s) {
		if entry.GetKey() != "gaming" {
			continue
		}
		reached := entry.GetSettings()
		if reached == nil {
			t.Fatalf("gaming reached no software encoder: %v", entry.GetReason())
		}
		codec := reached.GetCodec()
		c := mustCodec(t, codec)
		if step := reached.GetEffort(); !c.Effort.Has(step) {
			t.Errorf("gaming resolved on %s carrying the step %q, which that encoder does not take",
				codec, step)
		}
	}
}
