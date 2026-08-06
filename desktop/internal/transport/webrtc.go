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
// negotiates. The watch leg is a receiving GStreamer pipeline's alone: WHEP is a
// signaling exchange rather than a URL, so no viewer program opens it and this
// transport implements GstWatcher without Watcher. The web viewer reaches the
// same endpoint through the browser's own RTCPeerConnection (WhepSink), which is
// not this registry's business.
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
// The watch entry is the receiving GStreamer pipeline's, and there is no player
// entry: WHEP is a signaling exchange rather than an address, so no viewer
// program opens it. H.265 is absent because the relay refuses to serve it over
// WebRTC whenever the stream carries B-frames, which is a property of the encode
// rather than of the leg and unknowable for a stream this app did not produce.
func (WebRTC) Formats() Formats {
	return Formats{
		Publish: map[string]Carriage{
			capabilities.EngineFfmpeg: {Video: []string{"h264"}, Audio: []string{"opus"}},
			capabilities.EngineGst:    {Video: []string{"h264", "vp9", "vp8"}, Audio: []string{"opus"}},
		},
		Watch: map[string]Carriage{capabilities.EngineGst: {
			Video: []string{"h264", "vp9", "vp8"},
			Audio: []string{"opus"},
		}},
	}
}

func (WebRTC) PublishArgs(s settings.Stream) []string {
	return []string{"-f", "whip", whipURL(s, s.Name)}
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
func (WebRTC) GstSink(s settings.Stream) []string {
	return []string{
		"whipclientsink", "name=" + GstMuxName,
		"signaller::whip-endpoint=" + whipURL(s, s.Name),
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
func (WebRTC) GstSource(s settings.Stream, streamName string) []string {
	return []string{
		"whepsrc",
		"whep-endpoint=" + whepURL(s, streamName),
		"audio-caps=EMPTY",
	}
}

func whipURL(s settings.Stream, name string) string {
	return fmt.Sprintf("http://%s:%d/%s/whip", s.RelayHost, s.WebrtcPort, name)
}

func whepURL(s settings.Stream, name string) string {
	return fmt.Sprintf("http://%s:%d/%s/whep", s.RelayHost, s.WebrtcPort, name)
}
