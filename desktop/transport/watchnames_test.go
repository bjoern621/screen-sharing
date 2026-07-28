package transport

import (
	"slices"
	"testing"
)

// WatchNames is the set a viewer program can be pointed at, independent of the
// publish transport. It must list every transport with a URL watch form and
// exclude one whose playback is a signaling exchange (webrtc, played over WHEP).
func TestWatchNames(t *testing.T) {
	names := WatchNames()

	for _, want := range []string{"srt", "rtsp", "rtmp", "hls"} {
		if !slices.Contains(names, want) {
			t.Errorf("WatchNames() = %v, missing %q", names, want)
		}
	}
	if slices.Contains(names, "webrtc") {
		t.Errorf("WatchNames() = %v, must exclude webrtc, which no viewer program opens", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("WatchNames() = %v, want sorted", names)
	}
}

// PublishNames is the publish dropdown's list. A protocol the relay serves but
// does not ingest has no publish form at all and must stay out of it, rather
// than appearing greyed with a reason no capture backend could lift.
func TestPublishNames(t *testing.T) {
	names := PublishNames()

	for _, want := range []string{"srt", "rtsp", "rtmp", "webrtc"} {
		if !slices.Contains(names, want) {
			t.Errorf("PublishNames() = %v, missing %q", names, want)
		}
	}
	if slices.Contains(names, "hls") {
		t.Errorf("PublishNames() = %v, must exclude watch-only hls", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("PublishNames() = %v, want sorted", names)
	}
}

// The two watch lists answer different questions, so a transport reachable by a
// receiving pipeline and not by a player belongs to one and not the other.
func TestGstWatchNames(t *testing.T) {
	names := GstWatchNames()

	for _, want := range []string{"srt", "rtsp", "rtmp", "webrtc"} {
		if !slices.Contains(names, want) {
			t.Errorf("GstWatchNames() = %v, missing %q", names, want)
		}
	}
	if slices.Contains(names, "hls") {
		t.Errorf("GstWatchNames() = %v, must exclude hls, which no source element here decodes", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("GstWatchNames() = %v, want sorted", names)
	}
}
