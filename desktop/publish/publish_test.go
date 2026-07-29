package publish

import (
	"slices"
	"testing"
)

// A capture backend carries every transport its engine can serialize through, so
// the two engines part where a transport implements one publish form only: RTMP
// has an ffmpeg muxer and no GStreamer counterpart, which leaves it off every
// GStreamer backend and on every ffmpeg one.
//
// Every registered backend is named, so a row added without an expectation fails
// here rather than shipping a transport list nothing checked.
func TestTransportsFor(t *testing.T) {
	cases := map[string][]string{
		"x11grab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"kmsgrab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"ddagrab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"gdigrab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"avfoundation":          {"rtmp", "rtsp", "srt", "webrtc"},
		"portal":                {"rtsp", "srt", "webrtc"},
		"ximagesrc":             {"rtsp", "srt", "webrtc"},
		"avfvideosrc":           {"rtsp", "srt", "webrtc"},
		"d3d11screencapturesrc": {"rtsp", "srt", "webrtc"},
	}
	for capture := range captureBackends {
		if _, ok := cases[capture]; !ok {
			t.Errorf("capture backend %q states no expected transport list", capture)
		}
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
