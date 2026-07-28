package transport

import (
	"testing"

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

	if CanFFmpegPublish("hls") || CanGstPublish("hls") {
		t.Error("hls must declare no publish form")
	}
	if _, ok := PublishArgs(s); ok {
		t.Error("PublishArgs must report false for hls")
	}
	if _, ok := GstSink(s); ok {
		t.Error("GstSink must report false for hls")
	}
	if err := ValidatePublish("hls", "libx264"); err == nil {
		t.Error("publishing over hls must be refused whatever the codec")
	}
}
