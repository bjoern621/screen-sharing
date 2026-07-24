package transport

import (
	"fmt"

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

func (RTSP) PublishArgs(s settings.Stream) []string {
	return []string{"-f", "rtsp", "-rtsp_transport", "tcp", rtspURL(s, s.Name)}
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
