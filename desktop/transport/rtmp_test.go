package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/settings"
)

func rtmpTestStream() settings.Stream {
	return settings.Stream{Name: "alice", RelayHost: "relay.example", RtmpPort: 1935}
}

func TestRTMPRegistered(t *testing.T) {
	tr, ok := Get("rtmp")
	if !ok {
		t.Fatal("rtmp transport not registered")
	}
	if tr.Name() != "rtmp" {
		t.Fatalf("Name() = %q, want rtmp", tr.Name())
	}
}

func TestRTMPPublishArgs(t *testing.T) {
	args := RTMP{}.PublishArgs(rtmpTestStream())

	want := []string{"-f", "flv", "rtmp://relay.example:1935/alice"}
	if !slices.Equal(args, want) {
		t.Errorf("PublishArgs = %v, want %v", args, want)
	}
}

// The watch leg names the stream being watched, not the one being published: a
// viewer opens someone else's stream over a transport it picks per window.
func TestRTMPWatchURL(t *testing.T) {
	got := RTMP{}.WatchURL(rtmpTestStream(), "bob")
	if got != "rtmp://relay.example:1935/bob" {
		t.Errorf("WatchURL = %q, want the URL of the watched stream", got)
	}
}

func TestRTMPGstSource(t *testing.T) {
	src := RTMP{}.GstSource(rtmpTestStream(), "bob")

	want := []string{"rtmp2src", "location=rtmp://relay.example:1935/bob"}
	if !slices.Equal(src, want) {
		t.Errorf("GstSource = %v, want %v", src, want)
	}
}

// RTMP publishes through ffmpeg alone: flvmux writes the legacy tags only, so
// the GStreamer engine has no sink to terminate a pipeline with, and a capture
// backend on that engine must not be offered the transport at all.
func TestRTMPCapabilities(t *testing.T) {
	s := rtmpTestStream()
	s.Transport = "rtmp"

	if !CanFFmpegPublish("rtmp") {
		t.Error("CanFFmpegPublish must report true for rtmp")
	}
	if CanGstPublish("rtmp") {
		t.Error("CanGstPublish must report false for rtmp")
	}
	if _, ok := GstSink(s); ok {
		t.Error("GstSink must report false for rtmp")
	}
	if _, ok := WatchURL("rtmp", s, "bob"); !ok {
		t.Error("WatchURL must report true for rtmp")
	}
	if !CanGstWatch("rtmp") {
		t.Error("CanGstWatch must report true for rtmp")
	}
}
