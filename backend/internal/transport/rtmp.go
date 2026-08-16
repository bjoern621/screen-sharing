package transport

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// RTMP streams through the relay's RTMP listener, which is the leg broadcast tools already speak.
// Interop rather than low delay: FLV over one TCP connection, with no retransmit window and no
// jitter buffer, so it declares no watch knobs.
//
// ffmpeg's flv muxer writes the enhanced-RTMP codec tags the relay ingests, which takes the publish
// leg past FLV's original H.264.
// flvmux writes the legacy tags alone, so this transport declares no GStreamer publish form and a
// capture backend on that engine is told in the settings form rather than by a muxer refusing the
// pipeline.
type RTMP struct{}

func init() {
	Register(RTMP{})
}

func (RTMP) Name() string { return "rtmp" }

// Formats states the publish leg on the ffmpeg engine alone, as what the relay's enhanced-RTMP
// ingest takes from the flv muxer.
// VP8 is absent for want of a tag in that muxer, and AAC is the whole audio set, being the tag FLV
// has always carried.
//
// Both watch entries read the legacy tags and no more, the players through libavformat's FLV
// demuxer and the grid through rtmp2src.
// Two entries carrying the same list rather than one shared value, because nothing moves them
// together.
var rtmpFormats = Formats{
	Publish: map[string]Carriage{capabilities.EngineFfmpeg: {
		Video: []string{"h264", "hevc", "av1", "vp9"},
		Audio: []string{"aac"},
	}},
	Watch: map[string]Carriage{
		capabilities.EngineFfmpeg: {Video: []string{"h264"}, Audio: []string{"aac"}},
		capabilities.EngineGst:    {Video: []string{"h264"}, Audio: []string{"aac"}},
	},
}

func (RTMP) Formats() Formats { return rtmpFormats }

// PublishArgs muxes to FLV, the container RTMP carries, and names the stream by URL path.
func (RTMP) PublishArgs(s settings.Settings) []string {
	return []string{"-f", "flv", rtmpURL(s, s.Relay.Path(s.Publish.Name))}
}

func (RTMP) WatchURL(s settings.Settings, streamName string) string {
	assert.Assert(streamName != "", "a watch URL names the stream it opens")

	return rtmpURL(s, streamName)
}

// GstSource is one element: rtmp2src yields the FLV stream that decodebin then demuxes and decodes.
func (RTMP) GstSource(s settings.Settings, streamName string) []string {
	assert.Assert(streamName != "", "a receive source names the stream it decodes")

	return []string{"rtmp2src", "location=" + rtmpURL(s, streamName)}
}

// RtmpsPort is where a relay's encrypted RTMP listener answers: MediaMTX's own default, and no
// setting, for the reason RtspsPort is none.
const RtmpsPort = 1936

// rtmpURL addresses one path on the relay's RTMP listener,
// "rtmps://relay:1936/<path>?jwt=<token>" where the relay is encrypted and
// "rtmp://relay:1935/<path>?jwt=<token>" where it is not.
//
// Neither the name nor the port is asserted, unlike at the watch entry points above: this builder
// serves the publish leg too, where the name comes off the settings rather than a validated call
// and a port of zero is a stored value the migration repairs.
func rtmpURL(s settings.Settings, name string) string {
	scheme, port := "rtmp", s.Relay.RtmpPort
	if s.Relay.Tls() {
		scheme, port = "rtmps", RtmpsPort
	}
	return fmt.Sprintf("%s://%s:%d/%s", scheme, s.Relay.Host, port, name) + credentialQuery(s, "?")
}
