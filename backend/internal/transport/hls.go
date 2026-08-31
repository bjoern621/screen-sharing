package transport

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// HLS is a watch-only leg: the relay cuts an ingested stream into segments and serves them
// with a playlist over plain HTTP, and ingests no HLS itself.
// This transport therefore declares no publish form, and the publish dropdown never offers it.
//
// One HTTP port is what it buys, which proxies and firewalls pass and every browser plays.
// Delay is what it costs: nothing starts before a segment exists, and the relay's low-latency
// variant cuts the wait to part of a segment instead of removing it, leaving HLS an order
// of magnitude behind SRT and RTSP.
type HLS struct{}

func init() {
	Register(HLS{})
}

func (HLS) Name() string { return "hls" }

// hlsPlayback is the relay's own set, the segment formats its HLS muxer cuts: H.264 and H.265
// into MPEG-TS, AV1 and VP9 into fMP4, with Opus and AAC beside them.
// VP8 has no segment form there, so a VP8 stream is the one the playlist never carries.
//
// One value for the player and the page, and a publish entry for neither: the relay serves HLS and
// ingests none.
//
// What a browser decodes out of a segment is that browser's affair and no fact this table can hold.
// H.264 plays in all of them and H.265, AV1 and VP9 depend on the build and the machine, so
// a narrower set here would refuse the page for a stream that would have played.
var hlsPlayback = Carriage{
	Video: []string{"h264", "hevc", "av1", "vp9"},
	Audio: []string{"opus", "aac"},
}

// hlsPipeline is what a receiving pipeline reads back off those same segments: the relay's video
// set, no audio.
// The entry point narrows it rather than the decoder, a tile opening the video rendition alone
// (hlsplaylist.go).
var hlsPipeline = Carriage{
	Video: []string{"h264", "hevc", "av1", "vp9"},
}

var hlsFormats = Formats{Watch: map[string]Carriage{
	capabilities.EngineFfmpeg: hlsPlayback,
	capabilities.EngineGst:    hlsPipeline,
	EngineBrowser:             hlsPlayback,
}}

func (HLS) Formats() Formats { return hlsFormats }

// WatchURL is the master playlist, "http://relay:8888/<stream>/index.m3u8".
// Every reader follows the relay's redirect to the variant playlist by itself.
//
// The stream name is asserted and not checked: an unnamed stream is refused by every watch effect
// at the control boundary, so an empty one arriving here is a caller that skipped that boundary.
func (HLS) WatchURL(s settings.Settings, streamName string) string {
	assert.Assert(streamName != "", "a watch URL names the stream it opens")

	// No credential in the address.
	// A player opens this URL and the relay's HTTP servers take a token in a header (credential.go),
	// handed to the player beside the URL (internal/watch).
	return HLS{}.ListenerURL(s) + "/" + streamName + "/index.m3u8"
}

// ListenerURL is where the relay serves HLS: its own port, or the proxy's name where one fronts it
// (settings.Relay.HTTPOrigin).
func (HLS) ListenerURL(s settings.Settings) string {
	return s.Relay.HTTPOrigin(s.Relay.HlsPort)
}

// ResolveGstSource asks the relay where the segments are (hlsplaylist.go).
//
// urisourcebin rather than hlsdemux2 straight: the demuxer refuses to build outside a streams-aware
// parent, and what leaves the bin is the encoded stream every other source fragment ends at.
func (HLS) ResolveGstSource(s settings.Settings, streamName string) ([]string, error) {
	assert.Assert(streamName != "", "a receive source names the stream it decodes")

	media, err := hlsMediaSource(s, HLS{}.WatchURL(s, streamName))
	if err != nil {
		return nil, fmt.Errorf("no hls source for %q: %w", streamName, err)
	}
	return []string{"urisourcebin", "uri=" + media}, nil
}

// BrowserURL is the relay's player page, "http://relay:8888/<stream>/": the same path with no
// playlist name, the page fetching the playlist itself.
// The trailing slash saves a redirect, that address being where the relay would send the browser.
//
// The credential is the address's userinfo, the one form a browser carries (credential.go).
func (HLS) BrowserURL(s settings.Settings, streamName string) string {
	assert.Assert(streamName != "", "a player page names the stream it opens")

	return httpPageOrigin(s, s.Relay.HlsPort) + "/" + streamName + "/"
}
