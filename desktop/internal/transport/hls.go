package transport

import (
	"fmt"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// HLS is a watch-only leg. The relay cuts an ingested stream into segments and
// serves them with a playlist over plain HTTP; it takes no HLS ingest, so this
// transport declares no publish form at all and the publish dropdown never
// offers it.
//
// It is the leg that gets through where the others do not: one HTTP port, which
// proxies and firewalls pass and every browser plays. What it trades for that is
// delay. A viewer cannot start before a segment exists, so the relay's
// low-latency variant shortens the wait to a part of a segment rather than
// removing it, which puts HLS an order of magnitude behind SRT and RTSP here.
type HLS struct{}

func init() {
	Register(HLS{})
}

func (HLS) Name() string { return "hls" }

// Formats: the relay's HLS muxer segments H.264 and H.265 into MPEG-TS and adds
// AV1 and VP9 in its fMP4 form, with Opus and AAC beside them. VP8 has no segment
// form there at all, so a VP8 stream is the one the playlist never carries.
//
// The watch leg is the players' alone and there is no publish leg: the relay
// serves HLS and does not read it, and nothing on the GStreamer side reads the
// relay's playlist, so neither engine states a set it cannot build a pipeline
// for.
func (HLS) Formats() Formats {
	return Formats{Watch: map[string]Carriage{capabilities.EngineFfmpeg: {
		Video: []string{"h264", "hevc", "av1", "vp9"},
		Audio: []string{"opus", "aac"},
	}}}
}

// WatchURL returns the master playlist a player opens. Every viewer here follows
// the relay's redirect to the variant playlist on its own.
func (HLS) WatchURL(s settings.Stream, streamName string) string {
	return fmt.Sprintf("http://%s:%d/%s/index.m3u8", s.RelayHost, s.HlsPort, streamName)
}
