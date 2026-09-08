package form

import (
	"slices"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// strandedDeps is a Linux machine whose GStreamer tooling is missing,
// which is what greys every format, every encoder and every publish leg at once.
func strandedDeps() Deps {
	return Deps{
		Platform: platform.Info{OS: "linux", Display: "wayland"},
		Encoders: encoders.Availability{
			Unprobed: map[string]string{capabilities.EngineGst: "gst-inspect-1.0 is not on this machine"},
			// Each publish leg was asked for and none of its elements registered,
			// an unprobed engine leaving the legs unanswered rather than refused.
			Legs: map[string]bool{"srt": false, "rtsp": false, "webrtc": false, "rtmp": false, "moq": false},
		},
	}
}

// strandedDraft publishes over the portal, in a group, so nothing else refuses the start.
func strandedDraft() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "portal"
	s.Publish.Preset = ""
	s.Relay.Host = "relay.example"
	s.Relay.GroupKey = "8OEzNpqrEonSiyp6O+4bPM1Rq9NunjH6CaThcBigPpQ="
	s.Relay.DisplayName = "probe"
	return s
}

func strandedFieldKeys(f *screensharev1.Form, severity screensharev1.Severity) []string {
	var out []string
	for _, d := range f.GetDiagnostics() {
		if d.GetText().GetCode() == screensharev1.TextCode_TEXT_CODE_NOTHING_LEFT_TO_PICK &&
			d.GetSeverity() == severity {
			out = append(out, d.GetFieldKey())
		}
	}
	return out
}

// A control whose every entry is refused holds a value the same evaluation greys,
// so the draft names a pipeline nothing here can run and the start is refused rather than offered.
func TestAControlWithEveryEntryRefusedRefusesTheStart(t *testing.T) {
	f := Resolve(strandedDeps(), strandedDraft())

	for _, key := range []string{KeyFormat, KeyEncoder, KeyTransport} {
		var field *screensharev1.Field
		for _, g := range f.GetGroups() {
			for _, row := range g.GetFields() {
				if row.GetKey() == key {
					field = row
				}
			}
		}
		if field == nil {
			t.Fatalf("the form draws no %s", key)
		}
		for _, o := range field.GetOptions() {
			if o.GetEnabled() {
				t.Fatalf("%s offers %s, so this machine is not the one under test", key, o.GetValue())
			}
		}
	}

	keys := strandedFieldKeys(f, screensharev1.Severity_SEVERITY_ERROR)
	for _, want := range []string{KeyFormat, KeyEncoder, KeyTransport} {
		found := false
		for _, key := range keys {
			if key == want {
				found = true
			}
		}
		if !found {
			t.Errorf("no statement anchors on %s, and its every entry is refused: %v", want, keys)
		}
	}

	if f.GetPublishable() {
		t.Error("the form publishes a draft whose codec, encoder and leg are each refused")
	}
}

// A machine that runs what it offers says none of this.
func TestAControlWithAnEntryLeftStatesNothing(t *testing.T) {
	d := strandedDeps()
	d.Encoders = encoders.Availability{}
	f := Resolve(d, strandedDraft())

	if keys := strandedFieldKeys(f, screensharev1.Severity_SEVERITY_ERROR); len(keys) > 0 {
		t.Errorf("a machine with entries left is told it has none: %v", keys)
	}
}

// Every control that offers entries can be left with nothing to pick,
// and the statement about it anchors on the control,
// so a key missing from the anchors would fail the assert in diagnostics rather than render.
func TestEveryControlWithEntriesIsAnAnchor(t *testing.T) {
	for i := range fieldTable {
		f := &fieldTable[i]
		if f.options == nil && f.itemOptions == nil {
			continue
		}
		if !slices.Contains(warningAnchors, f.key) {
			t.Errorf("%s offers entries and anchors nowhere", f.key)
		}
	}
}

// A gap costs the stream where a start reads the group, and one tile where the viewer does.
func TestAViewerGapWarnsWhereAPublishGapRefuses(t *testing.T) {
	if got := strandedSeverity(GroupTransport); got != screensharev1.Severity_SEVERITY_ERROR {
		t.Errorf("a leg with nothing left to pick ranks %v, and a start is handed the leg", got)
	}
	if got := strandedSeverity(GroupWatch); got != screensharev1.Severity_SEVERITY_WARNING {
		t.Errorf("a watch leg with nothing left to pick ranks %v, and no stream waits on it", got)
	}
}
