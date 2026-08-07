package publish

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// A test stream is one gst-launch-1.0 process encoding a videotestsrc pattern
// into the relay, so it is launched by the binary GstExe names and resolved by
// FindGstExe, exactly as a GStreamer publish is. It named its own executable
// before, which was the same string written twice and one of them free to drift.

// testPatterns are the videotestsrc patterns handed out round-robin,
// so simultaneous test streams are visually distinct.
var testPatterns = []string{"smpte", "ball", "gradient", "pinwheel", "spokes", "circular"}

// TestPattern returns the videotestsrc pattern of the i-th test stream.
func TestPattern(i int) string {
	// Go's remainder keeps the sign, so a negative index reaches the slice.
	assert.Assert(i >= 0, "a test stream is numbered from zero", i)
	return testPatterns[i%len(testPatterns)]
}

// BuildTestStreamArgs returns the gst-launch-1.0 arguments publishing one
// synthetic stream to the relay under name.
// The relay re-serves it on every listener, so all viewing paths (native grid,
// web grid, per-stream viewers) see it like a real stream.
// Publishing always goes over RTSP regardless of s.Transport, and the encode
// is plain 4:2:0 H.264, which every path decodes, including browsers.
// timeoverlay makes motion and latency visible.
func BuildTestStreamArgs(s settings.Stream, name string, pattern string) ([]string, error) {
	s.Transport = "rtsp"
	s.Name = name
	// The sink reads RTSP's own publish-leg settings, which this path takes from
	// the caller like any other publish: forcing the transport does not make the
	// values it reads legal.
	if err := transport.ValidatePublishSettings(s); err != nil {
		return nil, err
	}
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
