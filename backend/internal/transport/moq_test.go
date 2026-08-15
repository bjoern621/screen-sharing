package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

func moqTestStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:    "10.0.0.5",
			MoqPort: 8892,
		},
		Publish: settings.Publish{
			Name: "alice",
		},
	}
}

func TestMoQRegistered(t *testing.T) {
	tr, ok := Get("moq")
	if !ok {
		t.Fatal("moq transport not registered")
	}
	if tr.Name() != "moq" {
		t.Fatalf("Name() = %q, want moq", tr.Name())
	}
}

// The trailing slash is the reader page, and the address without it is where the relay redirects
// from.
func TestMoQBrowserURL(t *testing.T) {
	want := "https://10.0.0.5:8892/bob/"
	got := MoQ{}.BrowserURL(moqTestStream(), "bob")
	if got != want {
		t.Errorf("BrowserURL = %q, want %q", got, want)
	}
}

// The scheme and the port hold under Tls, where every other HTTP leg drops both for the proxy's
// name on 443.
// WebTransport refuses a plaintext listener and no proxy carries it, so this is the one leg the
// relay terminates itself in every deployment.
func TestMoQKeepsItsOwnListenerBehindAProxy(t *testing.T) {
	s := moqTestStream()
	s.Relay.Host = "relay.example.com"

	if !s.Relay.Tls() {
		t.Fatal("a relay named across the internet is the encrypted case this test is about")
	}
	if want, got := "https://relay.example.com:8892/bob/", (MoQ{}).BrowserURL(s, "bob"); got != want {
		t.Errorf("BrowserURL = %q, want %q", got, want)
	}
}

// A browser sets no header on an address it is handed, and the relay's HTTP servers read no query,
// so the token rides as the Basic password the browser turns into one.
func TestMoQCarriesTheCredentialAsUserinfo(t *testing.T) {
	s := moqTestStream()
	s.Relay.Token = "a.b.c"

	if want, got := "https://jwt:a.b.c@10.0.0.5:8892/bob/", (MoQ{}).BrowserURL(s, "bob"); got != want {
		t.Errorf("BrowserURL = %q, want %q", got, want)
	}
}

// The one reader that reaches it. libavformat has no MoQ demuxer and GStreamer no MoQ source, so a
// carriage for either would offer a leg nothing here can open.
func TestMoQIsWatchedInABrowserAlone(t *testing.T) {
	if !CanWatch("moq", EngineBrowser) {
		t.Error("moq must state a browser watch carriage")
	}
	for _, engine := range []string{capabilities.EngineFfmpeg, capabilities.EngineGst} {
		if CanWatch("moq", engine) {
			t.Errorf("moq must state no %s watch carriage", engine)
		}
		if CanPublish("moq", engine) {
			t.Errorf("moq must state no %s publish form", engine)
		}
	}
	if _, ok := WatchURL("moq", moqTestStream(), "bob"); ok {
		t.Error("WatchURL must report false for moq, which no player opens")
	}
	if _, ok := GstSource("moq", moqTestStream(), "bob"); ok {
		t.Error("GstSource must report false for moq, which no source element subscribes to")
	}
}

// The reason to offer it beside the other two browser legs: it is the only one carrying every
// bitstream this app encodes.
func TestMoQCarriesEveryFormatTheOtherBrowserLegsDrop(t *testing.T) {
	moq, ok := WatchCarriage("moq", EngineBrowser)
	if !ok {
		t.Fatal("moq states no browser carriage")
	}
	for _, format := range capabilities.Formats() {
		if !slices.Contains(moq.Video, format) {
			t.Errorf("moq drops %q, which the relay packages into a track", format)
		}
	}
}
