package gstrun

import (
	"testing"

	"github.com/go-gst/go-gst/pkg/gst"
)

// A capsfilter's alternatives are narrowed to the surface's own colour, so what the encoder is
// handed is what the desktop had.
//
// The pipelines below stand in for the publish chain: a source claiming a colour,
// the videoconvert every capture chain carries, and the encoder input stating each colour this
// publish accepts.
// What replaces the encoder is a fakesink, because what is under test is the negotiation ahead of
// it and an encoder would only add a plugin this may not carry.
//
// The claim matters because the untouched behaviour is silent.
// videoconvert fixates a capsfilter to its first structure whatever the frames carry,
// so without the narrowing an HDR desktop would be converted into the standard-range row and every
// viewer would be told a colour the surface never had.

// surfaceRows is the colours the encoder input accepts, in the order a publish states them:
// standard range first, then the two BT.2100 curves, all at full range (publish/gstpipeline.go,
// gstColorimetries).
const surfaceRows = "video/x-raw,format=P010_10LE,colorimetry=1:3:5:1" +
	";video/x-raw,format=P010_10LE,colorimetry=1:6:14:7" +
	";video/x-raw,format=P010_10LE,colorimetry=1:6:15:7"

func TestTheEncoderInputTakesTheSurfacesOwnTransfer(t *testing.T) {
	for _, tc := range []struct {
		surface string
		want    string
	}{
		// The two HDR curves: each wins its own row, which is the whole point.
		// Neither leads the rows, so an answer of the leading one is the failure this exists for.
		{surface: "bt2100-pq", want: "1:6:14:7"},
		{surface: "bt2100-hlg", want: "1:6:15:7"},
		// A standard-range desktop takes the leading row, which is what it would take with no narrowing
		// at all: the answer does not move, because the row already described the surface.
		{surface: "bt709", want: "1:3:5:1"},
		// A transfer no row names narrows nothing, so the leading row stands and the parent refuses the
		// run on the same caps, in words that name both ends (publish/gsthdr.go).
		// An empty capsfilter here would fail with a negotiation error instead, which names neither.
		{surface: "1:6:11:7", want: "1:3:5:1"},
	} {
		t.Run(tc.surface, func(t *testing.T) {
			got := encoderInput(t,
				"videotestsrc num-buffers=2 ! video/x-raw,format=BGRx,width=320,height=180,colorimetry="+
					tc.surface+" ! videoconvert ! "+surfaceRows+" ! fakesink name=sink")
			if got != tc.want {
				t.Errorf("a %s surface reaches the encoder as %s, want %s", tc.surface, got, tc.want)
			}
		})
	}
}

// Mastering display metadata reaches the encoder where the capture states it,
// and is absent where it does not.
//
// It rides through because nothing names it: the encoder input pins the memory, the format,
// the colorimetry and the size, so every other field the capture put on its caps survives the
// intersection untouched.
// That is the whole mechanism, and this is what says it stays true - a row that started naming
// these fields would drop the ones it did not match, and a stream would lose the grading it was
// mastered against with nothing saying so.
func TestMasteringMetadataReachesTheEncoder(t *testing.T) {
	const (
		display = "34000:16000:13250:34500:7500:3000:15635:16450:10000000:1"
		light   = "1000:400"
	)
	caps := negotiatedCaps(t,
		"videotestsrc num-buffers=2 ! video/x-raw,format=BGRx,width=320,height=180,colorimetry=bt2100-pq"+
			",mastering-display-info=(string)"+display+",content-light-level=(string)"+light+
			" ! videoconvert ! "+surfaceRows+" ! fakesink name=sink")

	s := caps.GetStructure(0)
	if got := s.GetString("mastering-display-info"); got != display {
		t.Errorf("the mastering display info reaches the encoder as %q, want %q", got, display)
	}
	if got := s.GetString("content-light-level"); got != light {
		t.Errorf("the content light level reaches the encoder as %q, want %q", got, light)
	}

	// A capture stating neither hands the encoder neither, rather than a default somebody made up:
	// a grading nobody measured is worse than none at all.
	plain := negotiatedCaps(t,
		"videotestsrc num-buffers=2 ! video/x-raw,format=BGRx,width=320,height=180,colorimetry=bt2100-pq"+
			" ! videoconvert ! "+surfaceRows+" ! fakesink name=sink")
	if got := plain.GetStructure(0).GetString("mastering-display-info"); got != "" {
		t.Errorf("a capture stating no mastering display reaches the encoder carrying %q", got)
	}
}

// encoderInput plays one description to its end the way a run does, and returns the colorimetry the
// last element was handed.
//
// The pipeline is driven here rather than through Run because the answer is on a pad inside it:
// a run reports what the capture negotiated, and what this is about is the far end of the
// conversion.
// Everything a run does to the pipeline before it plays is done here in the same order.
func encoderInput(t *testing.T, description string) string {
	t.Helper()
	return negotiatedCaps(t, description).GetStructure(0).GetString(colorimetryField)
}

// negotiatedCaps plays one description to its end and returns the caps the element named sink was
// handed.
func negotiatedCaps(t *testing.T, description string) *gst.Caps {
	t.Helper()
	gst.Init()

	el, err := gst.ParseLaunch(description)
	if err != nil {
		t.Fatalf("building the pipeline: %v", err)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		t.Fatalf("the description built a %T rather than a pipeline", el)
	}
	defer pipeline.SetState(gst.StateNull)

	narrowToSurface(pipeline)

	if ret := pipeline.SetState(gst.StatePlaying); ret == gst.StateChangeFailure {
		t.Fatal("the pipeline refused to play")
	}
	for msg := range pipeline.GetBus().Messages(t.Context()) {
		switch msg.Type() {
		case gst.MessageError:
			debug, err := msg.ParseError()
			t.Fatalf("the pipeline failed: %v\n%s", err, debug)
		case gst.MessageEOS:
			return sinkCaps(t, pipeline)
		}
	}
	t.Fatal("the pipeline neither ended nor failed")
	return nil
}

// sinkCaps is what the element named sink was handed.
func sinkCaps(t *testing.T, pipeline gst.Pipeline) *gst.Caps {
	t.Helper()
	sink := pipeline.GetByName("sink")
	if sink == nil {
		t.Fatal("the pipeline carries no element named sink")
	}
	pad := sink.GetStaticPad("sink")
	if pad == nil {
		t.Fatal("the sink has no sink pad")
	}
	caps := pad.GetCurrentCaps()
	if caps == nil || caps.GetSize() == 0 {
		t.Fatal("nothing negotiated caps on the sink")
	}
	return caps
}
