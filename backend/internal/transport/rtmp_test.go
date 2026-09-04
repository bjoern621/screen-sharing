package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

func rtmpTestStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:     "10.0.0.5",
			RtmpPort: 1936,
			GroupKey: testGroupKey.String(),
		},
	}
}

// rtmpPath is where the fixture's stream lives on the relay:
// a stream lives in a group, so the path leads with that group's prefix.
var rtmpPath = testGroupKey.Prefix() + "monitor-0"

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

	want := []string{"-f", "flv", "-tls_verify", "0", "rtmps://10.0.0.5:1936/" + rtmpPath}
	if !slices.Equal(args, want) {
		t.Errorf("PublishArgs = %v, want %v", args, want)
	}
}

// The watch leg names the stream being watched and not the one being published: a viewer opens
// someone else's stream, over a transport it picks per window.
func TestRTMPWatchURL(t *testing.T) {
	got := RTMP{}.WatchURL(rtmpTestStream(), "bob")
	if got != "rtmps://10.0.0.5:1936/bob" {
		t.Errorf("WatchURL = %q, want the URL of the watched stream", got)
	}
}

func TestRTMPGstSource(t *testing.T) {
	src := RTMP{}.GstSource(rtmpTestStream(), "bob")

	want := []string{"rtmp2src", "location=rtmps://10.0.0.5:1936/bob", "tls-validation-flags=no-flags"}
	if !slices.Equal(src, want) {
		t.Errorf("GstSource = %v, want %v", src, want)
	}
}

// flvmux writes the legacy tags alone, so the GStreamer engine has no sink to terminate a publish
// pipeline with and a capture backend on that engine is never offered the transport.
func TestRTMPCapabilities(t *testing.T) {
	s := rtmpTestStream()
	s.Publish.Transport = "rtmp"

	if !CanPublish("rtmp", capabilities.EngineFfmpeg) {
		t.Error("CanPublish must report true for rtmp on the ffmpeg engine")
	}
	if CanPublish("rtmp", capabilities.EngineGst) {
		t.Error("CanPublish must report false for rtmp on the gstreamer engine")
	}
	if _, ok := GstSink(s); ok {
		t.Error("GstSink must report false for rtmp")
	}
	if _, ok := WatchURL("rtmp", s, "bob"); !ok {
		t.Error("WatchURL must report true for rtmp")
	}
	// Both readers take the legacy FLV tags, so the leg is watchable on the engine it cannot publish
	// on.
	for _, engine := range capabilities.Engines {
		if !CanWatch("rtmp", engine) {
			t.Errorf("CanWatch must report true for rtmp on the %s engine", engine)
		}
	}
}
