package transport

import (
	"reflect"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

func watchSettings() settings.Settings {
	return settings.Settings{
		Viewer: settings.Viewer{
			SrtWatchLatencyMs:  1200,
			RtspWatchLatencyMs: 200,
			RtspWatchProtocol:  "tcp",
		},
	}
}

func TestWatchOptionsCarryTheSettingsValues(t *testing.T) {
	got := WatchOptions("rtsp", watchSettings())
	if len(got) != 2 {
		t.Fatalf("got %d rtsp options, want the jitter buffer and the protocol: %+v", len(got), got)
	}
	if got[0].Key != "rtspWatchLatencyMs" || got[0].Value != "200" || got[0].Kind != OptionInt {
		t.Errorf("option 0 = %+v, want the jitter buffer at 200", got[0])
	}
	if got[1].Value != "tcp" || got[1].Kind != OptionChoice {
		t.Errorf("option 1 = %+v, want the protocol at tcp", got[1])
	}
	if len(got[1].Choices) == 0 {
		t.Error("a choice option offers no choices")
	}
}

// A transport with no watch form of its own declares no knobs, and neither does a name the registry
// does not know.
func TestWatchOptionsOfATransportWithout(t *testing.T) {
	if got := WatchOptions("webrtc", watchSettings()); len(got) != 0 {
		t.Errorf("webrtc options = %+v, want none", got)
	}
	if got := WatchOptions("carrier-pigeon", watchSettings()); len(got) != 0 {
		t.Errorf("unknown transport options = %+v, want none", got)
	}
}

func TestSetWatchOption(t *testing.T) {
	s := watchSettings()
	if err := SetWatchOption("srt", &s, "srtWatchLatencyMs", "800"); err != nil {
		t.Fatal(err)
	}
	if s.Viewer.SrtWatchLatencyMs != 800 {
		t.Errorf("latency = %d, want 800", s.Viewer.SrtWatchLatencyMs)
	}
	if err := SetWatchOption("rtsp", &s, "rtspWatchProtocol", "udp"); err != nil {
		t.Fatal(err)
	}
	if s.Viewer.RtspWatchProtocol != "udp" {
		t.Errorf("protocol = %q, want udp", s.Viewer.RtspWatchProtocol)
	}
}

// A refused value leaves the settings as they were: the caller is told, rather than the knob
// quietly taking something else.
func TestSetWatchOptionRefuses(t *testing.T) {
	cases := []struct {
		name      string
		transport string
		key       string
		value     string
	}{
		{"a key of another transport", "srt", "rtspWatchProtocol", "udp"},
		{"a value that is not a number", "srt", "srtWatchLatencyMs", "soon"},
		{"a value below the minimum", "srt", "srtWatchLatencyMs", "0"},
		{"a choice outside the set", "rtsp", "rtspWatchProtocol", "sctp"},
		{"a transport with no options", "webrtc", "srtWatchLatencyMs", "800"},
		{"a transport that is not registered", "carrier-pigeon", "srtWatchLatencyMs", "800"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := watchSettings()
			err := SetWatchOption(c.transport, &s, c.key, c.value)
			if err == nil {
				t.Fatalf("%s=%s was accepted on %s", c.key, c.value, c.transport)
			}
			if !reflect.DeepEqual(s, watchSettings()) {
				t.Errorf("a refused option changed the settings: %+v", s)
			}
			if !strings.Contains(err.Error(), c.transport) && !strings.Contains(err.Error(), c.key) {
				t.Errorf("error = %q, want it to name the transport or the key", err)
			}
		})
	}
}

// Every option a transport declares can be written back, which is what the viewer does with the
// values it read.
func TestEveryDeclaredOptionRoundTrips(t *testing.T) {
	for _, name := range Names() {
		s := watchSettings()
		for _, o := range WatchOptions(name, s) {
			if err := SetWatchOption(name, &s, o.Key, o.Value); err != nil {
				t.Errorf("%s: reading %s back: %v", name, o.Key, err)
			}
		}
		if !reflect.DeepEqual(s, watchSettings()) {
			t.Errorf("%s: writing its own values back changed the settings: %+v", name, s)
		}
	}
}
