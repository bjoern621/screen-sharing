package transport

import (
	"fmt"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// RTSP streams through the relay's RTSP listener. Each track travels as its
// own RTP stream inside the RTSP session, so no MPEG-TS mux is involved.
//
// The publish serializations force TCP-interleaved RTP. The RTSP default
// negotiates a separate UDP port pair per track, which NAT and firewalls drop
// silently, and lost RTP over UDP is never retransmitted; interleaving rides
// the one RTSP TCP connection instead. The watch leg takes the same choice from
// settings.Stream.RtspWatchProtocol, because a viewer on the same LAN as the
// relay can trade that safety for UDP's lower delay.
type RTSP struct{}

func init() {
	Register(RTSP{})
}

func (RTSP) Name() string { return "rtsp" }

// rtspFormats are the bitstream formats RTSP carries in both directions. RTP has
// a payload format for every format the app encodes and the relay ingests and
// re-serves all of them, which makes RTSP the one transport that carries the
// whole codec table and the fallback the other legs point at.
var rtspFormats = []string{"h264", "hevc", "av1", "vp9", "vp8"}

func (RTSP) Formats() Formats {
	return Formats{Publish: rtspFormats, Watch: rtspFormats}
}

// draftRtpFormats are the video formats whose RTP payload format is still an IETF
// draft. ffmpeg's RTP muxer refuses to write one unless compliance is loosened,
// failing the publish with "Could not write header" before a frame is sent. The
// relay ingests both regardless, so the flag is the whole difference between a
// working AV1 or VP9 publish and none. It is keyed by format rather than codec
// because the payload format follows the bitstream, not the encoder that made it.
var draftRtpFormats = map[string]bool{"vp9": true, "av1": true}

// PublishArgs returns the ffmpeg output args for this transport. The RTSP muxer
// wraps the RTP muxer, so the draft-payload flag applies here as well.
func (RTSP) PublishArgs(s settings.Stream) []string {
	args := []string{"-f", "rtsp", "-rtsp_transport", "tcp"}
	if c, ok := capabilities.Get(s.Codec); ok && draftRtpFormats[c.Format] {
		args = append(args, "-strict", "experimental")
	}
	return append(args, rtspURL(s, s.Name))
}

// GstSink returns the sink terminating a GStreamer pipeline for this transport.
//
// rtspclientsink is muxer and sink in one: it payloads every attached parsed
// stream into its own RTP track of the same session. It therefore carries the
// GstMuxName, which is where the pipeline's audio branch attaches.
func (RTSP) GstSink(s settings.Stream) []string {
	return []string{
		"rtspclientsink", "name=" + GstMuxName,
		"protocols=tcp",
		"location=" + rtspURL(s, s.Name),
	}
}

func (RTSP) WatchURL(s settings.Stream, streamName string) string {
	return rtspURL(s, streamName)
}

// GstSource returns the source elements a receiving GStreamer pipeline decodes
// from. Both knobs are the watch-leg settings: latency sizes the
// rtpjitterbuffer in milliseconds, protocols names the RTP lower transport
// rtspsrc offers the relay. Neither is left at the element's own default, which
// is a 2000 ms buffer and a UDP-first negotiation.
func (RTSP) GstSource(s settings.Stream, streamName string) []string {
	return []string{
		"rtspsrc",
		"location=" + rtspURL(s, streamName),
		"protocols=" + s.RtspWatchProtocol,
		fmt.Sprintf("latency=%d", s.RtspWatchLatencyMs),
	}
}

// rtspProtocols are the RTP lower transports rtspsrc is offered on the watch
// leg, the values RtspWatchProtocol takes.
var rtspProtocols = []string{"tcp", "udp"}

// rtspWatchKnobs are the watch-leg knobs a viewer can change per stream, the
// settings fields GstSource reads. The publish leg has none: it is TCP-interleaved
// regardless.
var rtspWatchKnobs = []watchKnob{
	intKnob("rtspWatchLatencyMs", "RTSP jitter buffer (ms)",
		"How long the receiver holds RTP packets before decoding, to reorder them and absorb network jitter. "+
			"It is display delay, so it belongs just above the link's jitter: 200 ms suits a LAN, a lossy remote link wants more.",
		minWatchLatencyMs,
		func(s *settings.Stream) *int { return &s.RtspWatchLatencyMs }),
	choiceKnob("rtspWatchProtocol", "RTSP transport",
		"How RTP reaches the viewer inside the RTSP session. UDP takes a port pair per track and trades retransmission for delay; "+
			"TCP interleaves both tracks on the RTSP connection, which NAT and firewalls let through.",
		rtspProtocols,
		func(s *settings.Stream) *string { return &s.RtspWatchProtocol }),
}

func (RTSP) WatchOptions(s settings.Stream) []WatchOption { return knobOptions(rtspWatchKnobs, s) }

func (t RTSP) SetWatchOption(s *settings.Stream, key, value string) error {
	return knobSet(t.Name(), rtspWatchKnobs, s, key, value)
}

func rtspURL(s settings.Stream, name string) string {
	return fmt.Sprintf("rtsp://%s:%d/%s", s.RelayHost, s.RtspPort, name)
}
