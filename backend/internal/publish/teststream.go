package publish

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// One gst-launch-1.0 process per test stream, encoding a videotestsrc pattern into the relay,
// launched from the binary GstExe names and located through FindGstExe like any GStreamer publish.

// TestSurface is one row of what the test streams draw: the picture, the colour it is drawn in, and
// whatever travels beside it.
//
// The colour sits on the row and not on the set, the set being built to be watched.
// Standard range throughout leaves a viewer's HDR path unexercised, and a tone-mapped tile is only
// readable against one drawn as it arrived.
type TestSurface struct {
	// Pattern is the videotestsrc pattern, and what tells simultaneous test streams apart
	// on screen.
	Pattern string
	// Format is the pixel layout drawn into, Colorimetry the colour drawn in.
	// Stating both keeps the carriage a property of this table, where leaving either to the source
	// would hang it off the frame size videotestsrc defaults from.
	Format      string
	Colorimetry string
	// Audio is the second track's codec, spelled as the audio capability table spells it, and empty
	// where the row carries picture alone.
	//
	// On the row for the colour's reason.
	// A tile's volume, the meter beside it and two streams sounding at once are the viewer's audio
	// path, and silence throughout reaches none of them.
	// One row rather than every row, which leaves a silent tile to compare against and stops the grid
	// playing all of itself at once.
	Audio string
	// Label carries what a row's number cannot, and is empty where the number suffices.
	// It travels in the stream name, so it is what a viewer picking off the roster reads before
	// anything has decoded.
	//
	// The label is an expectation the stream is measured against:
	// a decode's own findings are reported off the pipeline,
	// so an HDR-labelled row whose tile draws no HDR badge is the failure this makes visible.
	Label string
}

// The colours drawn in.
// BT.709 for standard range, these being 720p surfaces, and PQ for HDR, that being the curve
// mastered content carries and a viewer meets.
const (
	testSDR = "bt709"
	testHDR = "bt2100-pq"
)

// The pixel layouts drawn into.
// Eight bits cannot carry an HDR surface, the same rule the publish path holds a real capture to,
// so ten bits go with the HDR row.
const (
	testChroma8  = "I420"
	testChroma10 = "I420_10LE"
)

// testAudio codes the sounding row.
// Opus, every leg here carrying it, WebRTC included, which leaves that row watchable over all
// of them instead of over the AAC-carrying ones alone.
const testAudio = "opus"

// testAudioWave is what the sounding row plays, testAudioVolume how loud.
//
// Noise rather than a tone or a tick, the meter beside a tile being one of the things this row
// is for: a meter needs a signal that does not stop.
// Between ticks it would read silence, and one sine frequency plays for as long as the backend
// lives.
// A fifth of full scale lands near -30 dBFS, quiet enough to leave running.
const (
	testAudioWave   = "pink-noise"
	testAudioVolume = 0.2
)

// testSurfaces go to the slots in order, the list repeating past its end.
//
// Second place puts the HDR row inside the set this process brings up by itself.
// A viewer having to ask for the whole list before an HDR tile appeared would leave that path
// untried on an ordinary run.
//
// High 10 H.264: the native tile decodes it and browsers do not, so every other row holds 4:2:0 and
// keeps the relay's browser page served.
//
// Third place puts the sounding row in that same starting set, on the same grounds.
var testSurfaces = []TestSurface{
	{Pattern: "smpte", Format: testChroma8, Colorimetry: testSDR},
	{Pattern: "ball", Format: testChroma10, Colorimetry: testHDR, Label: "hdr"},
	{Pattern: "gradient", Format: testChroma8, Colorimetry: testSDR, Audio: testAudio, Label: "audio"},
	{Pattern: "pinwheel", Format: testChroma8, Colorimetry: testSDR},
	{Pattern: "spokes", Format: testChroma8, Colorimetry: testSDR},
	{Pattern: "circular", Format: testChroma8, Colorimetry: testSDR},
}

// TestSurfaceOf is the i-th test stream's surface, the rows repeating past the end of the list.
func TestSurfaceOf(i int) TestSurface {
	// Go's remainder keeps the sign, so a negative i indexes out of range rather than wrapping.
	assert.Assert(i >= 0, "a test stream is numbered from zero", i)
	return testSurfaces[i%len(testSurfaces)]
}

// BuildTestStreamArgs is the gst-launch-1.0 argv that publishes one synthetic stream to the relay
// under name.
// Re-served on every listener, so the native grid, the web grid and a per-stream viewer meet it
// on the terms they meet a real stream on.
// The leg is RTSP whatever s.Transport holds, and the encode H.264, decoded at eight bits per
// component everywhere.
// timeoverlay shows motion and latency, and takes a ten-bit surface as readily as an eight-bit one.
// Naming an audio codec adds a second track beside the picture, payloaded by the sink into an RTP
// stream of its own within the same session.
//
// Source caps state the surface's colour and x264enc copies it into the VUI, which is how it
// survives to the stream: measured through this argv, a PQ surface decodes as bt2100-pq in High 10.
// That is the difference between a stream a viewer treats as HDR and a picture that merely looks
// bright.
func BuildTestStreamArgs(s settings.Settings, name string, surface TestSurface) ([]string, error) {
	assert.Assert(surface.Pattern != "", "a test stream draws a pattern", name)
	assert.Assert(surface.Format != "" && surface.Colorimetry != "",
		"a test stream states the surface it draws into", name, surface.Pattern)

	s.Publish.Transport = "rtsp"
	s = s.WithStreamName(name)
	// RTSP's publish-leg settings are the sink's input and arrive from the caller as they do on any
	// other publish: pinning the transport does not vouch for the values read under it.
	if err := transport.ValidatePublishSettings(s); err != nil {
		return nil, err
	}
	sink, ok := transport.GstSink(s)
	if !ok {
		return nil, fmt.Errorf("the rtsp transport has no GStreamer sink")
	}

	args := []string{
		"videotestsrc", "is-live=true", "pattern=" + surface.Pattern,
		"!", "video/x-raw,format=" + surface.Format + ",width=1280,height=720,framerate=30/1" +
			",colorimetry=" + surface.Colorimetry,
		"!", "timeoverlay", "valignment=bottom", "halignment=right",
		"!", "x264enc", "bitrate=3000", "pass=cbr", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=60",
		"!", "h264parse", "config-interval=-1",
		"!",
	}
	audio := testAudioBranch(surface)
	// An attached track leaves the sink waiting on two pads, and the queue stops one pad's stall
	// reaching back up the other branch.
	// A publish pipeline places the same queue on the same grounds (gstpipeline.go).
	if len(audio) > 0 {
		args = append(args, "queue", "!")
	}
	args = append(args, sink...)
	return append(args, audio...), nil
}

// testAudioBranch runs a sounding row's track from the synthetic source to the sink's mux pad, and
// is nil for a silent one.
//
// Encoder, the parser framing its output, and the rate pinned in the capsfilter all come off
// the audio capability table, which codes a test stream through the elements a publish codes
// through instead of a second set of names free to drift.
// What a publish branch carries and this one drops is the converter pair: audiotestsrc yields
// the rate and channel count it is asked for, where a monitor yields whatever its device runs at.
//
// A codec the table does not carry, or one no GStreamer element codes, is a broken table and not
// an unequipped machine: both row and table live in this repo, and a missing element surfaces
// from the launched child.
func testAudioBranch(surface TestSurface) []string {
	if surface.Audio == "" {
		return nil
	}

	a, ok := capabilities.GetAudio(surface.Audio)
	assert.Assert(ok, "a test surface names an audio codec the table carries", surface.Audio)
	enc, ok := a.EncoderOn(EngineGst)
	assert.Assert(ok, "a test stream's audio codec has a GStreamer encoder", a.Name)
	assert.Assert(enc.Parser != "", "a GStreamer audio encoder states its parser", a.Name)

	return []string{
		"audiotestsrc", "is-live=true", "wave=" + testAudioWave, fmt.Sprintf("volume=%.2f", testAudioVolume),
		"!", fmt.Sprintf("audio/x-raw,rate=%d,channels=2", a.Rate),
		"!", enc.Element, fmt.Sprintf("bitrate=%d", a.BitrateK*1000),
		"!", enc.Parser,
		"!", transport.GstMuxName + ".",
	}
}
