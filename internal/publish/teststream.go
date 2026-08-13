package publish

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// A test stream is one gst-launch-1.0 process encoding a videotestsrc pattern into the relay,
// so it is launched by the binary GstExe names and resolved by FindGstExe,
// exactly as a GStreamer publish is.
// It named its own executable before, which was the same string written twice and one of them free
// to drift.

// TestSurface is what one test stream draws, the colour it draws it in and what it carries beside
// the picture.
//
// The colour belongs to the row rather than to the set, because the set exists to be watched:
// a viewer's HDR path is not exercised by a grid of standard-range streams,
// and the two beside each other are what makes a tone-mapped tile comparable to one drawn as it
// arrives.
type TestSurface struct {
	// Pattern is the videotestsrc pattern, which is what tells simultaneous test streams apart on
	// screen.
	Pattern string
	// Format is the pixel layout the pattern is drawn into and Colorimetry the colour it is drawn in.
	// Both are stated rather than left to the source, so what a test stream carries is a property of
	// this table and not of the frame size videotestsrc picks a default from.
	Format      string
	Colorimetry string
	// Audio names the codec of the second track the row publishes, as the audio capability table
	// names it, and is empty on the rows that carry picture alone.
	//
	// It belongs to the row for the reason the colour does.
	// A viewer's audio path is the volume a tile carries, the level meter beside it and two streams
	// playing at once, and a set of silent streams reaches none of it.
	// One row and not all of them, so there is a silent tile to compare against and the grid does not
	// play everything it holds at once.
	Audio string
	// Label distinguishes a row whose number says nothing about what is worth knowing about it,
	// and is empty on the rows that need none.
	// It reaches the relay as part of the stream name, which is what a viewer picking one off the
	// roster reads before anything has decoded.
	//
	// It is a label on an expectation and never a claim about the stream: what a decode turned out to
	// carry is reported off the pipeline, so a row named for HDR whose tile draws no HDR badge is
	// exactly the failure worth seeing.
	Label string
}

// The colours the rows are drawn in.
// The standard-range one is BT.709 because these are 720p surfaces, and the HDR one is PQ because
// that is the curve mastered content carries and the one a viewer meets.
const (
	testSDR = "bt709"
	testHDR = "bt2100-pq"
)

// The pixel layouts.
// An HDR surface cannot ride in eight bits, which is the rule the publish path enforces on a real
// capture, so the HDR row is the ten-bit one.
const (
	testChroma8  = "I420"
	testChroma10 = "I420_10LE"
)

// testAudio is the codec the sounding row is coded in.
// Opus because every leg here carries it, WebRTC included, so that row is watchable over each of
// them rather than over the ones that carry AAC.
const testAudio = "opus"

// testAudioWave is what that row plays and testAudioVolume how loud.
//
// Noise rather than a tone or a tick: the level meter beside a tile is one of the things the row
// exists to exercise, and a meter wants a signal that is there continuously.
// A tick a second leaves it reading silence between ticks, and a sine at one frequency runs for as
// long as the backend does.
// A fifth of full scale arrives at about -30 dBFS, which is a level to leave playing.
const (
	testAudioWave   = "pink-noise"
	testAudioVolume = 0.2
)

// testSurfaces are handed out to the slots in order and repeated once the list runs out.
//
// The HDR row is second so that the set this process brings up with itself carries one:
// a viewer that has to ask for six streams before it can see an HDR tile would leave the path
// untested on every ordinary run.
//
// That row is High 10 H.264, which the native tile decodes and browsers do not.
// The rest of the set stays 4:2:0 for exactly that reason, so the relay's browser page is still
// served by five of the six rows.
//
// The sounding row is third and so inside that same set, for the same reason.
var testSurfaces = []TestSurface{
	{Pattern: "smpte", Format: testChroma8, Colorimetry: testSDR},
	{Pattern: "ball", Format: testChroma10, Colorimetry: testHDR, Label: "hdr"},
	{Pattern: "gradient", Format: testChroma8, Colorimetry: testSDR, Audio: testAudio, Label: "audio"},
	{Pattern: "pinwheel", Format: testChroma8, Colorimetry: testSDR},
	{Pattern: "spokes", Format: testChroma8, Colorimetry: testSDR},
	{Pattern: "circular", Format: testChroma8, Colorimetry: testSDR},
}

// TestSurfaceOf returns what the i-th test stream publishes.
func TestSurfaceOf(i int) TestSurface {
	// Go's remainder keeps the sign, so a negative index reaches the slice.
	assert.Assert(i >= 0, "a test stream is numbered from zero", i)
	return testSurfaces[i%len(testSurfaces)]
}

// BuildTestStreamArgs returns the gst-launch-1.0 arguments publishing one synthetic stream to the
// relay under name.
// The relay re-serves it on every listener, so all viewing paths (native grid, web grid,
// per-stream viewers) see it like a real stream.
// Publishing always goes over RTSP regardless of s.Transport, and the encode is H.264,
// which every path decodes at eight bits per component.
// timeoverlay makes motion and latency visible, and takes the ten-bit surface as it takes the
// eight-bit one.
// A surface naming an audio codec publishes a second track beside the picture, which the sink
// payloads into an RTP stream of its own inside the same session.
//
// The surface's colour reaches the stream because it is stated on the source caps and x264enc
// writes it into the VUI: a PQ surface measured through this argv decodes as bt2100-pq in High 10,
// which is what makes the HDR row a stream a viewer treats as HDR rather than a picture that merely
// looks bright.
func BuildTestStreamArgs(s settings.Settings, name string, surface TestSurface) ([]string, error) {
	assert.Assert(surface.Pattern != "", "a test stream draws a pattern", name)
	assert.Assert(surface.Format != "" && surface.Colorimetry != "",
		"a test stream states the surface it draws into", name, surface.Pattern)

	s.Publish.Transport = "rtsp"
	s.Publish.Name = name
	// The sink reads RTSP's own publish-leg settings, which this path takes from the caller like any
	// other publish: forcing the transport does not make the values it reads legal.
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
	// The sink waits on two pads once a track is attached, and the queue is what keeps one pad's
	// stall out of the other branch upstream of it.
	// It is the same queue a publish pipeline puts there, for the same reason (gstpipeline.go).
	if len(audio) > 0 {
		args = append(args, "queue", "!")
	}
	args = append(args, sink...)
	return append(args, audio...), nil
}

// testAudioBranch is the second track of a test stream that carries one, from the synthetic source
// to the sink's mux pad, and nil on a silent row.
//
// The encoder, the parser framing its output and the rate the capsfilter pins are read off the
// audio capability table, so a test stream is coded by the elements a publish is coded by rather
// than by a second set of names free to drift from them.
// The converter pair a publish branch carries is what this one does without: audiotestsrc produces
// the rate and the channel count asked of it, where a monitor runs at the rate its device runs at.
//
// A row naming a codec the table does not carry, or one no GStreamer element codes, is a broken
// table rather than a machine this cannot run on: the row and the table are both in this repo,
// and the missing element is what the launched child reports.
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
