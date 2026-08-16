package gstrun

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"
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

// What the shed threw away crosses on the same line, and a pipeline carrying no shed reports no
// figure rather than a drop of nothing: zero is a reading, and a run that counts none has not taken
// it.
func TestADropCountCrossesWithTheDelayAndIsAbsentWithoutAShed(t *testing.T) {
	dropped := uint64(704)
	var counted, uncounted bytes.Buffer

	writeDelay(&counted, Delay{TransitNs: 12_000_000, Frames: 60, Dropped: &dropped})
	writeDelay(&uncounted, Delay{TransitNs: 12_000_000, Frames: 60})

	got, ok := delayLine(counted.String())
	if !ok {
		t.Fatalf("no delay line on the output: %q", counted.String())
	}
	if got.Dropped == nil || *got.Dropped != dropped {
		t.Errorf("read back %v dropped, want %d", got.Dropped, dropped)
	}

	none, ok := delayLine(uncounted.String())
	if !ok {
		t.Fatalf("no delay line on the output: %q", uncounted.String())
	}
	if none.Dropped != nil {
		t.Errorf("read back %v dropped on a run counting none, want it absent", *none.Dropped)
	}
}

// A count on a pipeline that holds no such queue is one nothing took, which reads as absent.
func TestAShedCountIsAbsentWhereNothingCountsOne(t *testing.T) {
	if _, counted := (*shedCount)(nil).Read(); counted {
		t.Error("a pipeline carrying no shed reports a drop count")
	}

	c := &shedCount{}
	c.in.Add(30)
	c.out.Add(4)
	dropped, counted := c.Read()
	if !counted || dropped != 26 {
		t.Errorf("30 in and 4 out reads as %d dropped (counted=%v), want 26", dropped, counted)
	}

	// The two counters are read one after the other, so a frame crossing between them is held rather
	// than dropped.
	held := &shedCount{}
	held.in.Add(4)
	held.out.Add(5)
	if dropped, _ := held.Read(); dropped != 0 {
		t.Errorf("a frame counted out before it was counted in reads as %d dropped, want 0", dropped)
	}
}

// The queue's depth is what separates a frame on its way from one thrown away.
// Counted at the ends alone, every frame in the queue reads as dropped and stops reading that way
// once it leaves, which is a cumulative total that goes down.
func TestFramesInTheShedAreNotCountedAsDropped(t *testing.T) {
	held := uint64(0)
	c := &shedCount{level: func() uint64 { return held }}

	c.in.Add(10)
	held = 10
	if dropped, _ := c.Read(); dropped != 0 {
		t.Errorf("ten frames in the queue read as %d dropped, want none: they have not gone anywhere", dropped)
	}

	// They leave, and still nothing was dropped.
	c.out.Add(10)
	held = 0
	if dropped, _ := c.Read(); dropped != 0 {
		t.Errorf("ten frames through the queue read as %d dropped, want none", dropped)
	}

	// One is thrown away: in and out differ by one with the queue empty.
	c.in.Add(1)
	if dropped, _ := c.Read(); dropped != 1 {
		t.Errorf("one frame taken in and never handed on reads as %d dropped, want 1", dropped)
	}
}

// A total that counts down is a readout counting backwards in front of whoever is watching it.
// The three readings are taken one after another, so one of them moving under the other two is
// ordinary, and the figure holds where it got to.
func TestADropTotalNeverCountsDown(t *testing.T) {
	held := uint64(0)
	c := &shedCount{level: func() uint64 { return held }}

	c.in.Add(9)
	c.out.Add(4)
	first, _ := c.Read()
	if first != 5 {
		t.Fatalf("nine in, four out and none held reads as %d dropped, want 5", first)
	}

	// The queue fills, which subtracts from the same difference.
	held = 3
	if dropped, _ := c.Read(); dropped != first {
		t.Errorf("a queue filling took the total from %d to %d", first, dropped)
	}
}

// The depth is read off the element by property, through a type assertion, and a figure that stops
// matching goes to zero rather than to an error: a zero depth counts every frame in flight as
// dropped, which is the reading this whole count exists to avoid.
// So the assertion is held against what a queue actually answers.
func TestAQueuesDepthIsReadAndNotAssumed(t *testing.T) {
	gst.Init()

	el := gst.ElementFactoryMake("queue", "shed")
	if el == nil {
		t.Skip("this install carries no queue element")
	}
	el.SetObjectProperty("max-size-buffers", uint32(8))

	if got := queueLevel(el)(); got != 0 {
		t.Errorf("an idle queue reads as holding %d buffers, want none", got)
	}
	if _, ok := el.ObjectProperty("current-level-buffers").(uint32); !ok {
		t.Errorf("the depth no longer reads as uint32, so queueLevel needs the width the binding answers: %T",
			el.ObjectProperty("current-level-buffers"))
	}
}
