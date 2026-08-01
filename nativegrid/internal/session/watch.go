package session

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// SetWatched opens or closes stream i's tile.
//
// On: a player starts and the stream goes Loading, and the player's callbacks
// take it Live or Failed.
// Off: the player stops and the stream goes Idle.
//
// A stream already in the state asked for is left as it is, whatever it is doing
// there: Failed and Reconnecting are watched states, and replacing the player
// behind one of them is Restart's job rather than a side effect of asking for
// what already holds.
//
// This is the path a click takes, so it is also where the reconnect budget
// refills.
// A pending automatic retry is dropped for the same reason: what was asked for
// replaces what the model had planned.
func (s *Session) SetWatched(i int, on bool) {
	if s.at(i).state.Watched() == on {
		return
	}
	s.cancelRetry(i)
	s.at(i).attempts = 0
	if on {
		s.start(i)
		return
	}
	s.stop(i)
}

// Restart re-opens stream i on the fragment the roster holds for it now. It is
// the tile's retry button: a watch someone asks for again is a fresh one, so the
// reconnect budget refills with it.
func (s *Session) Restart(i int) {
	assert.Assert(s.at(i).state.Watched(), "only a watched stream is restarted", s.at(i).stream.Name)

	s.at(i).attempts = 0
	s.restart(i)
}

// restart replaces stream i's player, keeping the reconnect budget it has spent.
//
// The budget stays because the roster restarts a stream on every flap of the
// relay listing it. Refilling it there is a window that reconnects to a stream
// nobody publishes for as long as the relay keeps naming it, which is the cap
// the budget exists to put on.
func (s *Session) restart(i int) {
	s.cancelRetry(i)
	s.start(i)
}

// start opens a player for stream i, replacing a running one.
// The generation is bumped first, so the callbacks and the retry of the player
// left behind are dropped.
func (s *Session) start(i int) {
	e := s.at(i)
	nextGen(e)
	if e.player != nil {
		e.player.Stop()
		e.player = nil
	}
	e.message = ""
	e.audio = false
	e.state = Loading
	s.clearStall(i)

	// The render chain is the window's choice and no stream's, so it travels beside
	// the stream rather than in it. It is read back on every open rather than kept
	// with the player: a chain is fixed when the pipeline is parsed, so this call is
	// the only place a change to it can take effect.
	p, err := s.factory(e.stream, player.Open{Chain: s.RenderChain(i)}, s.events(i, e.gen))
	// The factory is the caller's, so the entry is read again rather than held
	// across it: a stream added in between moves the slice it points into.
	e = s.at(i)
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
	nextGen(e)
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

// nextGen retires the work of the player the stream is running: a callback and a
// retry carry the generation they were made in, and land nowhere once it is not
// the stream's any more. The counter only counts up, because a wrapped one would
// make a stale report equal the generation it is compared against.
func nextGen(e *entry) {
	e.gen++
	assert.Assert(e.gen != 0, "a stream generation counts up", e.gen)
}

// events are the callbacks of the player started for stream i in generation gen.
func (s *Session) events(i int, gen uint) player.Events {
	return player.Events{
		OnLive: s.hop(i, gen, func() {
			e := s.at(i)
			e.state = Live
			// The stream carries frames again, so the reconnects it took to get
			// here are done with and the next outage starts from a full budget.
			e.attempts = 0
			s.notify(Change{Kind: StateChanged, Index: i})
		}),
		OnAudio: s.hop(i, gen, func() {
			s.at(i).audio = true
			s.notify(Change{Kind: AudioReady, Index: i})
		}),
		OnEnd: func(message string) {
			s.hop(i, gen, func() {
				s.ended(i, gen, message)
			})()
		},
	}
}

// hop wraps a model change so it lands on the UI loop and only applies while the
// generation it was made for is still the stream's.
//
// The entry is read through at rather than handed to apply: a stream joining the
// model moves the whole slice, and a pointer taken before it would write into
// the array nothing reads any more.
func (s *Session) hop(i int, gen uint, apply func()) func() {
	return func() {
		s.dispatch(func() {
			if s.at(i).gen != gen {
				logger.Debugf("dropped a stale player report for %q", s.at(i).stream.Name)
				return
			}
			apply()
		})
	}
}
