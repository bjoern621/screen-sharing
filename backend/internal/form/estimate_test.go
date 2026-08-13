package form

import (
	"math"
	"testing"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/settings"
)

// estimateTestTolerance is what two figures may differ by and still be one prediction.
// The model is arithmetic over constants,
// so the only difference allowed for is how floats represent them.
const estimateTestTolerance = 1e-9

// The picture every prediction below is priced from: one 1080p output at index 0,
// refreshing at the rate the drafts target,
// so nothing diagnoses the monitor while the bitrate is under test.
func estimateTestDeps() Deps {
	return Deps{Monitors: []display.Monitor{
		{Index: 0, Width: 1920, Height: 1080, Primary: true, RefreshHz: 60},
	}}
}

// estimateTestStream sits on the anchor: the software H.264 encoder,
// whose quantizer counts on the anchor's own scale,
// at the pixel format and the quantizer the anchor was measured on.
// Every case below moves one term of it.
func estimateTestStream() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "x11grab"
	s.Publish.Codec = "libx264"
	s.Publish.Mode = "crf"
	s.Publish.Chroma = "yuv420p"
	s.Publish.ColorRange = "tv"
	s.Publish.Monitor = 0
	s.Publish.Fps = 60
	s.Publish.Cq = 23
	s.Publish.UplinkMbps = 100
	return s
}

func estimateTestClose(got, want float64) bool {
	return math.Abs(got-want) <= estimateTestTolerance
}

// Where the two figures come from: 1920x1080 at 60 is 124,416,000 pixels a second,
// each costing the anchor's 0.07 bits at H.264 4:2:0 on the anchor quantizer, so 8.70912 Mbit/s.
// The same pixel spends 12 bits uncompressed, so 1492.992 Mbit/s raw.
func TestTheAnchorCombinationPredictsTheAnchorFigure(t *testing.T) {
	est := estimate(estimateTestDeps(), estimateTestStream())
	if est == nil {
		t.Fatal("a monitor this machine has and a codec the table holds yield a prediction")
	}
	if !estimateTestClose(est.GetBitrateMbps(), 8.70912) {
		t.Errorf("bitrate = %v, want 8.70912", est.GetBitrateMbps())
	}
	if !estimateTestClose(est.GetRawMbps(), 1492.992) {
		t.Errorf("raw = %v, want 1492.992", est.GetRawMbps())
	}
	if !estimateTestClose(est.GetHeadroomMbps(), 100-8.70912) {
		t.Errorf("headroom = %v, want %v", est.GetHeadroomMbps(), 100-8.70912)
	}
}

// The factors below the anchor figure, in order: 0.5 is six quantizer points on the anchor scale,
// 0.6 is HEVC's coding efficiency, 1.5 is the 4:4:4 weight.
func TestTheCodecAndTheChromaPriceTheAnchorFigure(t *testing.T) {
	s := estimateTestStream()
	s.Publish.Codec = "libx265"
	s.Publish.Chroma = "yuv444p"
	s.Publish.Cq = 29

	est := estimate(estimateTestDeps(), s)
	if est == nil {
		t.Fatal("a monitor this machine has and a codec the table holds yield a prediction")
	}
	want := 8.70912 * 0.5 * 0.6 * 1.5
	if !estimateTestClose(est.GetBitrateMbps(), want) {
		t.Errorf("bitrate = %v, want %v", est.GetBitrateMbps(), want)
	}
	// 4:4:4 spends the 24 bits a pixel that RGB does,
	// so the raw rate is double the anchor's whatever the encoder then makes of it.
	if !estimateTestClose(est.GetRawMbps(), 2985.984) {
		t.Errorf("raw = %v, want 2985.984", est.GetRawMbps())
	}
}

// The same CQ number is a different quality per encoder,
// so the target is placed on the anchor scale before it is priced.
// libvpx counts to 63 and x265 to 51, and the two formats share a coding efficiency,
// so the top of either scale is the same quality and predicts the same rate.
func TestTheQuantizerIsPlacedOnTheAnchorScale(t *testing.T) {
	d := estimateTestDeps()

	onFiftyOne := estimateTestStream()
	onFiftyOne.Publish.Codec = "libx265"
	onFiftyOne.Publish.Cq = 51

	onSixtyThree := estimateTestStream()
	onSixtyThree.Publish.Codec = "libvpx-vp9"
	onSixtyThree.Publish.Cq = 63

	fifty, sixty := estimate(d, onFiftyOne), estimate(d, onSixtyThree)
	if fifty == nil || sixty == nil {
		t.Fatal("both codecs are rows of the capability table and yield a prediction")
	}
	if !estimateTestClose(fifty.GetBitrateMbps(), sixty.GetBitrateMbps()) {
		t.Errorf("the top of a 51-point scale predicts %v and the top of a 63-point one %v, and they are the same quality",
			fifty.GetBitrateMbps(), sixty.GetBitrateMbps())
	}
}

// The encoder holds the target,
// and a quality-model figure beside the number in the field would contradict it.
func TestABitrateTargetIsWhatABitrateModePredicts(t *testing.T) {
	s := estimateTestStream()
	s.Publish.Mode = "cbr"
	s.Publish.BitrateM = 20

	est := estimate(estimateTestDeps(), s)
	if est == nil {
		t.Fatal("a monitor this machine has and a codec the table holds yield a prediction")
	}
	if !estimateTestClose(est.GetBitrateMbps(), 20) {
		t.Errorf("bitrate = %v, want the 20 Mbit/s target", est.GetBitrateMbps())
	}
	if !estimateTestClose(est.GetHeadroomMbps(), 80) {
		t.Errorf("headroom = %v, want 80", est.GetHeadroomMbps())
	}
}

// A lossless encode has no quantizer to price, so the raw rate carries it:
// RGB at 24 bits a pixel is 2985.984 Mbit/s uncompressed,
// and the typical figure is the midpoint of the lossless spread.
func TestALosslessEncodeIsPricedFromTheRawRate(t *testing.T) {
	s := estimateTestStream()
	s.Publish.Codec = "libx265"
	s.Publish.Mode = "lossless"
	s.Publish.Chroma = "gbrp"

	est := estimate(estimateTestDeps(), s)
	if est == nil {
		t.Fatal("a monitor this machine has and a codec the table holds yield a prediction")
	}
	if !estimateTestClose(est.GetRawMbps(), 2985.984) {
		t.Errorf("raw = %v, want 2985.984", est.GetRawMbps())
	}
	if !estimateTestClose(est.GetBitrateMbps(), 2985.984*estimateLosslessTypical) {
		t.Errorf("bitrate = %v, want %v", est.GetBitrateMbps(), 2985.984*estimateLosslessTypical)
	}
}

// A line under the prediction reads as a negative figure rather than a clamped zero,
// the size of the shortfall being what says whether a lower quality would fit.
func TestAnUplinkBelowThePredictionLeavesNegativeHeadroom(t *testing.T) {
	s := estimateTestStream()
	s.Publish.UplinkMbps = 5

	est := estimate(estimateTestDeps(), s)
	if est == nil {
		t.Fatal("a monitor this machine has and a codec the table holds yield a prediction")
	}
	if est.GetHeadroomMbps() >= 0 {
		t.Errorf("headroom = %v, and a 5 Mbit/s line under an 8.7 Mbit/s prediction has none",
			est.GetHeadroomMbps())
	}
	if !estimateTestClose(est.GetHeadroomMbps(), 5-8.70912) {
		t.Errorf("headroom = %v, want %v", est.GetHeadroomMbps(), 5-8.70912)
	}
}

// The monitors are this machine's and the selected index is a stored settings file's,
// so the two can disagree: an output was unplugged, or the platform has no enumerator here.
// An environment condition and not a bug,
// so the prediction is withheld and the rest of the form still resolves.
func TestAMonitorTheMachineDoesNotHavePredictsNothing(t *testing.T) {
	cases := map[string]Deps{
		"no monitor enumerated at all": {},
		"another monitor enumerated": {Monitors: []display.Monitor{
			{Index: 3, Width: 2560, Height: 1440},
		}},
		"the monitor enumerated with no size": {Monitors: []display.Monitor{
			{Index: 0},
		}},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			if est := estimate(d, estimateTestStream()); est != nil {
				t.Errorf("estimate = %v, and a picture with no size prices nothing", est)
			}
		})
	}
}

// These values arrive from a settings file this app did not necessarily write.
// One outside the tables is answered with no prediction rather than a guessed weight,
// the publish path being what says why.
func TestAnUnpriceableSettingPredictsNothing(t *testing.T) {
	cases := map[string]func(*settings.Settings){
		"a pixel format no codec encodes":  func(s *settings.Settings) { s.Publish.Chroma = "nv12" },
		"a rate-control mode that is none": func(s *settings.Settings) { s.Publish.Mode = "magic" },
		"a codec the table does not hold":  func(s *settings.Settings) { s.Publish.Codec = "libnope" },
		"no frame rate at all":             func(s *settings.Settings) { s.Publish.Fps = 0 },
	}
	for name, move := range cases {
		t.Run(name, func(t *testing.T) {
			s := estimateTestStream()
			move(&s)
			if est := estimate(estimateTestDeps(), s); est != nil {
				t.Errorf("estimate = %v, and this setting has no price", est)
			}
		})
	}
}

// Resolve runs on every keystroke,
// so a figure that moved between two identical drafts would move under a user who changed nothing.
func TestTheSameDraftPredictsTheSameFigureTwice(t *testing.T) {
	d, s := estimateTestDeps(), estimateTestStream()
	first, second := estimate(d, s), estimate(d, s)
	if first == nil || second == nil {
		t.Fatal("a monitor this machine has and a codec the table holds yield a prediction")
	}
	if first.GetBitrateMbps() != second.GetBitrateMbps() ||
		first.GetRawMbps() != second.GetRawMbps() ||
		first.GetHeadroomMbps() != second.GetHeadroomMbps() {
		t.Errorf("one draft predicted %v and then %v", first, second)
	}
}
