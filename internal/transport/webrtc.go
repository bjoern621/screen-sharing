package transport

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// WebRTC is the relay's WHIP/WHEP pair (RFC 9725).
// The publisher offers SDP over an HTTP POST and then ships SRTP; a viewer runs the same exchange
// the other way round.
//
// WHEP is a signalling exchange rather than an address, so no viewer program opens it.
// Hence GstWatcher and BrowserWatcher without Watcher: a receiving pipeline runs the exchange
// itself, and a browser runs it from the page the relay serves.
type WebRTC struct{}

func init() {
	Register(WebRTC{})
}

func (WebRTC) Name() string { return "webrtc" }

// webrtcPlayback is what the relay serves over WHEP, named once because both readers take it off
// the one listener rather than agreeing by coincidence.
//
// H.265 is absent: the relay refuses to serve it over WebRTC whenever the stream carries B-frames,
// a property of the encode rather than of the leg and unknowable for a stream this app did not
// produce.
// AV1 is absent for the reason it is off the publish sets, and neither reader is offered a leg this
// app has not seen carry a stream.
// The browser reaches the same three from its own side: every browser that negotiates WebRTC
// decodes H.264 and VP8, and the ones this page is opened in decode VP9.
var webrtcPlayback = Carriage{
	Video: []string{"h264", "vp9", "vp8"},
	Audio: []string{"opus"},
}

// Formats states a publish set per engine, and the two differ.
// ffmpeg's whip muxer writes one H.264 track beside one Opus track and has no other payloader,
// where whipclientsink picks its payloader off the caps webrtcbin offers and so reaches VP8 and VP9
// over the same endpoint.
//
// AV1 is in no set.
// Its WHEP track negotiates and then yields no frame here, measured: neither an autoplugged nor an
// explicit rtpav1depay chain produces a picture, and a leg nothing reads back is not a leg.
//
// Opus is the whole audio set everywhere, because it is the whole audio set WebRTC negotiates.
// AAC has no SDP form there at all, which is the protocol's fact rather than either engine's.
//
// The watch side has no player entry, only the receiving pipeline's and the browser's.
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

// PublishArgs carries the credential as a muxer option rather than in the endpoint.
// The WHIP exchange is HTTP, where the relay reads a bearer token off a header (credential.go).
func (WebRTC) PublishArgs(s settings.Settings) []string {
	args := []string{"-f", "whip"}
	if token, ok := credentialToken(s); ok {
		args = append(args, "-authorization", token)
	}
	return append(args, whipURL(s, s.Relay.Path(s.Publish.Name)))
}

// GstSink names whipclientsink, muxer and sink in one as rtspclientsink is: it takes a parsed
// elementary stream per request pad, picks the pad template off the caps offered, and payloads each
// into its own track of the WebRTC session.
// It therefore carries GstMuxName, where the pipeline's audio branch attaches.
//
// Endpoint and credential are properties of the element's signaller object rather than of the
// element, so both go through the child-property syntax: the HTTP exchange carries them, not the
// media that follows.
func (WebRTC) GstSink(s settings.Settings) []string {
	sink := []string{
		"whipclientsink", "name=" + GstMuxName,
		"signaller::whip-endpoint=" + whipURL(s, s.Relay.Path(s.Publish.Name)),
	}
	if token, ok := credentialToken(s); ok {
		sink = append(sink, "signaller::auth-token="+token)
	}
	return sink
}

// GstSource names whepsrc, which runs the WHEP exchange and yields one RTP pad per negotiated
// track, the shape rtspsrc has, so the receiver's decodebin picks depayloader and decoder off the
// caps.
//
// audio-caps=EMPTY is what makes the exchange work at all.
// With an audio section in the offer, the element marks the video section bundle-only on port 0 and
// the relay answers 400 "codecs not supported by client".
// Dropping the audio proposal moves video into the first section, at the cost of this leg carrying
// video alone.
func (WebRTC) GstSource(s settings.Settings, streamName string) []string {
	assert.Assert(streamName != "", "a receive source names the stream it decodes")

	source := []string{
		"whepsrc",
		"whep-endpoint=" + whepURL(s, streamName),
		"audio-caps=EMPTY",
	}
	if token, ok := credentialToken(s); ok {
		source = append(source, "auth-token="+token)
	}
	return source
}

// BrowserURL is the relay's WHEP player page, running the same exchange whepsrc does from the
// browser's own RTCPeerConnection.
// It is served on the WebRTC listener, at the stream's path with the trailing slash the relay would
// otherwise redirect to.
func (WebRTC) BrowserURL(s settings.Settings, streamName string) string {
	assert.Assert(streamName != "", "a player page names the stream it opens")

	return webrtcOrigin(s) + "/" + streamName + "/" + credentialQuery(s, "?")
}

// Neither endpoint carries the credential in the address: both are HTTP, where the relay reads a
// token off a header the caller sets (credential.go).
func whipURL(s settings.Settings, name string) string {
	return webrtcOrigin(s) + "/" + name + "/whip"
}

func whepURL(s settings.Settings, name string) string {
	return webrtcOrigin(s) + "/" + name + "/whep"
}

// Where the signalling answers: the relay's listener, or the proxy in front of it.
// Only the exchange goes there; the media leg negotiates its own path and meets neither.
func webrtcOrigin(s settings.Settings) string {
	return s.Relay.HTTPOrigin(s.Relay.WebrtcPort)
}
