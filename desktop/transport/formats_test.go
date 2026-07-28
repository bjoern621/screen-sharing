package transport

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
)

// A leg's format set and that leg's serialization capability are two halves of
// one statement. A transport that declares formats it cannot serialize offers a
// combination nothing can build, and one that serializes a leg it declares no
// format for is offered for streams and refused for every one of them.
func TestFormatSetsMatchCapabilities(t *testing.T) {
	for _, name := range Names() {
		tr, _ := Get(name)
		f := tr.Formats()

		_, ffmpeg := tr.(FFmpegPublisher)
		_, gst := tr.(GstPublisher)
		if publishes := ffmpeg || gst; publishes != (len(f.Publish) > 0) {
			t.Errorf("%s: publish capability %v, publish formats %v", name, publishes, f.Publish)
		}

		_, url := tr.(Watcher)
		_, pipeline := tr.(GstWatcher)
		if watches := url || pipeline; watches != (len(f.Watch) > 0) {
			t.Errorf("%s: watch capability %v, watch formats %v", name, watches, f.Watch)
		}
	}
}

// Every format a codec here produces needs a way out and a way back: one
// transport that publishes it and one that a viewer receives it over. A format
// with neither is a codec the settings form offers and no stream can use.
func TestEveryFormatHasBothLegs(t *testing.T) {
	for _, format := range capabilities.Formats() {
		if len(PublishNamesFor(format)) == 0 {
			t.Errorf("format %s has no publish transport", format)
		}
		if len(WatchNamesFor(format)) == 0 {
			t.Errorf("format %s has no watch transport", format)
		}
	}
}

func TestCarriesFormat(t *testing.T) {
	cases := []struct {
		transport, format  string
		publish, watchable bool
	}{
		// MPEG-TS registers a stream type for H.264 and H.265 and for neither of
		// the others, in both directions.
		{"srt", "h264", true, true},
		{"srt", "vp9", false, false},
		{"srt", "av1", false, false},
		// RTP has a payload format for the whole table.
		{"rtsp", "av1", true, true},
		{"rtsp", "vp8", true, true},
		// The relay serves HLS and ingests none of it.
		{"hls", "h264", false, true},
		{"hls", "vp8", false, false},
		// RTMP is the asymmetric one: the flv muxer writes enhanced-RTMP tags the
		// relay ingests, and the FLV demuxers behind the viewers read H.264 alone.
		{"rtmp", "hevc", true, false},
		{"rtmp", "h264", true, true},
		// WHIP ingest is H.264; WHEP playback reaches the two VPx formats as well.
		{"webrtc", "h264", true, true},
		{"webrtc", "vp9", false, true},
		{"webrtc", "hevc", false, false},
		{"nope", "h264", false, false},
	}
	for _, tc := range cases {
		if got := CanPublishFormat(tc.transport, tc.format); got != tc.publish {
			t.Errorf("CanPublishFormat(%s, %s) = %v, want %v", tc.transport, tc.format, got, tc.publish)
		}
		if got := CanWatchFormat(tc.transport, tc.format); got != tc.watchable {
			t.Errorf("CanWatchFormat(%s, %s) = %v, want %v", tc.transport, tc.format, got, tc.watchable)
		}
	}
}

// The watch lists narrow per format, and a format no codec here produces narrows
// nothing: the relay snapshot can be older than the stream, so absent
// information must not take a working choice away.
func TestWatchNamesForNarrowsByFormat(t *testing.T) {
	if got := WatchNamesFor("vp9"); slices.Contains(got, "srt") {
		t.Errorf("WatchNamesFor(vp9) = %v, must exclude srt", got)
	}
	if got := WatchNamesFor("h264"); !slices.Contains(got, "srt") {
		t.Errorf("WatchNamesFor(h264) = %v, must include srt", got)
	}
	if got, all := WatchNamesFor("mpeg2"), WatchNames(); !slices.Equal(got, all) {
		t.Errorf("WatchNamesFor(unknown format) = %v, want every watch transport %v", got, all)
	}
	if got, all := WatchNamesFor(""), WatchNames(); !slices.Equal(got, all) {
		t.Errorf("WatchNamesFor(empty) = %v, want every watch transport %v", got, all)
	}
	// The grid's list is the wider one, since WHEP has no player URL.
	if got := GstWatchNamesFor("vp9"); !slices.Contains(got, "webrtc") {
		t.Errorf("GstWatchNamesFor(vp9) = %v, must include webrtc", got)
	}
}

func TestValidatePublish(t *testing.T) {
	if err := ValidatePublish("srt", "hevc_nvenc"); err != nil {
		t.Errorf("srt carries hevc_nvenc: %v", err)
	}
	if err := ValidatePublish("nope", "hevc_nvenc"); err == nil {
		t.Error("an unknown transport must be refused")
	}
	if err := ValidatePublish("srt", "nope"); err == nil {
		t.Error("an unknown codec must be refused")
	}
	// A refusal names where the codec would have worked, since that is the
	// change the user has to make.
	err := ValidatePublish("srt", "libvpx-vp9")
	if err == nil {
		t.Fatal("srt/MPEG-TS has no VP9 mapping and must be refused")
	}
	if !strings.Contains(err.Error(), "rtsp") {
		t.Errorf("refusal %q must name the transports that carry VP9", err)
	}
}
