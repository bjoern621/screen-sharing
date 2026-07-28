package relay

import "testing"

// Which protocols can carry a stream follows its bitstream format, and the relay
// names a track after the format rather than the encoder. A track name with no
// entry leaves the format empty, which the callers read as "unknown" and act on
// by not refusing anything: a snapshot older than the stream must not block a
// viewer that would have worked.
func TestFormatOfTracks(t *testing.T) {
	cases := []struct {
		tracks []string
		want   string
	}{
		{tracks: []string{"H264"}, want: "h264"},
		{tracks: []string{"AVC"}, want: "h264"},
		{tracks: []string{"H265"}, want: "hevc"},
		{tracks: []string{"HEVC"}, want: "hevc"},
		{tracks: []string{"VP9"}, want: "vp9"},
		{tracks: []string{"AV1"}, want: "av1"},
		{tracks: []string{"VP8"}, want: "vp8"},
		// The audio track of a muxed stream is not the format the transport question
		// is about, so the video one answers wherever it sits.
		{tracks: []string{"Opus", "AV1"}, want: "av1"},
		{tracks: []string{"h264"}, want: "h264"},
		{tracks: []string{"Opus"}, want: ""},
		{tracks: nil, want: ""},
	}
	for _, c := range cases {
		if got := formatOfTracks(c.tracks); got != c.want {
			t.Errorf("formatOfTracks(%v) = %q, want %q", c.tracks, got, c.want)
		}
	}
}
