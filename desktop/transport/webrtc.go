package transport

import (
	"fmt"

	"bjoernblessin.de/screenshare/settings"
)

// WebRTC publishes to the relay's WHIP endpoint (RFC 9725): ffmpeg's whip
// muxer offers SDP over HTTP POST, then ships SRTP directly to the relay.
//
// The muxer carries H.264 and Opus, which is why the capability table lists
// only the H.264 codecs for this transport.
//
// It implements no GStreamer sink and no watch form. The GStreamer counterpart
// (whipclientsink) speaks RTP on its pads, so it cannot take the parsed
// elementary streams the publish pipeline's mux contract hands over. Playback
// needs WHEP, which neither ffplay nor mpv speaks; a viewer engine for it is a
// separate watch capability.
type WebRTC struct{}

func init() {
	Register(WebRTC{})
}

func (WebRTC) Name() string { return "webrtc" }

func (WebRTC) PublishArgs(s settings.Stream) []string {
	url := fmt.Sprintf("http://%s:%d/%s/whip", s.RelayHost, s.WebrtcPort, s.Name)
	return []string{"-f", "whip", url}
}
