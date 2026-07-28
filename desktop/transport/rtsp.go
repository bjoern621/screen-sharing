package transport

import (
	"fmt"
	"slices"
	"strings"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// RTSP streams through the relay's RTSP listener. Each track travels as its
// own RTP stream inside the RTSP session, so no MPEG-TS mux is involved.
//
// Each leg names the RTP lower transport it runs over: the publish leg from
// settings.Stream.RtspPublishProtocol, the watch leg from RtspWatchProtocol.
// TCP interleaves both tracks on the connection the RTSP session already holds.
// UDP takes a port pair per track, which drops the delay in-order delivery adds;
// RTP lost either way is never retransmitted.
//
// The port pair is what makes TCP the default on both legs. It is negotiated
// separately from the RTSP connection, so a client behind NAT reaches it only by
// sending from those ports first: that creates the mapping, and the relay has to
// answer into it rather than at the port SETUP announced, which is the private
// one the NAT rewrote. The publish leg does the sending with the media itself;
// the watch leg has to open the path with probe packets before the first RTP can
// come back. Either way a network that drops outbound UDP ends it, which is a
// fact of the network that leg crosses and of nothing in here, and the failure
// is silent: the session sets up over TCP and no RTP follows.
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
	args := []string{"-f", "rtsp", "-rtsp_transport", s.RtspPublishProtocol}
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
		"protocols=" + s.RtspPublishProtocol,
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

// rtspProtocols are the RTP lower transports this protocol offers, the values
// both RtspPublishProtocol and RtspWatchProtocol take. One list for both legs:
// which transports carry RTP is a fact of RTSP, not of a direction.
var rtspProtocols = []string{"tcp", "udp"}

// ValidatePublishSettings rejects a lower transport RTSP does not run over.
// ffmpeg's -rtsp_transport and rtspclientsink's protocols property both take a
// fixed set of names, so a value outside it fails inside the publish process,
// where the reason reaches the user as another program's error text.
func (RTSP) ValidatePublishSettings(s settings.Stream) error {
	if !slices.Contains(rtspProtocols, s.RtspPublishProtocol) {
		return fmt.Errorf("rtsp publish protocol %q is not one of %s",
			s.RtspPublishProtocol, strings.Join(rtspProtocols, ", "))
	}
	return nil
}

// rtspWatchKnobs are the watch-leg knobs a viewer can change per stream, the
// settings fields GstSource reads. The publish leg's counterpart is a field of
// the settings form: it is chosen once for the stream this machine sends, not
// per stream it receives.
var rtspWatchKnobs = []watchKnob{
	intKnob("rtspWatchLatencyMs", "RTSP jitter buffer (ms)",
		"How long the receiver holds RTP packets before decoding, to reorder them and absorb network jitter. "+
			"It is display delay, so it belongs just above the link's jitter: 200 ms suits a LAN, a lossy remote link wants more.",
		minWatchLatencyMs,
		func(s *settings.Stream) *int { return &s.RtspWatchLatencyMs }),
	choiceKnob("rtspWatchProtocol", "RTSP transport",
		"How RTP reaches the viewer inside the RTSP session. UDP takes a port pair per track and trades retransmission for delay, "+
			"but the media travels toward the viewer on it, so nothing arrives until the viewer's own probe packets have opened "+
			"the mapping through its NAT and the relay answers where they came from. "+
			"TCP interleaves both tracks on the RTSP connection, which needs no second port and is what a filtering network is likeliest to pass.",
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
