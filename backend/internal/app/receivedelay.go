package app

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The path a frame takes between a screen and a window, as a budget of stages.
//
// Assembled here rather than by a shell, being a model of the path and not arithmetic.
// Which stage a figure belongs to, which figures add together, and what the sum is a sum of,
// are decisions, and a shell making them would be a second place deciding them (docs/ipc-api.md).
//
// One stage is in the path and in no measurement.
// The relay terminates the publishing protocol and re-muxes per reader, and neither end times that:
// the relay API states no per-path delay and is an operator's rather than a member's,
// and no leg carries a relay timestamp to subtract.
// So the total is a floor.

// publishedPathLocked is the relay path this machine publishes to and what its leg costs a frame,
// and the empty path where nothing is published.
//
// The relay path rather than the stream's bare name, because a decode is keyed on it.
// A group publishes under a prefix,
// so bare names would credit its publish to another group's stream of the same name.
//
// procMu is held by the caller.
func (a *App) publishedPathLocked() (string, publishDelay) {
	if a.run == nil {
		return "", publishDelay{}
	}
	s := a.run.settings
	return s.Relay.Path(s.Publish.Name), a.run.delay
}

// receiveDelayOf is the budget of one decode:
// what this machine measured of each stage, and their sum.
//
// now and last are two readings of one pipeline, seen whether there was an earlier one,
// the same three a rate is derived from and for the same reason:
// a transit is a mean over the interval between two readings rather than over the run.
// publishing is this machine's own publishing leg where the decode is of the stream it sends,
// and the zero value otherwise.
func receiveDelayOf(now, last receive.Stats, seen bool, publishing publishDelay) wire.DelayBudget {
	budget := wire.DelayBudget{
		Publish:     firstOf(publishing.Transit, stampedPublishOf(now, last, seen)),
		Path:        pathOf(now, last, seen),
		Receive:     transitOf(now, last, seen),
		ReceivePeak: transitPeakOf(now),
	}
	budget.Present = presentOf(now.LatencyMin, budget.Receive)
	budget.Total = totalOf(budget)

	assert.Assert(budget.Total == nil || *budget.Total >= 0,
		"a floor of measured delays is not negative", budget.Total)
	return budget
}

// transitOf is the mean wall clock one frame spent between the leg's source stamping it
// and the sink taking it, over the interval between two readings.
//
// nil where there is nothing to divide: a first reading, a pipeline rebuilt under one ref,
// which restarts the counters, and an interval no frame crossed.
// The last is nothing to time rather than a frame that took no time.
func transitOf(now, last receive.Stats, seen bool) *float64 {
	if !seen || now.TransitFrames <= last.TransitFrames || now.Transit < last.Transit {
		return nil
	}
	mean := float64(now.Transit-last.Transit) / float64(now.TransitFrames-last.TransitFrames)
	return msOfPtr(time.Duration(mean))
}

// firstOf is the nearer of two readings of one stage, and nil where neither took one.
//
// The publishing stage has two sources and one meaning:
// this machine's own run measures it directly,
// and a stamp carries another machine's over the relay.
// The local reading wins where there is one, being the same figure at full precision,
// there before a frame has been decoded, and there for a codec that carries no stamp at all.
func firstOf(local, stamped *float64) *float64 {
	if local != nil {
		return local
	}
	return stamped
}

// stampedPublishOf is the mean wall clock capture and encode cost the publishing machine reported,
// over the interval between two of this side's readings.
//
// Its counters are that pipeline's own running totals, so they divide as a local probe's do,
// and for the same reason:
// the average worth reading is over the interval between two samples rather than over the run
// (internal/framestamp, Stamp).
// nil on the three grounds a transit is, and on a publish that measured none of its own stages,
// which reaches here as no frames.
func stampedPublishOf(now, last receive.Stats, seen bool) *float64 {
	if !seen || now.PublishFrames <= last.PublishFrames || now.PublishTotal < last.PublishTotal {
		return nil
	}
	mean := float64(now.PublishTotal-last.PublishTotal) / float64(now.PublishFrames-last.PublishFrames)
	return msOfPtr(time.Duration(mean))
}

// pathOf is the mean wall clock one frame spent between the publishing machine's encoder
// and this decoder, over the interval between two readings.
//
// The same three readings a transit is taken from, and nil on the same three grounds:
// a first reading, a pipeline whose counters restarted, and an interval no stamped frame crossed.
// The last also covers every frame of a stream carrying no clock at all,
// a path nothing measured rather than a path of no length (internal/framestamp).
func pathOf(now, last receive.Stats, seen bool) *float64 {
	if !seen || now.PathFrames <= last.PathFrames || now.Path < last.Path {
		return nil
	}
	mean := float64(now.Path-last.Path) / float64(now.PathFrames-last.PathFrames)
	return msOfPtr(time.Duration(mean))
}

// transitPeakOf is the worst that stage has cost a single frame since the decode started.
//
// One reading and no interval, unlike every other stage here:
// a high-water mark already answers over the whole run,
// and subtracting two would read an interval that beat no record as an interval with nothing slow.
// nil before a frame has been measured at all.
func transitPeakOf(now receive.Stats) *float64 {
	if now.TransitPeak <= 0 {
		return nil
	}
	return msOfPtr(now.TransitPeak)
}

// presentOf is what the sink holds a frame for after it arrives:
// the pipeline's latency window less the work already done inside it.
//
// nil where either half is missing, and never negative.
// A transit past the window is a frame the sink drops for being late,
// not one it draws early, and the drop is counted where drops are counted.
func presentOf(latency time.Duration, transit *float64) *float64 {
	if latency <= 0 || transit == nil {
		return nil
	}
	wait := msOf(latency) - *transit
	if wait < 0 {
		wait = 0
	}
	return &wait
}

// totalOf adds the stages that carry a figure, and is nil where none does.
//
// A floor rather than a total.
// Path is the only measurement of the way between the two machines,
// so a stream carrying no stamp adds none of that way and the relay's own share is in no reading
// either way.
// Adding what is there still answers, every absent stage only making the real delay larger.
func totalOf(budget wire.DelayBudget) *float64 {
	stages := []*float64{budget.Publish, budget.Path, budget.Receive, budget.Present}

	total, measured := 0.0, false
	for _, stage := range stages {
		if stage == nil {
			continue
		}
		total += *stage
		measured = true
	}
	if !measured {
		return nil
	}
	return &total
}

// msOfPtr is a duration in milliseconds as an optional figure, the shape a budget's stages carry.
func msOfPtr(d time.Duration) *float64 {
	ms := msOf(d)
	return &ms
}
