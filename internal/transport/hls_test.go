package transport

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

func hlsTestStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:    "relay.example",
			HlsPort: 8888,
		},
		Publish: settings.Publish{
			Name: "alice",
		},
	}
}

func TestHLSRegistered(t *testing.T) {
	tr, ok := Get("hls")
	if !ok {
		t.Fatal("hls transport not registered")
	}
	if tr.Name() != "hls" {
		t.Fatalf("Name() = %q, want hls", tr.Name())
	}
}

func TestHLSWatchURL(t *testing.T) {
	want := "http://relay.example:8888/bob/index.m3u8"
	got := HLS{}.WatchURL(hlsTestStream(), "bob")
	if got != want {
		t.Errorf("WatchURL = %q, want %q", got, want)
	}
}

// The page fetches the playlist itself, so its address carries no playlist name.
// A player URL handed to a browser downloads a file instead of playing it.
func TestHLSBrowserURL(t *testing.T) {
	want := "http://relay.example:8888/bob/"
	got := HLS{}.BrowserURL(hlsTestStream(), "bob")
	if got != want {
		t.Errorf("BrowserURL = %q, want %q", got, want)
	}
}

// The relay serves the segments and ingests none, so every publish helper refuses the leg.
func TestHLSPublishesNothing(t *testing.T) {
	s := hlsTestStream()
	s.Publish.Transport = "hls"

	for _, engine := range capabilities.Engines {
		if CanPublish("hls", engine) {
			t.Errorf("hls must declare no publish form on the %s engine", engine)
		}
		if err := ValidatePublish("hls", engine, "libx264"); err == nil {
			t.Errorf("publishing over hls on the %s engine must be refused whatever the codec", engine)
		}
	}
	if _, ok := PublishArgs(s); ok {
		t.Error("PublishArgs must report false for hls")
	}
	if _, ok := GstSink(s); ok {
		t.Error("GstSink must report false for hls")
	}
}
