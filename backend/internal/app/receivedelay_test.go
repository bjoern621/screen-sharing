package app

import (
	"math"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/receive"
)

// srtGroups is a watch leg that states the window its transport holds a packet for.
func srtGroups(windowMs float64) []receive.StatGroup {
	return []receive.StatGroup{{
		Factory: "srtsrc",
		Element: "srtsrc0",
		Values: []receive.StatValue{
			{Key: "rtt-ms", Value: 9},
			{Key: "negotiated-latency-ms", Value: windowMs},
		},
	}}
}

func ms(v float64) *float64 { return &v }

// close reports whether an optional figure carries want, to the tenth of a millisecond.
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
// An encoder or a decoder that slows down halfway shows it in the next sample rather than being
// averaged out by the minutes before it.
func TestReceiveDelayTransitIsPerInterval(t *testing.T) {
	last := receive.Stats{Transit: 100 * time.Millisecond, TransitFrames: 10}
	now := receive.Stats{Transit: 400 * time.Millisecond, TransitFrames: 20}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	// 300 ms across 10 frames, and never the run's 400 across 20.
	closeTo(t, "receive", budget.Receive, ms(30))
}

// A first reading has nothing to divide, and neither does a pipeline rebuilt under one key: its
// counters restart, so the reading before it describes something that no longer exists.
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

// An interval no frame crossed is nothing to time, which is a different reading from a frame that
// took no time.
func TestReceiveDelayAnIdleIntervalTimesNothing(t *testing.T) {
	last := receive.Stats{Transit: 100 * time.Millisecond, TransitFrames: 10}
	now := receive.Stats{Transit: 100 * time.Millisecond, TransitFrames: 10}

	if budget := receiveDelayOf(now, last, true, publishDelay{}); budget.Receive != nil {
		t.Errorf("an interval with no frames measures %v ms, want it absent", *budget.Receive)
	}
}

// Work and wait together are the pipeline's latency window: what the sink still holds a frame for
// is that window less the work already done inside it.
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

// A transit past the window is a frame the sink drops for being late, not one it draws early, so
// the wait floors at zero rather than going negative.
func TestReceiveDelayPresentNeverGoesNegative(t *testing.T) {
	last := receive.Stats{}
	now := receive.Stats{
		Transit: 900 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
	}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	closeTo(t, "present", budget.Present, ms(0))
}

// A pipeline that has answered no latency query yet leaves the wait unmeasured rather than reading
// it as the whole window being work.
func TestReceiveDelayPresentNeedsAWindow(t *testing.T) {
	now := receive.Stats{Transit: 200 * time.Millisecond, TransitFrames: 10}

	if budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{}); budget.Present != nil {
		t.Errorf("a pipeline with no latency window waits %v ms, want it absent", *budget.Present)
	}
}

// The window a leg's own transport holds a packet for comes off that leg's counters, and a leg
// whose elements state none leaves it absent.
func TestReceiveDelayWatchLinkComesOffTheLeg(t *testing.T) {
	withSrt := receive.Stats{Groups: srtGroups(120)}
	closeTo(t, "watch link", receiveDelayOf(withSrt, receive.Stats{}, false, publishDelay{}).WatchLink, ms(120))

	jitter := receive.Stats{Groups: []receive.StatGroup{{
		Factory: "rtpjitterbuffer",
		Element: "rtpjitterbuffer0",
		Values:  []receive.StatValue{{Key: "num-pushed", Value: 900}},
	}}}
	if budget := receiveDelayOf(jitter, receive.Stats{}, false, publishDelay{}); budget.WatchLink != nil {
		t.Errorf("a leg stating no window reports %v ms, want it absent", *budget.WatchLink)
	}
}

// The total is the stages that were measured, added up, and no stage is invented to fill a gap.
// It is therefore a floor: the relay's own share is in the path and in no measurement.
func TestReceiveDelayTotalAddsTheMeasuredStages(t *testing.T) {
	last := receive.Stats{}
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
		Groups:     srtGroups(120),
	}

	budget := receiveDelayOf(now, last, true, publishDelay{Transit: ms(8), Link: ms(300)})

	// 8 publish, 300 out, 120 back, 20 decode, 40 waiting at the sink.
	closeTo(t, "total", budget.Total, ms(488))
}

// A decode of somebody else's stream has no publishing stages at all, and the total is then what
// this side alone could measure.
func TestReceiveDelayTotalWithoutThePublishingSide(t *testing.T) {
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
		Groups:     srtGroups(120),
	}

	budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{})

	if budget.Publish != nil || budget.PublishLink != nil {
		t.Error("a decode of another machine's stream carries publishing stages, want them absent")
	}
	closeTo(t, "total", budget.Total, ms(180))
}

// A decode nothing has measured yet reports no total, rather than a total of nothing.
func TestReceiveDelayTotalIsAbsentWithNoStage(t *testing.T) {
	if budget := receiveDelayOf(receive.Stats{}, receive.Stats{}, false, publishDelay{}); budget.Total != nil {
		t.Errorf("an unmeasured decode totals %v ms, want it absent", *budget.Total)
	}
}
