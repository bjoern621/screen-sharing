package gstrun

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// delayLine is the reported reading on a run's output, and false where none was written.
// The newest, since a run of more than one interval writes one per tick.
func delayLine(output string) (Delay, bool) {
	var newest Delay
	found := false
	for _, line := range strings.Split(output, "\n") {
		if d, ok := ParseDelay(line); ok {
			newest, found = d, true
		}
	}
	return newest, found
}

// The parser reads back what the writer wrote, presence included.
// The two live in one file so they stay one spelling, and this is what says they are.
func TestADelayLineRoundTripsThroughItsParser(t *testing.T) {
	window, rtt := 300.0, 8.5
	var out bytes.Buffer

	writeDelay(&out, Delay{TransitNs: 12_000_000, Frames: 60, LinkMs: &window, RttMs: &rtt})

	got, ok := delayLine(out.String())
	if !ok {
		t.Fatalf("no delay line on the output: %q", out.String())
	}
	if got.TransitNs != 12_000_000 || got.Frames != 60 {
		t.Errorf("read back %d ns over %d frames, want 12000000 over 60", got.TransitNs, got.Frames)
	}
	if got.LinkMs == nil || *got.LinkMs != window {
		t.Errorf("read back a window of %v, want %v", got.LinkMs, window)
	}
	if got.RttMs == nil || *got.RttMs != rtt {
		t.Errorf("read back a round trip of %v, want %v", got.RttMs, rtt)
	}
}

// A leg that keeps no link counters reports the transit alone, and the two link figures come back
// absent rather than as zeros.
// Zero is a link with no delay and no distance, which no leg has.
func TestADelayLineWithoutALinkCarriesNoWindow(t *testing.T) {
	var out bytes.Buffer

	writeDelay(&out, Delay{TransitNs: 5_000_000, Frames: 10})

	got, ok := delayLine(out.String())
	if !ok {
		t.Fatalf("no delay line on the output: %q", out.String())
	}
	if got.LinkMs != nil || got.RttMs != nil {
		t.Errorf("a leg keeping no counters reported %v and %v, want both absent", got.LinkMs, got.RttMs)
	}
}

// A line that is not a reading is not one, which is the whole of what a reader pointed at a child's
// standard output needs: an unrelated line there is the ordinary case.
func TestALineThatIsNotADelayIsNotRead(t *testing.T) {
	for _, line := range []string{
		"",
		"stats (00:00:07): 141 buffers",
		CapsPrefix + "video/x-raw",
		DelayPrefix + "not json",
	} {
		if _, ok := ParseDelay(line); ok {
			t.Errorf("%q read as a delay reading", line)
		}
	}
}

// The reading is the pipeline's own, taken while it runs.
//
// videotestsrc is-live=true stamps each buffer when it produces it, so the figure at the element
// after it is the wall clock the pipeline spent on that frame, which is what the whole measurement
// is: a subtraction and not a setting.
func TestARunTimesItsOwnPipeline(t *testing.T) {
	// Two intervals of the reporter's own second, so the run writes a reading and is not racing the
	// first tick.
	ctx, stop := context.WithTimeout(t.Context(), 2500*time.Millisecond)
	defer stop()

	var out lockedBuffer
	err := RunWithOptions(ctx,
		"videotestsrc is-live=true ! video/x-raw,width=64,height=64,framerate=30/1"+
			" ! progressreport name=stats update-freq=1 format=buffers do-query=false ! fakesink sync=false",
		Options{Delay: "stats"}, &out)
	if err != nil {
		t.Fatalf("running: %v", err)
	}

	got, ok := delayLine(out.String())
	if !ok {
		t.Fatalf("no delay line on the run's output: %q", out.String())
	}
	if got.Frames == 0 {
		t.Fatal("the run timed no frames at all, so nothing was measured")
	}

	// A second of a 30 fps pipeline, so the mean is well under a frame period and far under the run.
	mean := time.Duration(got.TransitNs / got.Frames)
	if mean <= 0 || mean > time.Second {
		t.Errorf("a frame crossed the pipeline in %v over %d frames, want a plausible wall clock",
			mean, got.Frames)
	}
}

// A pipeline holding no element of that name times nothing, and runs.
// The name comes from the parent, so a wrong one is this repo's bug rather than the machine's, and
// the stream is not what pays for it.
func TestARunWithNothingToMeasureStillPlays(t *testing.T) {
	var out bytes.Buffer

	err := RunWithOptions(t.Context(), "videotestsrc num-buffers=2 ! fakesink",
		Options{Delay: "nothing-of-that-name"}, &out)
	if err != nil {
		t.Fatalf("a run that can measure nothing reported: %v", err)
	}
	if _, ok := delayLine(out.String()); ok {
		t.Error("a run with nothing to measure reported a reading")
	}
}
