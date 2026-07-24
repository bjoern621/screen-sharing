package transport

import (
	"slices"
	"testing"
)

// WatchNames is the set a stream can be received over, independent of the
// publish transport. It must list every transport with a watch form and exclude
// one that only publishes (webrtc, whose playback needs WHEP).
func TestWatchNames(t *testing.T) {
	names := WatchNames()

	for _, want := range []string{"srt", "rtsp"} {
		if !slices.Contains(names, want) {
			t.Errorf("WatchNames() = %v, missing %q", names, want)
		}
	}
	if slices.Contains(names, "webrtc") {
		t.Errorf("WatchNames() = %v, must exclude publish-only webrtc", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("WatchNames() = %v, want sorted", names)
	}
}
