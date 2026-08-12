package transport

import (
	"fmt"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// WebRTC is the relay's WHIP/WHEP pair (RFC 9725): the publisher offers SDP over
// an HTTP POST and then ships SRTP to the relay, and a viewer takes the same
// exchange the other way round to receive it.
//
// Both publish engines carry the ingest, each limited by what its own side
// negotiates. On the watch leg WHEP is a signaling exchange rather than a URL, so
// no viewer program opens it and this transport implements GstWatcher and
// BrowserWatcher without Watcher: a receiving pipeline runs the exchange itself,
// and a browser runs it from the page the relay serves, through its own
// RTCPeerConnection.
type WebRTC struct{}

func init() {
	Register(WebRTC{})
}

func (WebRTC) Name() string { return "webrtc" }

// Formats: the two publish entries are what each engine's WHIP side negotiates,
// and they differ. ffmpeg's whip muxer writes one H.264 video track and one Opus
// audio track and has no payloader for anything else. whipclientsink picks its
// payloader from the caps webrtcbin offers, which covers the WebRTC video set the
// relay ingests, so the GStreamer engine reaches VP8 and VP9 over the same
// endpoint.
//
// AV1 is in neither set. Its WHEP track negotiates and then yields no frame here,
// measured: the relay reports the reader, and neither an autoplugged nor an
// explicit rtpav1depay chain produces a picture from it. A publish leg nothing
// can read back is not a leg.
//
// Opus is the whole audio set on every entry, because it is the whole audio set
// WebRTC negotiates. AAC has no SDP form there at all, which is a fact of the
// protocol rather than of either engine.
//
// The two watch entries are the receiving GStreamer pipeline's and the browser's,
// and there is no player entry: WHEP is a signaling exchange rather than an
// address, so no viewer program opens it. H.265 is absent from both because the
// relay refuses to serve it over WebRTC whenever the stream carries B-frames,
// which is a property of the encode rather than of the leg and unknowable for a
// stream this app did not produce.
//
// The browser set is the same three formats and states the same thing about the
// relay's listener, arrived at from the reader's side: every browser that
// negotiates WebRTC decodes H.264 and VP8, and the ones this page is opened in
// decode VP9. AV1 is off both watch entries for the reason it is off the publish
// ones - the leg yielded no picture here - and neither reader is offered a leg
// this app has not seen carry a stream.
// webrtcPlayback is what the relay serves over WHEP, named once because both
// readers take it off the same listener rather than agreeing by coincidence.
var webrtcPlayback = Carriage{
	Video: []string{"h264", "vp9", "vp8"},
	Audio: []string{"opus"},
}

func (WebRTC) Formats() Formats {
	return Formats{
		Publish: map[string]Carriage{
			capabilities.EngineFfmpeg: {Video: []string{"h264"}, Audio: []string{"opus"}},
			capabilities.EngineGst:    {Video: []string{"h264", "vp9", "vp8"}, Audio: []string{"opus"}},
		},
		Watch: map[string]Carriage{
			capabilities.EngineGst: webrtcPlayback,
			EngineBrowser:          webrtcPlayback,
		},
	}
}

func (WebRTC) PublishArgs(s settings.Settings) []string {
	return []string{"-f", "whip", whipURL(s, s.Publish.Name)}
}

// GstSink returns the sink terminating a GStreamer pipeline for this transport.
//
// whipclientsink is muxer and sink in one, as rtspclientsink is: it takes a parsed
// elementary stream on a request pad, picks the pad template by the caps offered, and
// payloads each into its own track of the WebRTC session. It therefore carries the
// GstMuxName, which is where the pipeline's audio branch attaches.
//
// The endpoint is a property of the element's signaller object rather than of the
// element, so it is set through the child-property syntax.
func (WebRTC) GstSink(s settings.Settings) []string {
	return []string{
		"whipclientsink", "name=" + GstMuxName,
		"signaller::whip-endpoint=" + whipURL(s, s.Publish.Name),
	}
}

// GstSource returns the source element a receiving GStreamer pipeline decodes
// from. whepsrc runs the WHEP exchange and yields one RTP pad per negotiated
// track, the same shape rtspsrc has, which is what lets the receiver's decodebin
// pick the depayloader and decoder from the caps.
//
// audio-caps=EMPTY is what makes the exchange work at all: with an audio section
// in the offer, the element marks the video section bundle-only on port 0 and
// the relay answers 400 "codecs not supported by client". Dropping the audio
// proposal moves video into the first section and the answer arrives, at the
// cost of this leg carrying video alone.
func (WebRTC) GstSource(s settings.Settings, streamName string) []string {
	return []string{
		"whepsrc",
		"whep-endpoint=" + whepURL(s, streamName),
		"audio-caps=EMPTY",
	}
}

// BrowserURL returns the relay's WHEP player page for the stream. The page runs
// the same exchange whepsrc does, from a browser's own RTCPeerConnection, and it
// is served on the WebRTC listener rather than anywhere this app serves: the
// address is the path's, with the trailing slash the relay would otherwise
// redirect to.
func (WebRTC) BrowserURL(s settings.Settings, streamName string) string {
	return fmt.Sprintf("http://%s:%d/%s/", s.Relay.Host, s.Relay.WebrtcPort, streamName)
}

func whipURL(s settings.Settings, name string) string {
	return fmt.Sprintf("http://%s:%d/%s/whip", s.Relay.Host, s.Relay.WebrtcPort, name)
}

func whepURL(s settings.Settings, name string) string {
	return fmt.Sprintf("http://%s:%d/%s/whep", s.Relay.Host, s.Relay.WebrtcPort, name)
}
