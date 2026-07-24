package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/settings"
)

func TestWebRTCRegistered(t *testing.T) {
	tr, ok := Get("webrtc")
	if !ok {
		t.Fatal("webrtc transport not registered")
	}
	if tr.Name() != "webrtc" {
		t.Fatalf("Name() = %q, want webrtc", tr.Name())
	}
}

func TestWebRTCPublishArgs(t *testing.T) {
	s := settings.Stream{Name: "alice", RelayHost: "relay.example", WebrtcPort: 8889}
	args := WebRTC{}.PublishArgs(s)

	want := []string{"-f", "whip", "http://relay.example:8889/alice/whip"}
	if !slices.Equal(args, want) {
		t.Errorf("PublishArgs = %v, want %v", args, want)
	}
}

// WebRTC has no GStreamer sink and no watch form; the capability helpers must
// report that instead of returning arguments.
func TestWebRTCCapabilities(t *testing.T) {
	s := settings.Stream{Name: "alice", RelayHost: "relay.example", WebrtcPort: 8889, Transport: "webrtc"}

	if _, ok := GstSink(s); ok {
		t.Error("GstSink must report false for webrtc")
	}
	if _, ok := WatchURL(s, "bob"); ok {
		t.Error("WatchURL must report false for webrtc")
	}
	if _, ok := PublishArgs(s); !ok {
		t.Error("PublishArgs must report true for webrtc")
	}
}
