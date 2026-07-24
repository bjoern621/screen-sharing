package publish

import (
	"fmt"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// TestStreamExe launches one synthetic test publisher: a gst-launch-1.0
// process encodes a videotestsrc pattern and pushes it to the relay.
const TestStreamExe = "gst-launch-1.0"

// testPatterns are the videotestsrc patterns handed out round-robin,
// so simultaneous test streams are visually distinct.
var testPatterns = []string{"smpte", "ball", "gradient", "pinwheel", "spokes", "circular"}

// TestPattern returns the videotestsrc pattern of the i-th test stream.
func TestPattern(i int) string {
	return testPatterns[i%len(testPatterns)]
}

// BuildTestStreamArgs returns the gst-launch-1.0 arguments publishing one
// synthetic stream to the relay under name.
// The relay re-serves it on every listener, so all viewing paths (native grid,
// WHEP grid, per-stream viewers) see it like a real stream.
// Publishing always goes over RTSP regardless of s.Transport, and the encode
// is plain 4:2:0 H.264, which every path decodes, including browsers.
// timeoverlay makes motion and latency visible.
func BuildTestStreamArgs(s settings.Stream, name string, pattern string) ([]string, error) {
	s.Transport = "rtsp"
	s.Name = name
	sink, ok := transport.GstSink(s)
	if !ok {
		return nil, fmt.Errorf("the rtsp transport has no GStreamer sink")
	}

	args := []string{
		"videotestsrc", "is-live=true", "pattern=" + pattern,
		"!", "video/x-raw,format=I420,width=1280,height=720,framerate=30/1",
		"!", "timeoverlay", "valignment=bottom", "halignment=right",
		"!", "x264enc", "bitrate=3000", "pass=cbr", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=60",
		"!", "h264parse", "config-interval=-1",
		"!",
	}
	return append(args, sink...), nil
}
