package transport

import (
	"fmt"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// RTMP streams through the relay's RTMP listener, the protocol broadcast tools
// speak. It is the interop leg rather than the low-delay one: FLV over a single
// TCP connection, with neither a retransmit window nor a jitter buffer to tune,
// so nothing about it is per-stream tunable and it declares no watch knobs.
//
// ffmpeg's flv muxer writes the enhanced-RTMP codec tags the relay ingests, so
// the publish leg reaches past FLV's original H.264. GStreamer has no
// counterpart: flvmux writes the legacy tags alone, so this transport declares
// no GStreamer publish form, and a capture backend on that engine says so in the
// settings form instead of building a pipeline the muxer would reject.
type RTMP struct{}

func init() {
	Register(RTMP{})
}

func (RTMP) Name() string { return "rtmp" }

// Formats: the publish leg is the ffmpeg engine's alone, and its set is what the
// relay's enhanced-RTMP ingest takes from the flv muxer. VP8 is absent because
// that muxer has no tag for it, and AAC is the whole audio set because the tag
// FLV has always carried is the one both ends agree on.
//
// Both watch entries read the legacy tags and no more, so each states H.264 and
// AAC: the players sit on libavformat's FLV demuxer and the grid on rtmp2src.
// They are two entries carrying the same list rather than one list standing for
// two viewers, because nothing makes them move together.
func (RTMP) Formats() Formats {
	return Formats{
		Publish: map[string]Carriage{capabilities.EngineFfmpeg: {
			Video: []string{"h264", "hevc", "av1", "vp9"},
			Audio: []string{"aac"},
		}},
		Watch: map[string]Carriage{
			capabilities.EngineFfmpeg: {Video: []string{"h264"}, Audio: []string{"aac"}},
			capabilities.EngineGst:    {Video: []string{"h264"}, Audio: []string{"aac"}},
		},
	}
}

// PublishArgs returns the ffmpeg output args for this transport. The flv muxer
// is the FLV container RTMP carries, and the URL names the stream by path.
func (RTMP) PublishArgs(s settings.Stream) []string {
	return []string{"-f", "flv", rtmpURL(s, s.Name)}
}

func (RTMP) WatchURL(s settings.Stream, streamName string) string {
	return rtmpURL(s, streamName)
}

// GstSource returns the source element a receiving GStreamer pipeline decodes
// from. rtmp2src is source and demuxer in one for the pipeline's purposes: it
// yields the FLV stream that decodebin demuxes and decodes.
func (RTMP) GstSource(s settings.Stream, streamName string) []string {
	return []string{"rtmp2src", "location=" + rtmpURL(s, streamName)}
}

func rtmpURL(s settings.Stream, name string) string {
	return fmt.Sprintf("rtmp://%s:%d/%s", s.RelayHost, s.RtmpPort, name)
}
