package app

import (
	"math"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/receive"
)

func ms(v float64) *float64 { return &v }

// closeTo reports whether an optional figure carries want, to the tenth of a millisecond.
func closeTo(t *testing.T, name string, got *float64, want *float64) {
	t.Helper()

	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s is absent, want %v ms", name, *want)
	case want == nil:
		t.Errorf("%s = %v ms, want it absent", name, *got)
	case math.Abs(*got-*want) > 0.1:
		t.Errorf("%s = %v ms, want %v ms", name, *got, *want)
	}
}

// The transit is a mean over the interval between two readings, not over the run.
// An encoder or a decoder that slows down halfway shows it in the next sample
// rather than averaged out by the minutes before it.
func TestReceiveDelayTransitIsPerInterval(t *testing.T) {
	last := receive.Stats{Transit: 100 * time.Millisecond, TransitFrames: 10}
	now := receive.Stats{Transit: 400 * time.Millisecond, TransitFrames: 20}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	// 300 ms across 10 frames, where the run holds 400 across 20.
	closeTo(t, "receive", budget.Receive, ms(30))
}

// A first reading has nothing to divide, and neither does a pipeline rebuilt under one key:
// its counters restart, so the reading before it describes a pipeline that is gone.
func TestReceiveDelayNeedsTwoReadingsOfOneRun(t *testing.T) {
	now := receive.Stats{Transit: 400 * time.Millisecond, TransitFrames: 20}

	if budget := receiveDelayOf(now, receive.Stats{}, false, publishDelay{}); budget.Receive != nil {
		t.Errorf("a first reading measures a transit of %v ms, want it absent", *budget.Receive)
	}

	rebuilt := receive.Stats{Transit: 10 * time.Millisecond, TransitFrames: 2}
	if budget := receiveDelayOf(rebuilt, now, true, publishDelay{}); budget.Receive != nil {
		t.Errorf("a restarted pipeline measures a transit of %v ms, want it absent", *budget.Receive)
	}
}

// An interval no frame crossed is nothing to time,
// a different reading from a frame that took no time.
func TestReceiveDelayAnIdleIntervalTimesNothing(t *testing.T) {
	last := receive.Stats{Transit: 100 * time.Millisecond, TransitFrames: 10}
	now := receive.Stats{Transit: 100 * time.Millisecond, TransitFrames: 10}

	if budget := receiveDelayOf(now, last, true, publishDelay{}); budget.Receive != nil {
		t.Errorf("an interval with no frames measures %v ms, want it absent", *budget.Receive)
	}
}

// Work and wait together are the pipeline's latency window:
// what the sink still holds a frame for is that window less the work already done inside it.
func TestReceiveDelayPresentIsTheRestOfTheWindow(t *testing.T) {
	last := receive.Stats{Transit: 0, TransitFrames: 0}
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
	}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	closeTo(t, "receive", budget.Receive, ms(20))
	closeTo(t, "present", budget.Present, ms(40))
}

// A transit past the window is a frame the sink drops for being late, not one it draws early,
// so the wait floors at zero rather than going negative.
func TestReceiveDelayPresentNeverGoesNegative(t *testing.T) {
	last := receive.Stats{}
	now := receive.Stats{
		Transit: 900 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
	}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	closeTo(t, "present", budget.Present, ms(0))
}

// A pipeline that has answered no latency query leaves the wait unmeasured
// rather than reading it as the whole window being work.
func TestReceiveDelayPresentNeedsAWindow(t *testing.T) {
	now := receive.Stats{Transit: 200 * time.Millisecond, TransitFrames: 10}

	if budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{}); budget.Present != nil {
		t.Errorf("a pipeline with no latency window waits %v ms, want it absent", *budget.Present)
	}
}

// What the way from the publishing machine cost is a mean over the interval between two readings,
// like every other measured stage.
func TestReceiveDelayPathIsPerInterval(t *testing.T) {
	last := receive.Stats{Path: 100 * time.Millisecond, PathFrames: 10}
	now := receive.Stats{Path: 700 * time.Millisecond, PathFrames: 20}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	closeTo(t, "path", budget.Path, ms(60))
}

// A stream carrying no clock leaves the way here unmeasured,
// and so does an interval no stamped frame crossed.
func TestReceiveDelayPathNeedsStampedFrames(t *testing.T) {
	unstamped := receive.Stats{Transit: 200 * time.Millisecond, TransitFrames: 10}
	if budget := receiveDelayOf(unstamped, receive.Stats{}, true, publishDelay{}); budget.Path != nil {
		t.Errorf("an unstamped stream measures a path of %v ms, want it absent", *budget.Path)
	}

	idle := receive.Stats{Path: 100 * time.Millisecond, PathFrames: 10}
	if budget := receiveDelayOf(idle, idle, true, publishDelay{}); budget.Path != nil {
		t.Errorf("an interval with no stamped frame measures %v ms, want it absent", *budget.Path)
	}
}

// The way between the two machines is one measured stage of the total, counted like the rest.
func TestReceiveDelayTotalCountsTheMeasuredPath(t *testing.T) {
	last := receive.Stats{}
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		Path: 400 * time.Millisecond, PathFrames: 10,
		LatencyMin: 60 * time.Millisecond,
	}

	budget := receiveDelayOf(now, last, true, publishDelay{Transit: ms(8)})

	// 8 encoding, 40 across, 20 decoding, 40 waiting at the sink.
	closeTo(t, "total", budget.Total, ms(108))
}

// The total is the stages that were measured, added up, and no stage is invented to fill a gap.
// So a floor: a stream carrying no stamp leaves the whole way between the machines unmeasured.
func TestReceiveDelayTotalAddsTheMeasuredStages(t *testing.T) {
	last := receive.Stats{}
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
	}

	budget := receiveDelayOf(now, last, true, publishDelay{Transit: ms(8)})

	// 8 publish, 20 decode, 40 waiting at the sink, and nothing for the way here.
	closeTo(t, "total", budget.Total, ms(68))
}

// A decode of another machine's stream reads its publishing stage off the stamp its frames carry,
// the only way it crosses a relay.
func TestReceiveDelayPublishingStagesComeOffTheStamp(t *testing.T) {
	last := receive.Stats{PublishTotal: 100 * time.Millisecond, PublishFrames: 10}
	now := receive.Stats{PublishTotal: 340 * time.Millisecond, PublishFrames: 40}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	// 240 ms across 30 frames, where the run holds 340 across 40.
	closeTo(t, "publish", budget.Publish, ms(8))
}

// This machine's own reading of its own publish wins over the one that went round the relay:
// the same measurement at full precision, and there for every codec, stamped or not.
func TestReceiveDelayPrefersTheLocalPublishReading(t *testing.T) {
	now := receive.Stats{PublishTotal: 900 * time.Millisecond, PublishFrames: 10}

	budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{Transit: ms(8)})

	closeTo(t, "publish", budget.Publish, ms(8))
}

// A stream carrying no stamp, published by another machine, states no publishing stage at all
// rather than a stage of nothing.
func TestReceiveDelayWithoutAnyPublishingReading(t *testing.T) {
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
	}

	budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{})

	if budget.Publish != nil {
		t.Error("an unstamped stream from another machine carries a publishing stage, want it absent")
	}
	closeTo(t, "total", budget.Total, ms(60))
}

// A decode nothing has measured reports no total, rather than a total of nothing.
func TestReceiveDelayTotalIsAbsentWithNoStage(t *testing.T) {
	if budget := receiveDelayOf(receive.Stats{}, receive.Stats{}, false, publishDelay{}); budget.Total != nil {
		t.Errorf("an unmeasured decode totals %v ms, want it absent", *budget.Total)
	}
}
