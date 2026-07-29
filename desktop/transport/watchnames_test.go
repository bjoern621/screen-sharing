package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
)

// WatchNames is the set one engine can be pointed at, independent of the publish transport.
// The two engines differ by one entry each, and each difference is a leg the other engine
// cannot run: WHEP is a signaling exchange rather than an address, so no player URL expresses
// it, and nothing on the GStreamer side reads the relay's HLS playlist.
func TestWatchNamesArePerEngine(t *testing.T) {
	players := WatchNames(capabilities.EngineFfmpeg)
	pipeline := WatchNames(capabilities.EngineGst)

	for _, want := range []string{"srt", "rtsp", "rtmp", "hls"} {
		if !slices.Contains(players, want) {
			t.Errorf("WatchNames(ffmpeg) = %v, missing %q", players, want)
		}
	}
	if slices.Contains(players, "webrtc") {
		t.Errorf("WatchNames(ffmpeg) = %v, must exclude webrtc, which no viewer program opens", players)
	}
	for _, want := range []string{"srt", "rtsp", "rtmp", "webrtc"} {
		if !slices.Contains(pipeline, want) {
			t.Errorf("WatchNames(gstreamer) = %v, missing %q", pipeline, want)
		}
	}
	if slices.Contains(pipeline, "hls") {
		t.Errorf("WatchNames(gstreamer) = %v, must exclude hls, which no source element here decodes", pipeline)
	}
	for _, engine := range capabilities.Engines {
		if names := WatchNames(engine); !slices.IsSorted(names) {
			t.Errorf("WatchNames(%s) = %v, want sorted", engine, names)
		}
	}
}

// PublishNames is the publish dropdown's list, per engine.
// A protocol the relay serves but does not ingest has no publish form on either engine and
// stays out of both lists, rather than appearing greyed with a reason no capture backend
// could lift.
// RTMP is the per-engine difference: the flv muxer writes the enhanced-RTMP tags the relay
// ingests, and flvmux writes the legacy ones alone.
func TestPublishNamesArePerEngine(t *testing.T) {
	ffmpeg := PublishNames(capabilities.EngineFfmpeg)
	gst := PublishNames(capabilities.EngineGst)

	for _, want := range []string{"srt", "rtsp", "rtmp", "webrtc"} {
		if !slices.Contains(ffmpeg, want) {
			t.Errorf("PublishNames(ffmpeg) = %v, missing %q", ffmpeg, want)
		}
	}
	for _, want := range []string{"srt", "rtsp", "webrtc"} {
		if !slices.Contains(gst, want) {
			t.Errorf("PublishNames(gstreamer) = %v, missing %q", gst, want)
		}
	}
	if slices.Contains(gst, "rtmp") {
		t.Errorf("PublishNames(gstreamer) = %v, must exclude rtmp, which flvmux cannot write the relay's tags for", gst)
	}
	for _, engine := range capabilities.Engines {
		names := PublishNames(engine)
		if slices.Contains(names, "hls") {
			t.Errorf("PublishNames(%s) = %v, must exclude watch-only hls", engine, names)
		}
		if !slices.IsSorted(names) {
			t.Errorf("PublishNames(%s) = %v, want sorted", engine, names)
		}
	}
}
