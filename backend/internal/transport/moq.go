package transport

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
)

// MoQ is Media over QUIC, a watch-only leg here because no reader in this app opens a MoQ track.
// The relay ingests over it as readily as it serves it, on the publish page under the same path.
// The browser is the one reader, on the page the relay serves, which subscribes over WebTransport
// and decodes with WebCodecs.
//
// The other readers are missing an implementation rather than holding a narrow set.
// libavformat has no MoQ demuxer, and GStreamer's QUIC elements carry raw streams and RTP over QUIC
// rather than MoQ tracks, so neither a player nor a receive pipeline has anything to open it with.
// The moqsrc and moqsink that exist outside this build speak moq-lite and hang,
// carrying a catalog and a container of their own,
// where this relay answers MOQT draft 19 with a WARP catalog and LOC packaging.
//
// Buys the format set, every bitstream this app encodes reaching a page where HLS drops VP8 and
// WHEP drops HEVC and AV1.
// A native leg would buy per-subscriber layer dropping beyond that,
// stated under "Adaptive bitrate" in docs/plan.md.
// Costs a listener no reverse proxy carries, exposed and certificated on its own (moqOrigin,
// docs/network-architecture.md).
type MoQ struct{}

func init() {
	Register(MoQ{})
}

func (MoQ) Name() string { return "moq" }

// moqPlayback is the relay's own set, the formats its MoQ listener packages into tracks.
// The whole video table and the audio codecs this app offers, the relay carrying G711 and LPCM
// beside them and nothing here producing either.
//
// What a browser decodes out of a track is that browser's affair, for HLS's reason: WebCodecs
// support differs per build and per machine, and a narrower set here would refuse the page
// for a stream that would have played.
var moqPlayback = Carriage{
	Video: []string{"h264", "hevc", "av1", "vp9", "vp8"},
	Audio: []string{"opus", "aac"},
}

var moqFormats = Formats{Watch: map[string]Carriage{
	EngineBrowser: moqPlayback,
}}

func (MoQ) Formats() Formats { return moqFormats }

// BrowserURL is the relay's MoQ player page, "https://relay:8892/<stream>/".
// The trailing slash saves a redirect, that address being where the relay would send the browser,
// and the reader page is what answers there: "/publish" under the same path is the other one.
//
// The credential is the userinfo rather than a query, where the HTTP servers read one
// (credential.go).
func (MoQ) BrowserURL(s settings.Settings, path string) string {
	assert.Assert(path != "", "a player page names the path it opens")

	return moqOrigin(s) + "/" + path + "/"
}

// ListenerURL is the relay's MoQ listener, "https://relay:8892".
//
// Always https and always the port, where HTTPOrigin drops the port under Tls.
// WebTransport refuses a plaintext listener, so this leg is encrypted on a LAN relay too, and
// its session is a CONNECT over HTTP/3, which a proxy listening on TCP 443 never sees.
// The relay therefore answers this port itself in every deployment, the shape RTSPS and RTMPS have.
func (MoQ) ListenerURL(s settings.Settings) string {
	return fmt.Sprintf("https://%s:%d", s.Relay.Host, s.Relay.MoqPort)
}

// ProbeURL is the player page under the path a check dials, a route the MoQ server owns.
// Without the credential moqOrigin carries: a check reads a status and opens no track.
func (MoQ) ProbeURL(s settings.Settings) string {
	return MoQ{}.ListenerURL(s) + "/" + checkPath + "/"
}

// moqOrigin is that listener carrying the credential the page is answered on.
func moqOrigin(s settings.Settings) string {
	return withCredential(s, MoQ{}.ListenerURL(s))
}
