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

// srtFormats are the bitstream formats SRT carries in both directions, which is
// what MediaMTX registers a stream type for in the MPEG-TS it ingests and
// re-serves: H.264 and H.265. AV1, VP9 and VP8 have no such mapping there, so a
// stream in one of them neither reaches an SRT publish nor comes back out of an
// SRT read, whatever the relay ingested it over.
var srtFormats = []string{"h264", "hevc"}

func (SRT) Formats() Formats {
	return Formats{Publish: srtFormats, Watch: srtFormats}
}

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

// GstSource returns the source elements a receiving GStreamer pipeline decodes
// from. As on the sink side, srtsrc takes streamid and latency (milliseconds)
// as properties on a bare srt:// URI; the buffer options in WatchURL are
// ffmpeg protocol knobs with no srtsrc equivalent.
func (SRT) GstSource(s settings.Stream, streamName string) []string {
	return []string{
		"srtsrc",
		fmt.Sprintf("uri=srt://%s:%d", s.RelayHost, s.RelayPort),
		"mode=caller",
		"streamid=read:" + streamName,
		fmt.Sprintf("latency=%d", s.SrtWatchLatencyMs),
	}
}

// srtWatchKnobs are the watch-leg knobs a viewer can change per stream, the
// settings fields GstSource and WatchURL read.
var srtWatchKnobs = []watchKnob{
	intKnob("srtWatchLatencyMs", "SRT latency (ms)",
		"Retransmit window of the watch leg (relay to viewer), where internet loss usually lives. "+
			"It is display delay: a lossy remote link wants more, a LAN less.",
		minWatchLatencyMs,
		func(s *settings.Stream) *int { return &s.SrtWatchLatencyMs }),
}

func (SRT) WatchOptions(s settings.Stream) []WatchOption { return knobOptions(srtWatchKnobs, s) }

func (t SRT) SetWatchOption(s *settings.Stream, key, value string) error {
	return knobSet(t.Name(), srtWatchKnobs, s, key, value)
}
