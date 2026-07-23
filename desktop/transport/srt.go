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
	url := fmt.Sprintf(
		"srt://%s:%d?streamid=publish:%s&pkt_size=1316&latency=%d&sndbuf=%d&ffs=%d",
		s.RelayHost, s.RelayPort, s.Name, s.SrtPublishLatencyMs*1000, srtBufBytes, srtBufBytes)

	return []string{"-f", "mpegts", url}
}

func (SRT) WatchURL(s settings.Stream, streamName string) string {
	return fmt.Sprintf(
		"srt://%s:%d?streamid=read:%s&latency=%d&rcvbuf=%d&ffs=%d",
		s.RelayHost, s.RelayPort, streamName, s.SrtWatchLatencyMs*1000, srtBufBytes, srtBufBytes)
}
