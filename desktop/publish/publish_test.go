package publish

import (
	"slices"
	"testing"
)

// A capture backend carries every transport its engine can serialize through, so
// the two engines part where a transport implements one publish form only: RTMP
// has an ffmpeg muxer and no GStreamer counterpart, which leaves it off the
// portal backend and on every ffmpeg one.
func TestTransportsFor(t *testing.T) {
	cases := map[string][]string{
		"x11grab": {"rtmp", "rtsp", "srt", "webrtc"},
		"kmsgrab": {"rtmp", "rtsp", "srt", "webrtc"},
		"ddagrab": {"rtmp", "rtsp", "srt", "webrtc"},
		"gdigrab": {"rtmp", "rtsp", "srt", "webrtc"},
		"portal":  {"rtsp", "srt", "webrtc"},
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
