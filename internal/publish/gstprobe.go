package publish

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// gstProbeSource is the layout the generated frames enter the chain in.
// Every screen capture backend this engine drives hands the pipeline packed 8-bit RGB, so the
// conversion into the encoder's planar layout is work a publish pays and the probe has to pay with
// it.
const gstProbeSource = "BGRx"

// gstProbeHeavy and gstProbeLight are the videotestsrc patterns the two ends of the measured range
// are taken on, and the whole of what separates them.
//
// Picked for what they cost an encoder rather than for what they look like.
// snow is uncorrelated noise: nothing predicts from the frame before it and nothing is redundant
// within a frame, which is the most work any content can ask for.
// ball is a moving object over a flat field, where almost every block predicts from its neighbour
// and codes to nothing.
//
// A screen sits between them and moves as its content changes, text and photographs toward the
// noise end and an idle desktop toward the ball, which is why both ends are reported and not an
// average.
const (
	gstProbeHeavy = "snow"
	gstProbeLight = "ball"
)

// gstProbeQueue decouples frame generation from encoding.
//
// On one thread the reading is their sum, which prices the generator into a figure meant to be the
// encoder's: against a lossless 4:4:4 encode, serialising the noise pattern read a third below the
// rate the encoder sustained.
// The bound is what the generator may run ahead by, counted in buffers because a byte or time bound
// would cap it wherever the picture size or the frame rate decides.
var gstProbeQueue = []string{
	"queue", "max-size-buffers=8", "max-size-bytes=0", "max-size-time=0",
}

// GstEncodeProbe is a gst-launch pipeline encoding generated frames with this stream's encoder and
// throwing the coded bytes away, so the rate this machine sustains is timed without a screen to
// capture or a relay to send to.
//
// Everything between source and sink is what a publish run builds: the same conversion into the
// same encoder-input caps, then the same encoder element with the rate-control properties the
// settings map onto it.
// Both ends differ away from the encoder, videotestsrc generating frames where a capture backend
// would acquire them and fakesink dropping bytes where a transport sink would payload and send
// them.
//
// heavy picks which end of the content range the frames are generated at (gstProbeHeavy,
// gstProbeLight), frames is how many are encoded, and the caller times the run.
//
// The transport is not checked, unlike every other pipeline this package builds: refusing a
// measurement because the codec cannot reach the relay over the configured leg would answer a
// question about this CPU with a fact about a protocol.
func GstEncodeProbe(s settings.Settings, width, height, frames int, heavy bool) ([]string, error) {
	assert.Assert(width > 0 && height > 0, "a probe encodes frames of a resolved picture size", width, height)
	assert.Assert(frames > 0, "a probe encodes at least one frame", frames)

	if err := capabilities.Validate(EngineGst, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM); err != nil {
		return nil, err
	}
	if s.Publish.Fps <= 0 {
		return nil, fmt.Errorf("the GStreamer publish engine needs a positive fps, got %d", s.Publish.Fps)
	}
	mem, err := gstMemory(s)
	if err != nil {
		return nil, err
	}
	inCaps, err := gstEncoderCaps(s, mem)
	if err != nil {
		return nil, err
	}
	encoder, link, err := gstEncoder(s, gstGop(s), mem.memory)
	if err != nil {
		return nil, err
	}
	assert.Assert(len(encoder) > 0, "a mapped codec yields an encoder", s.Publish.Codec)

	pipeline := gstProbeFrames(s, width, height, frames, heavy)
	// The generator produces system memory whatever a capture would have produced, so a device path's
	// converter is reached through the upload its family names.
	// One memory move ahead of the conversion is the least that puts a generated frame where a
	// captured one already is, and what the measurement keeps is the encoder and the caps it reads.
	if mem.upload != "" {
		pipeline = append(pipeline, "!", mem.upload)
	}
	pipeline = append(pipeline, "!")
	pipeline = append(pipeline, mem.convert...)
	pipeline = append(pipeline, "!", inCaps, "!")
	pipeline = append(pipeline, encoder...)
	// The link elements stay in: a parser repeating parameter sets per keyframe reads every frame the
	// encoder writes, so it is part of what a run costs.
	if len(link) > 0 {
		pipeline = append(pipeline, "!")
		pipeline = append(pipeline, link...)
	}
	return append(pipeline, "!", "fakesink", "sync=false"), nil
}

// GstProbeCeiling runs the same generated frames into a sink and nothing else, so the rate the
// generator alone reaches can be timed.
//
// It is what tells a measurement apart from its own instrument: an encoder faster than videotestsrc
// can fill a queue for is timed against the generator, and a probe that lands at its ceiling
// measured the source.
func GstProbeCeiling(s settings.Settings, width, height, frames int, heavy bool) []string {
	assert.Assert(width > 0 && height > 0, "a probe encodes frames of a resolved picture size", width, height)
	assert.Assert(frames > 0, "a probe encodes at least one frame", frames)

	return append(gstProbeFrames(s, width, height, frames, heavy), "!", "fakesink", "sync=false")
}

// gstProbeFrames is the generating half both probes share: the source, the raw caps it produces
// into, and the queue putting it on a thread of its own.
func gstProbeFrames(s settings.Settings, width, height, frames int, heavy bool) []string {
	pattern := gstProbeLight
	if heavy {
		pattern = gstProbeHeavy
	}
	src := []string{
		"videotestsrc",
		"num-buffers=" + strconv.Itoa(frames),
		"pattern=" + pattern,
		// The pattern advances by frame count rather than by clock, so two runs encode the same content
		// however fast the machine reached each frame.
		"animation-mode=frames",
		"!", fmt.Sprintf("video/x-raw,format=%s,width=%d,height=%d,framerate=%d/1",
			gstProbeSource, width, height, s.Publish.Fps),
		"!",
	}
	return append(src, gstProbeQueue...)
}
