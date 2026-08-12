package wire

import (
	"fmt"
	"reflect"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// populatedSettings is a settings.Settings with every field set, and every field set to
// a value no other field holds.
//
// Distinctness is the whole point of the fixture rather than a tidiness. The failure
// this conversion actually has is not a field left out - that shows up as a zero - but
// a field mapped to its twin: BitrateM written into maxrate_mbps, the publish RTSP
// protocol read back out of the watch one, the player's watch leg landing in the tile's.
// Every one of those survives a round trip unnoticed the moment two fields share a
// value, so no two here do - across the three groups and not only inside one, because
// the conversion writes all three.
func populatedSettings() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:       "relay.fixture",
			SrtPort:    1001,
			ApiPort:    1002,
			RtspPort:   1003,
			WebrtcPort: 1004,
			RtmpPort:   1005,
			HlsPort:    1006,
		},
		Publish: settings.Publish{
			Name:                "fixture-stream",
			Transport:           "srt",
			Codec:               "hevc_nvenc",
			Mode:                "vbr",
			Chroma:              "yuv444p",
			ColorRange:          "tv",
			Fps:                 144,
			Cq:                  21,
			BitrateM:            111,
			MaxrateM:            222,
			VbvMs:               333,
			Gop:                 444,
			Bframes:             3,
			Effort:              "p5",
			Tune:                "ll",
			Capture:             "ddagrab",
			AudioSources:        settings.Recording("desktop"),
			AudioCodec:          "aac",
			DrmMap:              "vaapi",
			Monitor:             2,
			CaptureMemory:       "gpu",
			Cursor:              "hidden",
			SrtPublishLatencyMs: 555,
			RtspPublishProtocol: "udp",
			UplinkMbps:          888,
			OutputResolution:    "1280x720",
		},
		Viewer: settings.Viewer{
			PlayerWatchTransport: "rtsp",
			TileWatchTransport:   "webrtc",
			RtspWatchProtocol:    "tcp",
			SrtWatchLatencyMs:    666,
			RtspWatchLatencyMs:   777,
			RenderChain:          "d3d11",
		},
	}
}

// eachField visits every leaf field of a settings value, group by group, naming each as
// the key a form would: the group, a dot, the field.
//
// The walk is two levels deep and stops there, because that is the shape settings.Settings
// has: three groups of plain values. A group gaining a struct field would leave this
// comparing two structs, which is still a comparison and still fails on a difference.
func eachField(s settings.Settings, visit func(name string, value reflect.Value)) {
	v := reflect.ValueOf(s)
	for i := range v.NumField() {
		group := v.Type().Field(i).Name
		g := v.Field(i)
		for j := range g.NumField() {
			name := group + "." + g.Type().Field(j).Name
			if offContract[name] {
				continue
			}
			visit(name, g.Field(j))
		}
	}
}

// offContract are the settings fields this conversion deliberately does not carry.
//
// One is, and it is a migration's own working room rather than a setting: the old key a
// file written before the audio list carried is read once, turned into the list and
// cleared, so a draft crossing to a shell and back has nothing to say about it and a
// fixture that gave it a value would be asserting that the contract carries a field it has
// no reason to (settings/migrate.go).
var offContract = map[string]bool{"Publish.LegacyAudio": true}

// A settings draft crosses to a shell and comes back edited on every keystroke, so a
// field that loses its value on the way is a setting the user cannot change and a
// setting that silently reverts.
//
// It is a plain round trip now. The contract carries every settings field, so nothing
// has to be merged onto a held draft to survive a shell that could not see it: the
// two knobs that used to need that were the obsolete GTK4 grid's, and both are
// ViewerSettings fields since the split.
func TestARoundTripKeepsEveryField(t *testing.T) {
	want := populatedSettings()
	got := ToSettings(Settings(want))

	if !reflect.DeepEqual(got, want) {
		eachField(want, func(name string, wantValue reflect.Value) {
			gotValue := fieldByName(got, name)
			if !reflect.DeepEqual(gotValue.Interface(), wantValue.Interface()) {
				t.Errorf("%s = %v after a round trip, want %v", name, gotValue, wantValue)
			}
		})
	}
}

// fieldByName reads one leaf out of a settings value by the name eachField gives it.
func fieldByName(s settings.Settings, name string) reflect.Value {
	var found reflect.Value
	eachField(s, func(n string, v reflect.Value) {
		if n == name {
			found = v
		}
	})
	return found
}

// A field added to settings.Settings and forgotten in the fixture would leave the round
// trip above comparing two zeroes and passing, which is the one way this package can
// drop a setting and be told it is fine.
//
// The instruction on failure: give the named field a value here that no other field in
// populatedSettings holds, and map it in both directions of the group it belongs to.
func TestEveryFieldIsGivenAValueInTheFixture(t *testing.T) {
	eachField(populatedSettings(), func(name string, v reflect.Value) {
		if v.IsZero() {
			t.Errorf("settings field %s is zero in populatedSettings, so the round-trip test proves nothing about it: give it a distinct value here and map it in both directions of its group's conversion",
				name)
		}
	})
}

// The fixture only catches a swapped twin while no two fields share a value, and it is
// a hand-written table that a later edit can quietly collide.
func TestTheFixtureGivesNoTwoFieldsTheSameValue(t *testing.T) {
	// Keyed by the value's rendering rather than by the value, because a settings field may
	// be a list and a list is not a map key. What the check is about is whether two fields
	// are indistinguishable to a reader of the round trip, and two that print alike are.
	seen := map[string]string{}
	eachField(populatedSettings(), func(name string, v reflect.Value) {
		value := fmt.Sprintf("%v", v.Interface())
		if first, ok := seen[value]; ok {
			t.Errorf("%s and %s both hold %v, so a conversion mapping one to the other's wire field would round trip unnoticed", first, name, value)
			return
		}
		seen[value] = name
	})
}

// A request that arrives with no settings set was written by another process, which
// makes it an environment condition the app survives and the caller's to reject. This
// package turning it into a panic would make a malformed message able to kill the
// process holding the encoder.
//
// The groups are read through their own accessors for the same reason, so a message
// that carried one group and not another converts rather than panicking on the absent
// one.
func TestANilMessageConvertsToTheZeroSettings(t *testing.T) {
	if got := ToSettings(nil); !reflect.DeepEqual(got, settings.Settings{}) {
		t.Errorf("ToSettings(nil) = %+v, want the zero settings.Settings", got)
	}
}

// The empty output resolution is the capture's own size reaching the encoder unscaled,
// which is a value and not an absence: settings.Publish.OutputSize answers false for it
// rather than a zero size, and a conversion that turned it into anything else - a
// literal "0x0", the monitor's dimensions filled in helpfully - would put a scaling
// stage in front of every stream that never asked for one.
//
// The round-trip test above already covers a resolution that is set. This one pins the
// meaning of the empty one, because that is the case a well-meaning edit breaks.
func TestAnUnscaledCaptureCrossesAsTheEmptyResolution(t *testing.T) {
	unscaled := populatedSettings()
	unscaled.Publish.OutputResolution = ""

	if got := Settings(unscaled).GetPublish().GetOutputResolution(); got != "" {
		t.Errorf("output_resolution = %q, want empty for a capture that is not scaled", got)
	}
	if got := ToSettings(Settings(unscaled)).Publish.OutputResolution; got != "" {
		t.Errorf("OutputResolution = %q, want empty for a capture that is not scaled", got)
	}
}

// A preset travels whole rather than as a diff against the defaults, so applying one is
// an assignment: the name selects it and the settings under it are the entire state the
// user saved.
//
// What it saves is the publish group and nothing else. Where the relay is belongs to a
// deployment and how this machine watches belongs to a viewer, so a preset carrying
// either would move a setting the user never meant to save.
func TestAPresetCarriesItsNameAndAllOfItsSettings(t *testing.T) {
	p := settings.Preset{Name: "work", Settings: populatedSettings().Publish}

	got := Preset(p)
	if got.GetName() != p.Name {
		t.Errorf("preset name = %q, want %q", got.GetName(), p.Name)
	}
	if back := ToPublish(got.GetSettings()); !reflect.DeepEqual(back, p.Settings) {
		t.Errorf("preset settings = %+v, want %+v", back, p.Settings)
	}
}

// An empty list and a missing one are the same thing on the wire, and a caller ranging
// over the result should not have to know which it got.
func TestNoSavedPresetsConvertToAnEmptyList(t *testing.T) {
	got := Presets(nil)
	if got == nil {
		t.Fatal("Presets(nil) returned a nil slice, want an empty one")
	}
	if len(got) != 0 {
		t.Errorf("Presets(nil) has %d entries, want none", len(got))
	}
}

// The store's order is the order the user saved in and the order a shell lists, so it
// is part of what crosses.
func TestPresetsCrossInStoreOrder(t *testing.T) {
	ps := []settings.Preset{
		{Name: "first", Settings: populatedSettings().Publish},
		{Name: "second", Settings: settings.Defaults().Publish},
		{Name: "third", Settings: populatedSettings().Publish},
	}

	got := Presets(ps)
	if len(got) != len(ps) {
		t.Fatalf("Presets returned %d entries, want %d", len(got), len(ps))
	}
	for i, p := range ps {
		if got[i].GetName() != p.Name {
			t.Errorf("entry %d is %q, want %q", i, got[i].GetName(), p.Name)
		}
	}
}
