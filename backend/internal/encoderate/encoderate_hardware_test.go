//go:build hardware

// The one measurement against a real encoder, which reports the rate of whatever machine runs it.
// A runner codes too little in the probe's window to time,
// and answers "no measurable time in it" rather than anything about this commit.
// `task backend:test` sets the tag, and CI leaves it off (.github/workflows/check.yml).

package encoderate

import (
	"context"
	"os/exec"
	"testing"

	"bjoernblessin.de/screenshare/internal/publish"
)

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
