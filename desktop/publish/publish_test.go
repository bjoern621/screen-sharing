package publish

import (
	"slices"
	"testing"
)

// The ffmpeg-engine captures carry every transport that serializes to an ffmpeg
// command; the portal backend runs on GStreamer, so it drops WebRTC, which
// has no GStreamer sink.
func TestTransportsFor(t *testing.T) {
	cases := map[string][]string{
		"x11grab": {"rtsp", "srt", "webrtc"},
		"kmsgrab": {"rtsp", "srt", "webrtc"},
		"ddagrab": {"rtsp", "srt", "webrtc"},
		"gdigrab": {"rtsp", "srt", "webrtc"},
		"portal":  {"rtsp", "srt"},
	}
	for capture, want := range cases {
		got, err := TransportsFor(capture)
		if err != nil {
			t.Errorf("TransportsFor(%q) error: %v", capture, err)
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("TransportsFor(%q) = %v, want %v", capture, got, want)
		}
	}
}

func TestTransportsForUnknownCapture(t *testing.T) {
	if _, err := TransportsFor("nope"); err == nil {
		t.Error("TransportsFor unknown capture must error")
	}
}
