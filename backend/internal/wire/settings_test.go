package wire

import (
	"fmt"
	"reflect"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// populatedSettings is a settings.Settings with every field set,
// each to a value no other field holds.
//
// The failure this conversion has is not a field left out, which shows up as a zero,
// but a field mapped to its twin: BitrateM written into maxrate_mbps,
// the publish RTSP protocol read back out of the watch one,
// the player's watch leg landing in the tile's.
// Each of those survives a round trip unnoticed the moment two fields share a value,
// so no two here do, across the three groups and not only inside one,
// the conversion writing all three.
func populatedSettings() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:        "relay.fixture",
			SrtPort:     1001,
			RtspPort:    1003,
			WebrtcPort:  1004,
			RtmpPort:    1005,
			HlsPort:     1006,
			MoqPort:     1007,
			GroupKey:    "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
			DisplayName: "fixture-member",
			DiscordMode: true,
			DiscordLink: "fixture-discord-link",
		},
		Publish: settings.Publish{
			Transport:           "srt",
			Format:              "hevc",
			Encoder:             "nvenc",
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
			UplinkMeasuredUnix:  999,
			OutputResolution:    "1280x720",
			Preset:              "fixture-preset",
		},
		Viewer: settings.Viewer{
			TileWatchTransport: "webrtc",
			RtspWatchProtocol:  "tcp",
			SrtWatchLatencyMs:  666,
			RtspWatchLatencyMs: 777,
			RenderChain:        "d3d11",
			PreviewRoute:       settings.PreviewEndToEnd,
		},
		App: settings.App{
			SendCrashReports:    true,
			CheckUpdatesOnStart: true,
		},
	}
}

// eachField visits every leaf field of a settings value, group by group,
// naming each the way a form key does: the group, a dot, the field.
//
// The walk stops two levels deep, the shape settings.Settings has: three groups of plain values.
// A group gaining a struct field would leave this comparing two structs,
// which is still a comparison and still fails on a difference.
func eachField(s settings.Settings, visit func(name string, value reflect.Value)) {
	v := reflect.ValueOf(s)
	for i := range v.NumField() {
		// streamName is unexported: WithStreamName is its only way in, and it holds no group of its
		// own for this walk to descend into.
		if v.Type().Field(i).PkgPath != "" {
			continue
		}
		group := v.Type().Field(i).Name
		g := v.Field(i)
		for j := range g.NumField() {
			// brokered is unexported: runtime facts with WithBrokeredGroup as their only way in,
			// so the contract has nothing to carry for them.
			if g.Type().Field(j).PkgPath != "" {
				continue
			}
			name := group + "." + g.Type().Field(j).Name
			if offContract[name] {
				continue
			}
			visit(name, g.Field(j))
		}
	}
}

// offContract are the settings fields this conversion does not carry.
//
// Two are a migration's working room rather than settings:
// the old keys a file written before the audio list and before the format and encoder pair carries
// are read once, turned into what replaced them and cleared,
// so a draft crossing to a shell and back has nothing to say about either (settings/migrate.go).
//
// The last is a credential rather than a setting: the relay token is minted per command
// from the group key beside it, lives for minutes and is written by one function in the backend,
// so a shell has nothing to edit about it,
// and a contract carrying it would hand every shell a secret it has no use
// for (internal/app, settingsForCommand).
var offContract = map[string]bool{
	"Publish.FlatAudio": true,
	"Publish.FlatCodec": true,
	"Relay.Token":       true,
}

// A settings draft crosses to a shell and comes back edited on every keystroke,
// so a field that loses its value on the way is a setting the user cannot change,
// and one that reverts with nothing said.
//
// A plain round trip, the contract carrying every settings field:
// nothing has to be merged onto a held draft to survive a shell that could not see it.
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

// Settings.stream_name carries what StreamName derives, computed rather than read off a stored
// field, so there is no domain field for the round trip above to walk.
func TestTheWireCarriesTheComputedStreamName(t *testing.T) {
	s := populatedSettings()
	if got, want := Settings(s).GetStreamName(), s.StreamName(); got != want {
		t.Errorf("stream_name on the wire = %q, want %q", got, want)
	}
}

func fieldByName(s settings.Settings, name string) reflect.Value {
	var found reflect.Value
	eachField(s, func(n string, v reflect.Value) {
		if n == name {
			found = v
		}
	})
	return found
}

// A field added to settings.Settings and forgotten in the fixture leaves the round trip above
// comparing two zeroes and passing,
// the one way this package can drop a setting and be told it is fine.
func TestEveryFieldIsGivenAValueInTheFixture(t *testing.T) {
	eachField(populatedSettings(), func(name string, v reflect.Value) {
		if v.IsZero() {
			t.Errorf("settings field %s is zero in populatedSettings, so the round-trip test proves nothing about it: give it a distinct value here and map it in both directions of its group's conversion",
				name)
		}
	})
}

// The fixture catches a swapped twin only while no two fields share a value,
// and it is a hand-written table an edit can collide with nothing said.
//
// Flags stand outside it, holding one of two values and colliding as soon as there are three.
// TestEveryFlagRoundTripsOnItsOwn carries their half of the question.
func TestTheFixtureGivesNoTwoFieldsTheSameValue(t *testing.T) {
	// Keyed by the value's rendering rather than by the value,
	// a settings field being able to be a list and a list not being a map key.
	// The question is whether two fields are indistinguishable to a reader of the round trip,
	// and two that print alike are.
	seen := map[string]string{}
	eachField(populatedSettings(), func(name string, v reflect.Value) {
		if v.Kind() == reflect.Bool {
			return
		}
		value := fmt.Sprintf("%v", v.Interface())
		if first, ok := seen[value]; ok {
			t.Errorf("%s and %s both hold %v, so a conversion mapping one to the other's wire field would round trip unnoticed", first, name, value)
			return
		}
		seen[value] = name
	})
}

// Every flag, set on its own against a fixture holding none of the others.
//
// The fixture's own uniqueness cannot reach them: a flag holds one of two values,
// so two of them are indistinguishable there and a conversion writing one into the other's
// wire field round trips unnoticed.
// Set alone, that swap comes back with the wrong field standing.
func TestEveryFlagRoundTripsOnItsOwn(t *testing.T) {
	for _, name := range flagFields() {
		t.Run(name, func(t *testing.T) {
			want := withOnlyFlag(name)
			got := ToSettings(Settings(want))

			if !reflect.DeepEqual(got, want) {
				eachField(want, func(field string, wantValue reflect.Value) {
					gotValue := fieldByName(got, field)
					if !reflect.DeepEqual(gotValue.Interface(), wantValue.Interface()) {
						t.Errorf("%s = %v with %s set alone, want %v",
							field, gotValue, name, wantValue)
					}
				})
			}
		})
	}
}

// flagFields is every bool leaf of the settings, named the way eachField names one.
func flagFields() []string {
	var names []string
	eachField(populatedSettings(), func(name string, v reflect.Value) {
		if v.Kind() == reflect.Bool {
			names = append(names, name)
		}
	})
	return names
}

// withOnlyFlag is the fixture with on standing and every other flag down.
func withOnlyFlag(on string) settings.Settings {
	s := populatedSettings()

	v := reflect.ValueOf(&s).Elem()
	for i := range v.NumField() {
		if v.Type().Field(i).PkgPath != "" {
			continue
		}
		group := v.Type().Field(i).Name
		g := v.Field(i)
		for j := range g.NumField() {
			if g.Type().Field(j).PkgPath != "" || g.Field(j).Kind() != reflect.Bool {
				continue
			}
			g.Field(j).SetBool(group+"."+g.Type().Field(j).Name == on)
		}
	}
	return s
}

// A request arriving with no settings set was written by another process,
// an Umgebungsfehler the app survives and the caller rejects.
// A panic here would let a malformed message kill the process holding the encoder.
//
// The groups go through their own accessors for the same reason,
// so a message carrying one group and not another converts rather than panicking on the absent one.
func TestANilMessageConvertsToTheZeroSettings(t *testing.T) {
	if got := ToSettings(nil); !reflect.DeepEqual(got, settings.Settings{}) {
		t.Errorf("ToSettings(nil) = %+v, want the zero settings.Settings", got)
	}
}

// The empty output resolution is the capture's own size reaching the encoder unscaled,
// a value and not an absence: settings.Publish.OutputSize answers false for it rather than a zero
// size, and a conversion turning it into anything else, a literal "0x0" or the monitor's dimensions
// filled in, would put a scaling stage in front of every stream that never asked for one.
//
// The round trip above covers a resolution that is set.
// This pins the meaning of the empty one, the case an edit meaning well breaks.
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

// A preset travels whole rather than as a diff against the defaults,
// so applying one is an assignment:
// the name selects it, and the settings under it are the entire state the user saved.
//
// It saves the publish group and nothing else.
// Where the relay is belongs to a deployment and how this machine watches belongs to a viewer,
// so a preset carrying either would move a setting the user never meant to save.
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

// An empty list and a missing one are the same thing on the wire,
// and a caller ranging over the result does not have to know which it holds.
func TestNoSavedPresetsConvertToAnEmptyList(t *testing.T) {
	got := Presets(nil)
	if got == nil {
		t.Fatal("Presets(nil) returned a nil slice, want an empty one")
	}
	if len(got) != 0 {
		t.Errorf("Presets(nil) has %d entries, want none", len(got))
	}
}

// The store's order is the order the user saved in and the order a shell lists,
// so it is part of what crosses.
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
