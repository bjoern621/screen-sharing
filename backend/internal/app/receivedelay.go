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

// publishedStreamLocked is the stream this machine publishes and what its leg costs a frame,
// and the empty name where nothing is published.
//
// Named as a decode is keyed, inside the prefix this machine reaches under,
// so one string decides whether a decode is of this machine's own stream.
//
// procMu is held by the caller.
func (a *App) publishedStreamLocked() (string, publishDelay) {
	if a.run == nil {
		return "", publishDelay{}
	}
	s := a.run.settings
	return s.StreamName(), a.run.delay
}

// receiveDelayOf is the budget of one decode:
// what this machine measured of each stage, and their sum.
//
// now and last are two readings of one pipeline, seen whether there was an earlier one,
// the same three a rate is derived from and for the same reason:
// a transit is a mean over the interval between two readings rather than over the run.
// publishing is this machine's own publishing leg where the decode is of the stream it sends,
// and the zero value otherwise.
//
// Two measurements bracket one stretch of the pipeline in common:
// the way here ends at the decoder, and the transit starts at the leg's source, ahead of it.
// The stages split there, so the sum crosses that stretch once.
func receiveDelayOf(now, last receive.Stats, seen bool, publishing publishDelay) wire.DelayBudget {
	transit := transitOf(now, last, seen)
	arrive := arriveOf(now, last, seen)
	budget := wire.DelayBudget{
		Publish: firstOf(publishing.Transit, stampedPublishOf(now, last, seen)),
		Path:    pathOf(now, last, seen, arrive),
		Arrive:  arrive,
	}
	budget.Decode = decodeOf(transit, budget.Arrive)
	budget.Present = presentOf(now.LatencyMin, transit)
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

// arriveOf is the mean wall clock one frame spent between the leg's source stamping it
// and the decoder being handed it, over the interval between two readings.
//
// The same three readings a transit is taken from, and nil on the same three grounds.
// Also nil before the pipeline picks a decoder, there being no pad to measure at.
func arriveOf(now, last receive.Stats, seen bool) *float64 {
	if !seen || now.ArriveFrames <= last.ArriveFrames || now.Arrive < last.Arrive {
		return nil
	}
	mean := float64(now.Arrive-last.Arrive) / float64(now.ArriveFrames-last.ArriveFrames)
	return msOfPtr(time.Duration(mean))
}

// decodeOf is what the decode itself cost: the transit less the arrival it opens with.
//
// The whole transit where nothing measured the arrival, which is a pipeline holding no decoder
// to measure at and therefore no way here either, so nothing is counted twice.
// Never negative.
// The two are means over one interval taken across different frames,
// so an arrival crossing the transit is those sets parting rather than a decode of less than nothing.
func decodeOf(transit, arrive *float64) *float64 {
	if transit == nil {
		return nil
	}
	if arrive == nil {
		return transit
	}
	work := *transit - *arrive
	if work < 0 {
		work = 0
	}
	return &work
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

// pathOf is the mean wall clock the network and the relay alone spent moving one frame between
// the publishing machine's encoder and this machine's own buffering, over the interval between
// two readings, arrive subtracted out.
//
// Both readings end at the same pad, the one delay.arrive starts counting from,
// so the raw subtraction would carry that stage's stretch twice were it left in.
// nil on the same three grounds a transit is:
// a first reading, a pipeline whose counters restarted, and an interval no stamped frame crossed.
// The last also covers every frame of a stream carrying no clock at all,
// a path nothing measured rather than a path of no length (internal/framestamp).
// Never negative: the two means are taken across frame sets that need not match exactly,
// so buffering that outruns the raw path is those sets parting rather than a stretch shorter
// than nothing (decodeOf).
func pathOf(now, last receive.Stats, seen bool, arrive *float64) *float64 {
	if !seen || now.PathFrames <= last.PathFrames || now.Path < last.Path {
		return nil
	}
	mean := float64(now.Path-last.Path) / float64(now.PathFrames-last.PathFrames)
	path := msOf(time.Duration(mean))
	if arrive != nil {
		path -= *arrive
		if path < 0 {
			path = 0
		}
	}
	return &path
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

// totalOf adds the stretches that carry a figure, each counted once, and is nil where none does.
//
// A floor rather than a total.
// Path is the only measurement of the way between the two machines,
// so a stream carrying no stamp adds none of that way and the relay's own share is in no reading
// either way.
// Adding what is there still answers, every absent stage only making the real delay larger.
func totalOf(budget wire.DelayBudget) *float64 {
	stages := []*float64{budget.Publish, budget.Path, budget.Arrive, budget.Decode, budget.Present}

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
