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

// What the way from the publishing machine cost is a mean over the interval between two readings,
// like every other measured stage.
func TestReceiveDelayPathIsPerInterval(t *testing.T) {
	last := receive.Stats{Path: 100 * time.Millisecond, PathFrames: 10}
	now := receive.Stats{Path: 700 * time.Millisecond, PathFrames: 20}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	closeTo(t, "path", budget.Path, ms(60))
}

// A stream carrying no clock leaves the way here unmeasured, and so does an interval no stamped
// frame crossed.
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

// The relay's own share is what the way here cost less the two legs' own windows, which is a figure
// exactly where both legs state one.
func TestReceiveDelayRelayIsWhatTheLegsDoNotAccountFor(t *testing.T) {
	now := receive.Stats{Path: 200 * time.Millisecond, PathFrames: 1, Groups: srtGroups(120)}

	budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{Link: ms(60)})

	// 200 across, of which 60 out and 120 back are the legs' own.
	closeTo(t, "relay", budget.Relay, ms(20))
}

// A leg that states no window leaves the relay's share inside the measured whole rather than
// derived from a figure that is not there.
func TestReceiveDelayRelayNeedsBothWindows(t *testing.T) {
	now := receive.Stats{Path: 200 * time.Millisecond, PathFrames: 1}

	if budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{}); budget.Relay != nil {
		t.Errorf("a path with no stated windows derives a relay share of %v ms, want it absent", *budget.Relay)
	}
}

// Two windows summing past what the way here cost describe no relay at all: the measurement is the
// one thing that was measured, so the derivation is dropped rather than reported as negative.
func TestReceiveDelayRelayIsNeverNegative(t *testing.T) {
	now := receive.Stats{Path: 50 * time.Millisecond, PathFrames: 1, Groups: srtGroups(120)}

	if budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{Link: ms(60)}); budget.Relay != nil {
		t.Errorf("windows past the measured path derive a relay share of %v ms, want it absent", *budget.Relay)
	}
}

// A measured way here stands for the three stages it spans, so the total counts it once instead of
// adding the legs' windows on top of it.
func TestReceiveDelayTotalCountsTheMeasuredPathOnce(t *testing.T) {
	last := receive.Stats{}
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		Path: 400 * time.Millisecond, PathFrames: 10,
		LatencyMin: 60 * time.Millisecond,
		Groups:     srtGroups(120),
	}

	budget := receiveDelayOf(now, last, true, publishDelay{Transit: ms(8), Link: ms(300)})

	// 8 encoding, 40 across, 20 decoding, 40 waiting at the sink, and neither window on top.
	closeTo(t, "total", budget.Total, ms(108))
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

// A decode of somebody else's stream reads that machine's own stages off the stamp its frames
// carry, which is the only way they cross a relay.
func TestReceiveDelayPublishingStagesComeOffTheStamp(t *testing.T) {
	last := receive.Stats{PublishTotal: 100 * time.Millisecond, PublishFrames: 10}
	now := receive.Stats{
		PublishTotal: 340 * time.Millisecond, PublishFrames: 40,
		PublishLink: 300 * time.Millisecond,
	}

	budget := receiveDelayOf(now, last, true, publishDelay{})

	// 240 ms across 30 frames, and never the run's 340 across 40.
	closeTo(t, "publish", budget.Publish, ms(8))
	closeTo(t, "publish link", budget.PublishLink, ms(300))
}

// This machine's own reading of its own publish wins over the one that went round the relay: it is
// the same measurement at full precision and it is there for every codec, stamped or not.
func TestReceiveDelayPrefersTheLocalPublishReading(t *testing.T) {
	now := receive.Stats{
		PublishTotal: 900 * time.Millisecond, PublishFrames: 10,
		PublishLink: 900 * time.Millisecond,
	}

	budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{Transit: ms(8), Link: ms(300)})

	closeTo(t, "publish", budget.Publish, ms(8))
	closeTo(t, "publish link", budget.PublishLink, ms(300))
}

// A stream carrying no stamp, published by another machine, states no publishing stage at all
// rather than a stage of nothing.
func TestReceiveDelayWithoutAnyPublishingReading(t *testing.T) {
	now := receive.Stats{
		Transit: 200 * time.Millisecond, TransitFrames: 10,
		LatencyMin: 60 * time.Millisecond,
		Groups:     srtGroups(120),
	}

	budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{})

	if budget.Publish != nil || budget.PublishLink != nil {
		t.Error("an unstamped stream from another machine carries publishing stages, want them absent")
	}
	closeTo(t, "total", budget.Total, ms(180))
}

// A leg stating no window carries a zero one, which is no window rather than a window of nothing.
func TestReceiveDelayStampedLinkOfZero(t *testing.T) {
	now := receive.Stats{PublishTotal: 80 * time.Millisecond, PublishFrames: 10}

	if budget := receiveDelayOf(now, receive.Stats{}, true, publishDelay{}); budget.PublishLink != nil {
		t.Errorf("a stamped leg stating no window reports %v ms, want it absent", *budget.PublishLink)
	}
}

// A decode nothing has measured yet reports no total, rather than a total of nothing.
func TestReceiveDelayTotalIsAbsentWithNoStage(t *testing.T) {
	if budget := receiveDelayOf(receive.Stats{}, receive.Stats{}, false, publishDelay{}); budget.Total != nil {
		t.Errorf("an unmeasured decode totals %v ms, want it absent", *budget.Total)
	}
}
