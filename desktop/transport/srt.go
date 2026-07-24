package transport

import (
	"fmt"

	"bjoernblessin.de/screenshare/settings"
)

// SRT streams via a MediaMTX relay. Every value here was proven in the script
// prototype; change them only with a measured reason:
//   - latency is MICROSECONDS in ffmpeg's srt protocol, not milliseconds.
//   - sndbuf/rcvbuf/ffs must be large so lossless keyframe bursts survive
//     while a display-paced player drains slowly.
//   - pkt_size 1316 = 7 MPEG-TS packets per SRT datagram.
type SRT struct{}

func init() {
	Register(SRT{})
}

// srtBufBytes sizes the SRT window and socket buffers (~150 MB).
const srtBufBytes = 150_000_000

func (SRT) Name() string { return "srt" }

func (SRT) PublishArgs(s settings.Stream) []string {
	// ffmpeg's srt protocol: latency in MICROSECONDS, and ffmpeg's own buffer
	// option names (pkt_size/sndbuf/ffs).
	url := fmt.Sprintf(
		"srt://%s:%d?streamid=publish:%s&pkt_size=1316&latency=%d&sndbuf=%d&ffs=%d",
		s.RelayHost, s.RelayPort, s.Name, s.SrtPublishLatencyMs*1000, srtBufBytes, srtBufBytes)

	return []string{"-f", "mpegts", url}
}

// GstSink returns the muxer and sink elements that terminate a GStreamer
// pipeline for this transport.
//
// GStreamer's srtsink uses libsrt, whose configuration differs from ffmpeg's srt
// protocol: the URI is a bare srt://host:port, and streamid and latency (in
// MILLISECONDS, not ffmpeg's microseconds) are separate properties. alignment=7
// packs 7 * 188-byte TS packets per buffer to match the SRT payload size.
func (SRT) GstSink(s settings.Stream) []string {
	return []string{
		"mpegtsmux", "name=" + GstMuxName, "alignment=7",
		"!", "srtsink",
		fmt.Sprintf("uri=srt://%s:%d", s.RelayHost, s.RelayPort),
		"mode=caller",
		"streamid=publish:" + s.Name,
		fmt.Sprintf("latency=%d", s.SrtPublishLatencyMs),
		"wait-for-connection=false",
	}
}

func (SRT) WatchURL(s settings.Stream, streamName string) string {
	return fmt.Sprintf(
		"srt://%s:%d?streamid=read:%s&latency=%d&rcvbuf=%d&ffs=%d",
		s.RelayHost, s.RelayPort, streamName, s.SrtWatchLatencyMs*1000, srtBufBytes, srtBufBytes)
}
