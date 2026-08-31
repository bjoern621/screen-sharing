package gstrun

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/screenshare/internal/framestamp"
	"bjoernblessin.de/screenshare/internal/padprobe"
)

// stamped is one codec's whole round trip: what encodes it, what frames it, what it is carried
// under and what decodes it again.
// A row per codec that carries a stamp, so a codec joining internal/framestamp joins this check
// with it.
var stamped = []struct {
	name    string
	encoder string
	parser  string
	decoder string
	caps    string
}{
	{
		name:    "h264",
		encoder: "x264enc",
		parser:  "h264parse",
		decoder: "avdec_h264",
		caps:    "video/x-h264,stream-format=byte-stream,alignment=au",
	},
	{
		name:    "h265",
		encoder: "x265enc",
		parser:  "h265parse",
		decoder: "avdec_h265",
		caps:    "video/x-h265,stream-format=byte-stream,alignment=au",
	},
}

// stampFrames writes a unit a real encoder's stream carries through a parser and a decoder:
// the stamp is read back on the decoder's input, and the decoder goes on producing pictures.
//
// The one thing no unit test can state.
// Whether a decoder skips an unregistered message is the decoder's answer, and a unit this got
// wrong is a stream that plays nowhere.
func TestStampSurvivesParseAndDecode(t *testing.T) {
	gst.Init()

	for _, c := range stamped {
		t.Run(c.name, func(t *testing.T) {
			for _, factory := range []string{"videotestsrc", c.encoder, c.parser, c.decoder} {
				if gst.ElementFactoryFind(factory) == nil {
					t.Skipf("this GStreamer registers no %s", factory)
				}
			}

			const frames = 20
			// key-int-max keeps the stream to one parameter set per few frames, and zerolatency keeps
			// the encoder from holding pictures back: both shorten the run rather than change what
			// is measured.
			pipeline := parsePipeline(t, "videotestsrc num-buffers="+itoa(frames)+" ! "+
				"video/x-raw,width=320,height=240,framerate=30/1 ! "+
				c.encoder+" tune=zerolatency key-int-max=10 ! "+c.parser+" ! identity name=stats ! "+
				c.caps+" ! "+c.parser+" ! "+c.decoder+" name=decoder ! fakesink name=drawn sync=false")

			stampFrames(pipeline, "stats", watchDelay(pipeline, "stats"), &linkWindow{})

			var found, arrived, decoded atomic.Uint64
			countStamps(t, pipeline, "decoder", &found, &arrived)
			countBuffers(t, pipeline, "drawn", &decoded)

			play(t, pipeline)

			if arrived.Load() == 0 {
				t.Fatal("no encoded frame reached the decoder")
			}
			if decoded.Load() == 0 {
				t.Fatal("the decoder produced no picture out of a stamped stream")
			}
			if found.Load() != arrived.Load() {
				t.Errorf("%d of %d frames reaching the decoder carried a stamp", found.Load(), arrived.Load())
			}
		})
	}
}

// A stamp read off the wire is the instant it was written at, making a subtraction against it
// a delay rather than an offset.
func TestStampCarriesTheClock(t *testing.T) {
	gst.Init()
	for _, factory := range []string{"videotestsrc", "x264enc", "h264parse"} {
		if gst.ElementFactoryFind(factory) == nil {
			t.Skipf("this GStreamer registers no %s", factory)
		}
	}

	pipeline := parsePipeline(t, "videotestsrc num-buffers=5 ! "+
		"video/x-raw,width=320,height=240,framerate=30/1 ! "+
		"x264enc tune=zerolatency ! h264parse ! identity name=stats ! "+
		"video/x-h264,stream-format=byte-stream,alignment=au ! fakesink name=drawn sync=false")

	stampFrames(pipeline, "stats", watchDelay(pipeline, "stats"), &linkWindow{})

	var worst atomic.Int64
	var seen atomic.Uint64
	readStamps(t, pipeline, "drawn", func(s framestamp.Stamp) {
		seen.Add(1)
		if d := time.Since(s.At); d > time.Duration(worst.Load()) {
			worst.Store(int64(d))
		}
	})

	play(t, pipeline)

	if seen.Load() == 0 {
		t.Fatal("no stamp reached the sink")
	}
	// The whole run is a handful of frames off a test pattern, so an instant read on the far side
	// of it is seconds old at the most.
	// A stamp holding anything else is a clock that is not this one.
	if d := time.Duration(worst.Load()); d < 0 || d > 10*time.Second {
		t.Errorf("a stamp read %v old, which is no reading of this clock", d)
	}
}

// What the publishing pipeline measured of its own work rides out with the frames, putting those
// stages in front of a viewer on another machine.
// Carried as the running totals, so what a reader divides is its own interval.
func TestStampCarriesThePublishingSidesReading(t *testing.T) {
	gst.Init()
	for _, factory := range []string{"videotestsrc", "x264enc", "h264parse"} {
		if gst.ElementFactoryFind(factory) == nil {
			t.Skipf("this GStreamer registers no %s", factory)
		}
	}

	// Paced against the clock the probe measures against: a source pushing as fast as it can stamps
	// frames for moments that have not arrived, and the probe refuses those rather than reporting
	// a delay of nothing (internal/pipedelay).
	pipeline := parsePipeline(t, "videotestsrc num-buffers=30 is-live=true ! "+
		"video/x-raw,width=320,height=240,framerate=30/1 ! "+
		"x264enc tune=zerolatency ! h264parse ! identity name=stats ! "+
		"video/x-h264,stream-format=byte-stream,alignment=au ! fakesink name=drawn sync=false")

	window := &linkWindow{}
	// A leg that states one, which on this pipeline nothing does: the reporting tick fills it
	// on a run, and this run plays no sink that keeps a window.
	ms := 300.0
	window.take(&ms)
	stampFrames(pipeline, "stats", watchDelay(pipeline, "stats"), window)

	var last atomic.Pointer[framestamp.Stamp]
	readStamps(t, pipeline, "drawn", func(s framestamp.Stamp) { last.Store(&s) })

	play(t, pipeline)

	s := last.Load()
	if s == nil {
		t.Fatal("no stamp reached the sink")
	}
	// The probe measures the same frames it stamps, so by the last one it has a reading of its own.
	if s.PublishFrames == 0 {
		t.Error("the last frame carried no count of what the publishing pipeline measured")
	}
	if s.LinkMs != 300 {
		t.Errorf("the last frame carried a window of %d ms, want 300", s.LinkMs)
	}
}

func parsePipeline(t *testing.T, description string) gst.Pipeline {
	t.Helper()

	el, err := gst.ParseLaunch(description)
	if err != nil {
		t.Skipf("this GStreamer cannot build the pipeline: %v", err)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		t.Fatalf("the description built a %T rather than a pipeline", el)
	}
	return pipeline
}

// play runs the pipeline to end of stream and fails on anything the bus reports as an error.
func play(t *testing.T, pipeline gst.Pipeline) {
	t.Helper()

	if ret := pipeline.SetState(gst.StatePlaying); ret == gst.StateChangeFailure {
		t.Skip("this machine's GStreamer refused to play the pipeline")
	}
	defer pipeline.SetState(gst.StateNull)

	bus := pipeline.GetBus()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		msg := bus.TimedPop(gst.ClockTime(time.Second))
		if msg == nil {
			continue
		}
		switch msg.Type() {
		case gst.MessageError:
			debug, err := msg.ParseError()
			t.Fatalf("the pipeline failed: %v (%s)", err, debug)
		case gst.MessageEOS:
			return
		}
	}
	t.Fatal("the pipeline neither ended nor failed")
}

// countStamps counts the buffers reaching an element and how many of them carry a readable stamp.
func countStamps(t *testing.T, pipeline gst.Pipeline, element string, stamped, arrived *atomic.Uint64) {
	t.Helper()

	onBuffer(t, pipeline, element, "sink", func(buffer *gst.Buffer) {
		arrived.Add(1)
		if _, found := readBuffer(buffer); found {
			stamped.Add(1)
		}
	})
}

func countBuffers(t *testing.T, pipeline gst.Pipeline, element string, count *atomic.Uint64) {
	t.Helper()

	onBuffer(t, pipeline, element, "sink", func(*gst.Buffer) { count.Add(1) })
}

func readStamps(t *testing.T, pipeline gst.Pipeline, element string, take func(framestamp.Stamp)) {
	t.Helper()

	onBuffer(t, pipeline, element, "sink", func(buffer *gst.Buffer) {
		if s, found := readBuffer(buffer); found {
			take(s)
		}
	})
}

func onBuffer(t *testing.T, pipeline gst.Pipeline, element, padName string, take func(*gst.Buffer)) {
	t.Helper()

	el := pipeline.GetByName(element)
	if el == nil {
		t.Fatalf("the pipeline holds no element named %q", element)
	}
	pad := el.GetStaticPad(padName)
	if pad == nil {
		t.Fatalf("%s has no %s pad", element, padName)
	}
	pad.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		if buffer := padprobe.Buffer(info); buffer != nil {
			take(buffer)
		}
		return gst.PadProbeOK
	})
}

func readBuffer(buffer *gst.Buffer) (framestamp.Stamp, bool) {
	m, mapped := buffer.Map(gst.MapRead)
	if !mapped {
		return framestamp.Stamp{}, false
	}
	defer m.Close()
	return framestamp.Read(m.Data())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}
