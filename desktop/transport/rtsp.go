package transport

import (
	"fmt"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// RTSP streams through the relay's RTSP listener. Each track travels as its
// own RTP stream inside the RTSP session, so no MPEG-TS mux is involved.
//
// Both serializations force TCP-interleaved RTP. The RTSP default negotiates a
// separate UDP port pair per track, which NAT and firewalls drop silently, and
// lost RTP over UDP is never retransmitted; interleaving rides the one RTSP
// TCP connection instead.
type RTSP struct{}

func init() {
	Register(RTSP{})
}

func (RTSP) Name() string { return "rtsp" }

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
// from. latency sizes the rtpjitterbuffer in milliseconds; rtspsrc's 2000 ms
// default adds two seconds of display delay, far above what a LAN needs.
func (RTSP) GstSource(s settings.Stream, streamName string) []string {
	return []string{
		"rtspsrc",
		"location=" + rtspURL(s, streamName),
		"protocols=tcp",
		"latency=200",
	}
}

func rtspURL(s settings.Stream, name string) string {
	return fmt.Sprintf("rtsp://%s:%d/%s", s.RelayHost, s.RtspPort, name)
}
