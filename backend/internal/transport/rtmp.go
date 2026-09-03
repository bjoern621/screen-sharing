package transport

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// RTMP streams through the relay's RTMP listener, the leg broadcast tools already speak.
// Interop rather than low delay: FLV over one TCP connection, with no retransmit window and no
// jitter buffer, so it declares no watch knobs.
//
// ffmpeg's flv muxer writes the enhanced-RTMP codec tags the relay ingests, taking the publish leg
// past FLV's original H.264.
// flvmux writes the legacy tags alone, so this transport declares no GStreamer publish form and
// a capture backend on that engine is told in the settings form rather than by a muxer refusing
// the pipeline.
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
// Two entries carrying the same list rather than one shared value, nothing moving them together.
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
	args := append([]string{"-f", "flv"}, ffmpegTlsVerify(s)...)
	return append(args, rtmpURL(s, s.PublishPath()))
}

func (RTMP) WatchURL(s settings.Settings, streamName string) string {
	assert.Assert(streamName != "", "a watch URL names the stream it opens")

	return rtmpURL(s, streamName)
}

// GstSource is one element: rtmp2src yields the FLV stream that decodebin then demuxes and decodes.
func (RTMP) GstSource(s settings.Settings, streamName string) []string {
	assert.Assert(streamName != "", "a receive source names the stream it decodes")

	return []string{"rtmp2src", "location=" + rtmpURL(s, streamName), gstTlsValidation(s)}
}

// ListenerURL is the relay's RTMPS listener, "rtmps://relay:1936".
//
// One scheme, for the reason RTSP's listener has one: every relay terminates TLS on this leg and
// binds no cleartext listener (deploy/mediamtx-groups.yml, rtmpEncryption).
func (RTMP) ListenerURL(s settings.Settings) string {
	return fmt.Sprintf("rtmps://%s:%d", s.Relay.Host, s.Relay.RtmpPort)
}

// rtmpURL addresses one path on that listener, "rtmps://relay:1936/<path>?jwt=<token>".
//
// Neither the name nor the port is asserted, unlike at the watch entry points: this builder serves
// the publish leg too, where the name comes off the settings rather than a validated call and
// a port of zero is a stored value the migration repairs.
func rtmpURL(s settings.Settings, name string) string {
	return RTMP{}.ListenerURL(s) + "/" + name + credentialQuery(s, "?")
}
