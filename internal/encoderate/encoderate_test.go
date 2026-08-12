package encoderate

import (
	"context"
	"os/exec"
	"testing"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// probeStream is a stream small and ordinary enough to measure in a test: the
// software encoder every install carries, on the GStreamer engine's portal backend.
func probeStream() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "portal"
	s.Publish.Codec = "libx264"
	s.Publish.Mode = "crf"
	s.Publish.Chroma = "yuv420p"
	s.Publish.Fps = 30
	// The two ladder steps this codec declares for this mode, which is what a draft
	// naming it holds after the migration or the repair. The defaults carry the default
	// codec's, and a step is one encoder's own identifier, so the builder refuses it here
	// rather than encoding at a step libx264 never heard of.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)
	return s
}

// The whole measurement, end to end, on a real encoder. What it proves is the shape
// of the answer rather than a figure: the rate depends on the machine running the
// test, and the only thing that holds on all of them is that harder content does not
// code faster than easier content.
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
	// Bounded is deliberately not asserted either way. Whether the generator or the
	// encoder paced a run is a fact about the machine the test happens to run on, and
	// pinning it would make a faster CPU a failing build.
}

// The picture size is what decides how much work a frame is, so a size the machine
// could not report is refused rather than replaced with one of this package's
// choosing: a rate measured at a size the stream will not use answers a question
// nobody asked.
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

// The engine follows from the capture backend, exactly as a publish does, so a backend
// no engine runs is refused here rather than measured on whichever engine the map
// happens to iterate to first.
func TestMeasureRefusesAnUnknownCaptureBackend(t *testing.T) {
	s := probeStream()
	s.Publish.Capture = "nosuchgrabber"
	if _, err := Measure(context.Background(), s, 320, 240); err == nil {
		t.Error("the measurement picked an engine for a capture backend that has none")
	}
}

// Every publish engine has to state how its encoder is timed. An engine added to
// publish.Engines without a probe here would reach the assert inside Measure, which
// is a panic in front of a user rather than a failure in front of whoever added it.
func TestEveryEngineIsTimed(t *testing.T) {
	for _, engine := range publish.Engines() {
		if _, ok := engineProbes[engine]; !ok {
			t.Errorf("publish engine %q has no encode-rate probe", engine)
		}
	}
}

// A run the clock cannot separate the ends of says nothing about how fast the encoder
// is, and a figure divided out of it would be a guess in the one field every
// frame-rate warning is judged against.
func TestRateRefusesAnUnmeasurableRun(t *testing.T) {
	if _, err := rate(probeFrames, 0); err == nil {
		t.Error("a run of no measurable time yielded a rate")
	}
}
