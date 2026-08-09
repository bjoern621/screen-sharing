package form

import (
	"math"
	"testing"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/settings"
)

// estimateTestTolerance is what two figures may differ by and still be the same
// prediction. The model is arithmetic over constants, so the only difference it allows
// for is the float representation of them.
const estimateTestTolerance = 1e-9

// The picture every prediction below is priced from: one 1080p output at index 0,
// running at the rate the streams target, so nothing here diags about the monitor while
// testing the bitrate.
func estimateTestDeps() Deps {
	return Deps{Monitors: []display.Monitor{
		{Index: 0, Width: 1920, Height: 1080, Primary: true, RefreshHz: 60},
	}}
}

// estimateTestStream sits exactly on the anchor: the software H.264 encoder, whose
// quantizer counts on the anchor's own 51-point scale, at the pixel format and the
// quantizer the anchor was measured on. Every other case below moves one term of it.
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

// 1920x1080 at 60 is 124,416,000 pixels a second. On the anchor - H.264 at 1.0, 4:2:0
// at 1.0, the quantizer on the scale it was calibrated on - each of them costs the
// anchor's 0.07 bits, which is 8.70912 Mbit/s. Uncompressed, 4:2:0 spends 12 bits on
// the same pixel, which is 1492.992.
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

// Six quantizer points on the anchor scale halve the rate, and the coding efficiency
// and the chroma weight multiply it. HEVC at 0.6 and 4:4:4 at 1.5, six points softer
// than the anchor: 8.70912 * 0.5 * 0.6 * 1.5.
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
	// 4:4:4 carries the same 24 bits a pixel that RGB does, so the uncompressed rate is
	// double the anchor's regardless of what the encoder then does with it.
	if !estimateTestClose(est.GetRawMbps(), 2985.984) {
		t.Errorf("raw = %v, want 2985.984", est.GetRawMbps())
	}
}

// The same CQ number is a different quality per encoder, so the target is placed on the
// anchor scale before it is priced. libvpx counts to 63 and x265 to 51, and both codecs
// produce a format of the same coding efficiency, so the top of either scale is the same
// quality and has to predict the same rate.
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

// A mode that aims at a bitrate the user set is predicted at that target: the encoder
// holds it, and a quality-model figure beside the number in the field would be the form
// contradicting itself.
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

// A lossless encode has no quantizer to price, so it is priced from the raw rate: RGB at
// 24 bits a pixel is 2985.984 Mbit/s uncompressed, and the typical figure sits between
// the near-static end and the motion end of the lossless spread.
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

// The headroom is the whole point of the uplink field: a line under the prediction reads
// as a negative figure rather than as a clamped zero, since the size of the shortfall is
// what says whether a lower quality would fit.
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

// The monitors are this machine's and the selected index is a stored settings file's, so
// the two can disagree: an output was unplugged, or the platform has no enumerator here.
// That is an environment condition and not a bug, so the prediction is withheld and the
// rest of the form still resolves.
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

// The pixel format and the rate-control mode arrive from a settings file this app did not
// necessarily write. A value outside the tables is answered with no prediction rather
// than priced against a guessed weight, the publish path being the one that says why.
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

// Resolve runs on every keystroke, so a prediction that moved between two identical
// drafts would move the figure under a user who changed nothing.
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
