package publish

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// gstProbeSource is the raw layout the probe's generated frames enter the chain
// in. Every screen capture backend this engine drives hands the pipeline packed
// 8-bit RGB, so the conversion into the encoder's planar layout is work a publish
// run pays and work the probe has to pay with it.
const gstProbeSource = "BGRx"

// gstProbeHeavy and gstProbeLight are the videotestsrc patterns the two ends of the
// measured range are taken on, and the whole of what separates them.
//
// The two are picked for what they cost an encoder rather than for what they look
// like. snow is uncorrelated noise: nothing predicts from the frame before it and
// nothing is redundant within a frame, which is the most work any content can ask
// for. ball is a moving object over a flat field: almost every block predicts from
// its neighbour and codes to nothing.
//
// A screen sits between them and moves within them as its content changes, which is
// why the probe reports both and not an average of the two. Text and photographs
// push toward the noise end, an idle desktop toward the ball.
const (
	gstProbeHeavy = "snow"
	gstProbeLight = "ball"
)

// gstProbeQueue decouples frame generation from encoding.
//
// Without it both run on one thread and the measurement is their sum, which prices
// the generator into a figure that is supposed to be the encoder's: measured against
// a lossless 4:4:4 encode, serialising the noise pattern took the reading a third
// below the rate the encoder actually sustained. The buffer bound is what the
// generator may run ahead by, and it is counted in buffers because the byte and time
// bounds would each cap it somewhere the picture size or the frame rate decides.
var gstProbeQueue = []string{
	"queue", "max-size-buffers=8", "max-size-bytes=0", "max-size-time=0",
}

// GstEncodeProbe returns a gst-launch pipeline that encodes generated frames with
// this stream's encoder and throws the coded bytes away, so the rate this machine
// sustains can be timed without a screen to capture or a relay to send to.
//
// Everything between the source and the sink is what a publish run builds: the same
// conversion into the same encoder-input caps, then the same encoder element with
// the rate-control properties the settings map onto it. Only the two ends differ,
// and both differ away from the encoder: videotestsrc generates frames where a
// capture backend would acquire them, and fakesink drops the coded bytes where a
// transport sink would payload and send them.
//
// heavy picks the end of the content range the frames are generated at
// (gstProbeHeavy, gstProbeLight). frames is how many are encoded, and the caller
// times the run.
//
// The transport is not checked, unlike every other pipeline this package builds. A
// probe measures what the machine does to frames, and refusing that measurement
// because the codec cannot reach the relay over the configured leg would answer a
// question about this CPU with a fact about a protocol.
func GstEncodeProbe(s settings.Stream, width, height, frames int, heavy bool) ([]string, error) {
	assert.Assert(width > 0 && height > 0, "a probe encodes frames of a resolved picture size", width, height)
	assert.Assert(frames > 0, "a probe encodes at least one frame", frames)

	if err := capabilities.Validate(EngineGst, s.Codec, s.CapabilityOptions(), s.Cq, s.BitrateM); err != nil {
		return nil, err
	}
	if s.Fps <= 0 {
		return nil, fmt.Errorf("the GStreamer publish engine needs a positive fps, got %d", s.Fps)
	}
	mem, err := gstMemory(s)
	if err != nil {
		return nil, err
	}
	inCaps, err := gstEncoderCaps(s, mem)
	if err != nil {
		return nil, err
	}
	encoder, link, err := gstEncoder(s, gstGop(s))
	if err != nil {
		return nil, err
	}
	assert.Assert(len(encoder) > 0, "a mapped codec yields an encoder", s.Codec)

	pipeline := append(gstProbeFrames(s, width, height, frames, heavy), "!", mem.convert, "!", inCaps, "!")
	pipeline = append(pipeline, encoder...)
	// The link elements stay in: they are what a run puts between encoder and sink,
	// and a parser that repeats parameter sets per keyframe reads every frame the
	// encoder writes.
	if len(link) > 0 {
		pipeline = append(pipeline, "!")
		pipeline = append(pipeline, link...)
	}
	return append(pipeline, "!", "fakesink", "sync=false"), nil
}

// GstProbeCeiling returns the same generated frames running into a sink and nothing
// else, so the rate the generator alone reaches can be timed.
//
// It is what tells a measurement apart from its own instrument. An encoder faster
// than videotestsrc can fill a queue for is timed against the generator rather than
// against itself, and the two readings together are what says so: a probe that lands
// at its ceiling measured the source.
func GstProbeCeiling(s settings.Stream, width, height, frames int, heavy bool) []string {
	assert.Assert(width > 0 && height > 0, "a probe encodes frames of a resolved picture size", width, height)
	assert.Assert(frames > 0, "a probe encodes at least one frame", frames)

	return append(gstProbeFrames(s, width, height, frames, heavy), "!", "fakesink", "sync=false")
}

// gstProbeFrames is the generating half both probes share: the source, the raw caps
// it produces into, and the queue that puts it on a thread of its own.
func gstProbeFrames(s settings.Stream, width, height, frames int, heavy bool) []string {
	pattern := gstProbeLight
	if heavy {
		pattern = gstProbeHeavy
	}
	src := []string{
		"videotestsrc",
		"num-buffers=" + strconv.Itoa(frames),
		"pattern=" + pattern,
		// The ball moves by frame count rather than by clock, so a given frame holds
		// the same picture however fast the machine reached it and two runs encode the
		// same content.
		"animation-mode=frames",
		"!", fmt.Sprintf("video/x-raw,format=%s,width=%d,height=%d,framerate=%d/1",
			gstProbeSource, width, height, s.Fps),
		"!",
	}
	return append(src, gstProbeQueue...)
}
