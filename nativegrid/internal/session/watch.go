package session

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// SetWatched toggles watching stream i.
//
// On: a player starts and the stream goes Loading, and the player's callbacks
// take it Live or Failed.
// On with a player already running restarts it, which is also the retry button's
// path.
// Off: the player stops and the stream goes Idle.
//
// This is the path a click takes, so it is also where the reconnect budget
// refills.
// A pending automatic retry is dropped for the same reason: what was asked for
// replaces what the model had planned.
func (s *Session) SetWatched(i int, on bool) {
	s.cancelRetry(i)
	s.at(i).attempts = 0
	if on {
		s.start(i)
		return
	}
	s.stop(i)
}

// start opens a player for stream i, replacing a running one.
// The generation is bumped first, so the callbacks and the retry of the player
// left behind are dropped.
func (s *Session) start(i int) {
	e := s.at(i)
	e.gen++
	if e.player != nil {
		e.player.Stop()
		e.player = nil
	}
	e.message = ""
	e.audio = false
	e.state = Loading
	s.clearStall(i)

	p, err := s.factory(e.stream, s.events(i, e.gen))
	if err != nil {
		// A factory failure is a missing element or a fragment the parser
		// rejects, which the same fragment fails on again.
		// Nothing is scheduled for it: the tile's retry is the way out.
		e.state = Failed
		e.message = err.Error()
		logger.Warnf("watching %q failed: %v", e.stream.Name, err)
	} else {
		assert.IsNotNil(p, "a factory yields a player when it yields no error", e.stream.Name)
		e.player = p
		s.startSweep()
	}
	s.notify(Change{Kind: StateChanged, Index: i})
	s.persist.Schedule()
	s.watchSet.Schedule()
}

// stop closes stream i's tile: the player goes, the spotlight goes with it if
// it pointed here, and the generation bump drops whatever the player still
// reports.
func (s *Session) stop(i int) {
	e := s.at(i)
	e.gen++
	if e.player != nil {
		e.player.Stop()
		e.player = nil
	}
	e.message = ""
	e.audio = false
	e.state = Idle
	s.clearStall(i)
	if s.spot == i {
		s.spot = noSpot
	}
	logger.Infof("stopped watching %q", e.stream.Name)
	s.notify(Change{Kind: StateChanged, Index: i})
	s.persist.Schedule()
	s.watchSet.Schedule()
}

// RequestWatchLeg asks the app to receive stream i over a transport, with that
// transport's knobs at the given values, keyed as the roster declared them.
//
// Nothing changes here: the app decides what the leg means, and its answer is
// the roster push that carries the new source fragment, which SetRoster
// restarts a watched stream on. A refused request is answered too, with the
// values that still hold, so the controls that asked follow the model back.
func (s *Session) RequestWatchLeg(i int, transport string, options map[string]string) {
	e := s.at(i)
	logger.Infof("asking to watch %q over %s", e.stream.Name, transport)
	s.send(roster.Request{Stream: e.stream.Name, Transport: transport, Options: options})
}

// events are the callbacks of the player started for stream i in generation gen.
func (s *Session) events(i int, gen uint) player.Events {
	return player.Events{
		OnLive: s.hop(i, gen, func(e *entry) {
			e.state = Live
			// The stream carries frames again, so the reconnects it took to get
			// here are done with and the next outage starts from a full budget.
			e.attempts = 0
			s.notify(Change{Kind: StateChanged, Index: i})
		}),
		OnAudio: s.hop(i, gen, func(e *entry) {
			e.audio = true
			s.notify(Change{Kind: AudioReady, Index: i})
		}),
		OnEnd: func(message string) {
			s.hop(i, gen, func(*entry) {
				s.ended(i, gen, message)
			})()
		},
	}
}

// hop wraps a model change so it lands on the UI loop and only applies while the
// generation it was made for is still the stream's.
func (s *Session) hop(i int, gen uint, apply func(e *entry)) func() {
	return func() {
		s.dispatch(func() {
			e := s.at(i)
			if e.gen != gen {
				logger.Debugf("dropped a stale player report for %q", e.stream.Name)
				return
			}
			apply(e)
		})
	}
}
