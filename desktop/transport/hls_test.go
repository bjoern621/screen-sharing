package transport

import (
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

func hlsTestStream() settings.Stream {
	return settings.Stream{Name: "alice", RelayHost: "relay.example", HlsPort: 8888}
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

// HLS is the watch-only leg. Every publish helper has to refuse it, since the
// relay serves the segments and ingests none of them.
func TestHLSPublishesNothing(t *testing.T) {
	s := hlsTestStream()
	s.Transport = "hls"

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
