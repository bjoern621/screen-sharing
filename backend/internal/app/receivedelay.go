package app

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The path a frame takes between a screen and a window, as a budget of stages.
//
// It is assembled here rather than by a shell because it is a model of the path and not arithmetic:
// which stage a figure belongs to, which figures may be added together and what the total is a
// total of are decisions, and a shell that made them would be a second place deciding them
// (docs/ipc-api.md).
//
// One stage is in the path and in no measurement.
// The relay terminates the publishing protocol and re-muxes for every reader, and neither end can
// time that: its API states no per-path delay and is an operator's rather than a member's, and no
// leg carries a relay timestamp for a receiver to subtract.
// So the total is a floor, and saying so is the whole reason it is assembled in one place.

// publishedPathLocked is the relay path this machine is publishing to and what its leg costs a
// frame, and the empty path where nothing is being published.
//
// The relay path rather than the stream's own name, because that is what a decode is keyed on: a
// group publishes under a prefix, and comparing the bare names would credit one group's publish to
// another group's stream of the same name.
//
// procMu is held by the caller.
func (a *App) publishedPathLocked() (string, publishDelay) {
	if a.run == nil {
		return "", publishDelay{}
	}
	s := a.run.settings
	return s.Relay.Path(s.Publish.Name), a.run.delay
}

// watchLinkSources names, per element a watch leg can hold, the counter stating the delay that
// element's transport holds a packet for before the pipeline is handed it.
//
// A table because it is the same shape statSources is: which counter means the leg's window is the
// element's own knowledge, and a rule restated per call site is one that drifts.
// An element with no row states no window, which is not a window of zero: a leg that buffers inside
// the pipeline instead reports that buffering through the latency query, where present answers for
// it.
var watchLinkSources = map[string]string{
	"srtsrc": "negotiated-latency-ms",
}

// receiveDelayOf is the budget of one decode: what this machine measured of each stage, and their
// sum.
//
// now and last are two readings of one pipeline and seen whether there was an earlier one, the same
// three a rate is derived from and for the same reason: the transit is a mean over the interval
// between two readings rather than over the run.
// publishing is this machine's own publishing leg where the decode is of the stream it sends, and
// the zero value where it is not.
func receiveDelayOf(now, last receive.Stats, seen bool, publishing publishDelay) wire.DelayBudget {
	budget := wire.DelayBudget{
		Publish:     publishing.Transit,
		PublishLink: publishing.Link,
		WatchLink:   watchLinkOf(now.Groups),
		Receive:     transitOf(now, last, seen),
	}
	budget.Present = presentOf(now.LatencyMin, budget.Receive)
	budget.Total = totalOf(budget)

	assert.Assert(budget.Total == nil || *budget.Total >= 0,
		"a floor of measured delays is not negative", budget.Total)
	return budget
}

// transitOf is the mean wall clock one frame spent between the leg's source stamping it and the
// sink taking it, over the interval between two readings.
//
// nil where there is nothing to divide: a first reading, a pipeline rebuilt under one key, which
// restarts the counters, and an interval no frame crossed.
// The last of those is nothing to time rather than a frame that took no time.
func transitOf(now, last receive.Stats, seen bool) *float64 {
	if !seen || now.TransitFrames <= last.TransitFrames || now.Transit < last.Transit {
		return nil
	}
	mean := float64(now.Transit-last.Transit) / float64(now.TransitFrames-last.TransitFrames)
	return msOfPtr(time.Duration(mean))
}

// presentOf is what the sink still holds a frame for after it arrives, which is the pipeline's
// latency window less the work already done inside it.
//
// nil where either half is missing, and never negative: a transit past the window is a frame the
// sink drops for being late rather than one it draws early, and the drop is counted where drops are
// counted.
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

// watchLinkOf is the delivery window this leg's transport settled on, off the counters that leg's
// own elements keep.
// nil where no element of the pipeline states one.
func watchLinkOf(groups []receive.StatGroup) *float64 {
	for _, group := range groups {
		key, stated := watchLinkSources[group.Factory]
		if !stated {
			continue
		}
		for _, value := range group.Values {
			if value.Key == key {
				window := value.Value
				return &window
			}
		}
	}
	return nil
}

// totalOf adds the stages that carry a figure, and is nil where none does.
//
// A floor rather than a sum of the path: the relay's own share is in neither this list nor any
// measurement, and a publisher on another machine puts the first two stages out of reach too.
// Adding what is there is still the useful answer, because every absent stage only makes the real
// delay larger.
func totalOf(budget wire.DelayBudget) *float64 {
	stages := []*float64{
		budget.Publish, budget.PublishLink, budget.WatchLink, budget.Receive, budget.Present,
	}

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
