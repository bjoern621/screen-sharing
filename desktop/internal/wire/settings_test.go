package wire

import (
	"reflect"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// populatedStream is a settings.Stream with every field set, and every field set to a
// value no other field holds.
//
// Distinctness is the whole point of the fixture rather than a tidiness. The failure
// this conversion actually has is not a field left out - that shows up as a zero - but
// a field mapped to its twin: BitrateM written into maxrate_mbps, the publish RTSP
// protocol read back out of the watch one, the watch transport landing in the grid
// leg. Every one of those survives a round trip unnoticed the moment two fields share
// a value, so no two here do.
func populatedStream() settings.Stream {
	return settings.Stream{
		Name:       "fixture-stream",
		RelayHost:  "relay.fixture",
		RelayPort:  1001,
		ApiPort:    1002,
		RtspPort:   1003,
		WebrtcPort: 1004,
		RtmpPort:   1005,
		HlsPort:    1006,
		MoqPort:    1007,

		Transport:  "srt",
		Codec:      "hevc_nvenc",
		Mode:       "vbr",
		Chroma:     "yuv444p",
		ColorRange: "tv",
		Fps:        144,
		Cq:         21,
		BitrateM:   111,
		MaxrateM:   222,
		VbvMs:      333,
		Gop:        444,
		Bframes:    3,
		EncPreset:  "p5",

		Capture:       "ddagrab",
		Audio:         "desktop",
		AudioCodec:    "aac",
		DrmMap:        "vaapi",
		Monitor:       2,
		CaptureMemory: "gpu",

		SrtPublishLatencyMs: 555,
		SrtWatchLatencyMs:   666,

		RtspPublishProtocol: "udp",
		RtspWatchProtocol:   "tcp",
		RtspWatchLatencyMs:  777,

		UplinkMbps: 888,

		WatchTransport:   "rtsp",
		GridTransport:    "webrtc",
		OutputResolution: "1280x720",
	}
}

// offContract is every settings field the contract does not carry.
//
// Both are leftovers of the obsolete GTK4 grid window - the watch leg its tiles received
// over, and the jitter buffer its receiving pipeline sized itself by - and nothing on the
// wire describes that window. They are named here so the round trip below can say which
// half of the seam it is testing, and so a field added to this list without a reason
// fails to look like an accident.
var offContract = map[string]bool{
	"GridTransport":      true,
	"RtspWatchLatencyMs": true,
}

// A settings draft crosses to a shell and comes back edited on every keystroke, so a
// field that loses its value on the way is a setting the user cannot change and a
// setting that silently reverts.
//
// The round trip under test is StreamSettingsOnto and not StreamSettings, because that
// is the one every inbound settings path actually takes. A shell sends the whole message
// rather than a diff, so a field the contract dropped would be cleared by the next save
// made from a screen that never showed it, and no shell can be asked to preserve a value
// it has no way to see.
func TestARoundTripKeepsEveryField(t *testing.T) {
	want := populatedStream()
	got := StreamSettingsOnto(want, Settings(want))

	gotV, wantV := reflect.ValueOf(got), reflect.ValueOf(want)
	for i := range wantV.NumField() {
		name := wantV.Type().Field(i).Name
		if !reflect.DeepEqual(gotV.Field(i).Interface(), wantV.Field(i).Interface()) {
			t.Errorf("%s = %v after a round trip, want %v", name, gotV.Field(i), wantV.Field(i))
		}
	}
}

// The other half of that seam, stated so the preservation above cannot quietly become
// the whole story: the contract really does drop those fields, and exactly those. A
// field that started crossing again would make StreamSettingsOnto restore a value the
// message already carried, which is harmless until the two disagree.
func TestTheContractCarriesEveryFieldButTheOffContractOnes(t *testing.T) {
	want := populatedStream()
	got := StreamSettings(Settings(want))

	gotV, wantV := reflect.ValueOf(got), reflect.ValueOf(want)
	for i := range wantV.NumField() {
		name := wantV.Type().Field(i).Name
		crossed := reflect.DeepEqual(gotV.Field(i).Interface(), wantV.Field(i).Interface())
		if offContract[name] && crossed {
			t.Errorf("%s crossed the contract, which no longer declares it", name)
		}
		if !offContract[name] && !crossed {
			t.Errorf("%s = %v after crossing, want %v", name, gotV.Field(i), wantV.Field(i))
		}
	}
}

// A field added to settings.Stream and forgotten in the fixture would leave the round
// trip above comparing two zeroes and passing, which is the one way this package can
// drop a setting and be told it is fine.
//
// The instruction on failure: give the named field a value here that no other field in
// populatedStream holds, and map it in both Settings and StreamSettings.
func TestEveryStreamFieldIsGivenAValueInTheFixture(t *testing.T) {
	v := reflect.ValueOf(populatedStream())
	for i := range v.NumField() {
		if v.Field(i).IsZero() {
			t.Errorf("settings.Stream field %s is zero in populatedStream, so the round-trip test proves nothing about it: give it a distinct value here and map it in both directions of wire.Settings and wire.StreamSettings",
				v.Type().Field(i).Name)
		}
	}
}

// The fixture only catches a swapped twin while no two fields share a value, and it is
// a hand-written table that a later edit can quietly collide.
func TestTheFixtureGivesNoTwoFieldsTheSameValue(t *testing.T) {
	v := reflect.ValueOf(populatedStream())
	seen := make(map[any]string, v.NumField())
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		value := v.Field(i).Interface()
		if first, ok := seen[value]; ok {
			t.Errorf("%s and %s both hold %v, so a conversion mapping one to the other's wire field would round trip unnoticed", first, name, value)
			continue
		}
		seen[value] = name
	}
}

// A request that arrives with no settings set was written by another process, which
// makes it an environment condition the app survives and the caller's to reject. This
// package turning it into a panic would make a malformed message able to kill the
// process holding the encoder.
func TestANilMessageConvertsToTheZeroStream(t *testing.T) {
	if got := StreamSettings(nil); !reflect.DeepEqual(got, settings.Stream{}) {
		t.Errorf("StreamSettings(nil) = %+v, want the zero settings.Stream", got)
	}
}

// The empty output resolution is the capture's own size reaching the encoder unscaled,
// which is a value and not an absence: settings.Stream.OutputSize answers false for it
// rather than a zero size, and a conversion that turned it into anything else - a
// literal "0x0", the monitor's dimensions filled in helpfully - would put a scaling
// stage in front of every stream that never asked for one.
//
// The round-trip test above already covers a resolution that is set. This one pins the
// meaning of the empty one, because that is the case a well-meaning edit breaks.
func TestAnUnscaledCaptureCrossesAsTheEmptyResolution(t *testing.T) {
	unscaled := populatedStream()
	unscaled.OutputResolution = ""

	if got := Settings(unscaled).GetOutputResolution(); got != "" {
		t.Errorf("output_resolution = %q, want empty for a capture that is not scaled", got)
	}
	if got := StreamSettings(Settings(unscaled)).OutputResolution; got != "" {
		t.Errorf("OutputResolution = %q, want empty for a capture that is not scaled", got)
	}
}

// A preset travels whole rather than as a diff against the defaults, so applying one is
// an assignment: the name selects it and the settings under it are the entire state the
// user saved.
func TestAPresetCarriesItsNameAndAllOfItsSettings(t *testing.T) {
	p := settings.Preset{Name: "work", Settings: populatedStream()}

	got := Preset(p)
	if got.GetName() != p.Name {
		t.Errorf("preset name = %q, want %q", got.GetName(), p.Name)
	}
	// Read back onto the settings it was saved from, for the reason the round trip above
	// states: a preset is a StreamSettings and carries exactly what the contract does, so
	// the fields the contract has dropped are not in it either. A preset that stored one
	// would be storing a value no shell can set and no form can show.
	if back := StreamSettingsOnto(p.Settings, got.GetSettings()); !reflect.DeepEqual(back, p.Settings) {
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
		{Name: "first", Settings: populatedStream()},
		{Name: "second", Settings: settings.Defaults()},
		{Name: "third", Settings: populatedStream()},
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
