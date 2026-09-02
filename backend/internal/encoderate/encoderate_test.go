package encoderate

import (
	"context"
	"os/exec"
	"testing"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// probeStream is a stream ordinary enough to measure anywhere: the software encoder every install carries,
// on the GStreamer engine's portal backend.
func probeStream() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "portal"
	s.Publish.UseCodec("libx264")
	s.Publish.Mode = "crf"
	s.Publish.Chroma = "yuv420p"
	s.Publish.Fps = 30
	// A ladder step is one encoder's own identifier, and the defaults carry the default codec's,
	// so the builder refuses those rather than encoding at a step libx264 never heard of.
	// These two are what a draft naming this codec and mode holds after the migration or the repair.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec(), s.Publish.Mode)
	return s
}

// The whole measurement against a real encoder.
// Only the shape of the answer holds on every machine, since the rate is the test host's:
// harder content does not code faster than easier content.
func TestMeasureBracketsTheContentRange(t *testing.T) {
	if _, err := exec.LookPath(publish.GstExe); err != nil {
		t.Skipf("%s not installed", publish.GstExe)
	}

	rate, err := Measure(context.Background(), probeStream(), 320, 240)
	if err != nil {
		t.Fatalf("measuring: %v", err)
	}
	if rate.LowFps <= 0 || rate.HighFps <= 0 {
		t.Errorf("a measurement reports a positive rate at both ends, got %.1f-%.1f",
			rate.LowFps, rate.HighFps)
	}
	if rate.LowFps > rate.HighFps {
		t.Errorf("the hard end of the range codes faster than the easy one: %.1f > %.1f",
			rate.LowFps, rate.HighFps)
	}
	// Bounded is asserted neither way.
	// Which of the generator and the encoder paced a run is a fact about the test host,
	// so pinning it would make a faster CPU a failing build.
}

// The picture size decides how much work a frame is,
// so a size the machine could not report is refused rather than replaced:
// a rate measured at a size the stream will not use answers a question nobody asked.
func TestMeasureRefusesAnUnresolvedPictureSize(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"no width", 0, 240},
		{"no height", 320, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Measure(context.Background(), probeStream(), tc.width, tc.height); err == nil {
				t.Error("the measurement invented a picture size")
			}
		})
	}
}

// The engine follows from the capture backend, exactly as a publish does.
// A backend no engine runs is refused rather than measured on the map's first hit.
func TestMeasureRefusesAnUnknownCaptureBackend(t *testing.T) {
	s := probeStream()
	s.Publish.Capture = "nosuchgrabber"
	if _, err := Measure(context.Background(), s, 320, 240); err == nil {
		t.Error("the measurement picked an engine for a capture backend that has none")
	}
}

// Every publish engine states how its encoder is timed.
// An engine added to publish.Engines without a probe here reaches the assert inside Measure,
// which is a panic in front of a user rather than a failure in front of whoever added it.
func TestEveryEngineIsTimed(t *testing.T) {
	for _, engine := range publish.Engines() {
		if _, ok := engineProbes[engine]; !ok {
			t.Errorf("publish engine %q has no encode-rate probe", engine)
		}
	}
}

// A run whose ends the clock cannot separate says nothing about the encoder.
// A figure divided out of it would be a guess in the field every frame-rate warning is judged against.
func TestRateRefusesAnUnmeasurableRun(t *testing.T) {
	if _, err := rate(probeFrames, 0); err == nil {
		t.Error("a run of no measurable time yielded a rate")
	}
}

// A run the machine paced rather than the encoder can time the hard content above the easy one.
// Both figures are still readings of this machine at these settings,
// so the bracket runs between them:
// refusing would answer a measurement the reader asked for with nothing,
// and reporting them in the order they were taken would put a warning threshold above the rate it bounds.
//
// Each end's own flag travels with its figure, a bounded reading being a fact about that run.
func TestBracketOrdersEndsThatRanTheWrongWayRound(t *testing.T) {
	got := bracket(120, 80, true, false)

	if got.LowFps != 80 || got.HighFps != 120 {
		t.Errorf("the bracket runs %.1f-%.1f fps, want 80.0-120.0", got.LowFps, got.HighFps)
	}
	if got.LowBounded || !got.HighBounded {
		t.Errorf("the bounded flags are low %t high %t, want them carried with their own figures",
			got.LowBounded, got.HighBounded)
	}
}

// The ordinary reading is left exactly as it was measured.
func TestBracketKeepsEndsThatRanTheRightWayRound(t *testing.T) {
	got := bracket(80, 120, true, false)

	if got.LowFps != 80 || got.HighFps != 120 {
		t.Errorf("the bracket runs %.1f-%.1f fps, want 80.0-120.0", got.LowFps, got.HighFps)
	}
	if !got.LowBounded || got.HighBounded {
		t.Errorf("the bounded flags are low %t high %t, want them left where they were measured",
			got.LowBounded, got.HighBounded)
	}
}
