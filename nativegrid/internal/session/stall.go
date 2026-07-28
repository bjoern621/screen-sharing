package session

import (
	"time"

	"bjoernblessin.de/go-utils/util/logger"
)

// sweepInterval is how often the watched players are read, and stallSweeps how
// many readings in a row have to repeat the same frame count before the stream
// counts as stalled.
// Their product is the delay before a frozen tile says so, which is long enough
// to sit out a source that sends slower than the sweep.
const (
	sweepInterval = 2 * time.Second
	stallSweeps   = 2
)

// Stalled reports whether stream i is live but no longer receiving frames.
//
// A receive pipeline that stops getting data neither errors nor ends: it keeps
// the last picture on the paintable.
// The frame count is the only thing that says the stream is gone.
func (s *Session) Stalled(i int) bool { return s.at(i).stalled }

// startSweep arms the next stall sweep, unless one is already due.
// It is called whenever a player starts, and the sweep re-arms itself while
// anything is watched, so a window with no tiles open holds no timer.
func (s *Session) startSweep() {
	if s.sweepEvery <= 0 || s.stallSweep != nil {
		return
	}
	s.stallSweep = time.AfterFunc(s.sweepEvery, func() { s.dispatch(s.tick) })
}

// stopSweep drops the pending sweep.
func (s *Session) stopSweep() {
	if s.stallSweep == nil {
		return
	}
	s.stallSweep.Stop()
	s.stallSweep = nil
}

// tick is one armed sweep, on the UI loop.
func (s *Session) tick() {
	s.stallSweep = nil
	s.sweep()
	if s.Watching() > 0 {
		s.startSweep()
	}
}

// sweep reads every watched player's frame count and marks the streams whose
// count stopped moving.
//
// It polls rather than being told, because nothing in the pipeline reports the
// absence of data.
// Only a Live stream can stall: a loading or reconnecting one has no frames to
// miss.
func (s *Session) sweep() {
	for i := range s.entries {
		e := &s.entries[i]
		if e.state != Live || e.player == nil {
			s.clearStall(i)
			continue
		}
		frames := e.player.Stats().Frames
		if frames != e.frames {
			e.frames = frames
			e.still = 0
			s.setStalled(i, false)
			continue
		}
		e.still++
		s.setStalled(i, e.still >= stallSweeps)
	}
}

// clearStall takes stream i out of the stalled state and forgets its frame
// count, which is what a restart or an unwatch leaves behind.
func (s *Session) clearStall(i int) {
	e := s.at(i)
	e.frames = 0
	e.still = 0
	s.setStalled(i, false)
}

// setStalled notifies only on a change, so sweeping a stalled tile costs nothing
// until it moves again.
func (s *Session) setStalled(i int, stalled bool) {
	e := s.at(i)
	if e.stalled == stalled {
		return
	}
	e.stalled = stalled
	if stalled {
		logger.Warnf("%q stopped receiving frames", e.stream.Name)
	} else {
		logger.Infof("%q is receiving frames again", e.stream.Name)
	}
	s.notify(Change{Kind: StallChanged, Index: i})
}
