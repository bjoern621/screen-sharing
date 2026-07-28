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
// take it Live or Failed. On with a player already running restarts it, which is
// also the retry path. Off: the player stops and the stream goes Idle. Either way
// the generation is bumped first, so the callbacks of the player left behind are
// dropped.
func (s *Session) SetWatched(i int, on bool) {
	e := s.at(i)
	e.gen++
	if e.player != nil {
		e.player.Stop()
		e.player = nil
	}
	e.message = ""
	e.audio = false

	if !on {
		e.state = Idle
		if s.spot == i {
			s.spot = noSpot
		}
		logger.Infof("stopped watching %q", e.stream.Name)
		s.notify(Change{Kind: StateChanged, Index: i})
		s.persist.Schedule()
		return
	}

	e.state = Loading
	p, err := s.factory(e.stream, s.events(i, e.gen))
	if err != nil {
		e.state = Failed
		e.message = err.Error()
		logger.Warnf("watching %q failed: %v", e.stream.Name, err)
	} else {
		assert.IsNotNil(p, "a factory yields a player when it yields no error", e.stream.Name)
		e.player = p
	}
	s.notify(Change{Kind: StateChanged, Index: i})
	s.persist.Schedule()
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
		OnLive: s.hop(i, gen, func(e *entry) Change {
			e.state = Live
			return Change{Kind: StateChanged, Index: i}
		}),
		OnAudio: s.hop(i, gen, func(e *entry) Change {
			e.audio = true
			return Change{Kind: AudioReady, Index: i}
		}),
		OnEnd: func(message string) {
			s.hop(i, gen, func(e *entry) Change {
				e.state = Failed
				e.message = message
				return Change{Kind: StateChanged, Index: i}
			})()
		},
	}
}

// hop wraps a model change so it lands on the UI loop and only applies while the
// generation it was made for is still the stream's. The change it returns is the
// one the views are notified of.
func (s *Session) hop(i int, gen uint, apply func(e *entry) Change) func() {
	return func() {
		s.dispatch(func() {
			e := s.at(i)
			if e.gen != gen {
				logger.Debugf("dropped a stale player report for %q", e.stream.Name)
				return
			}
			s.notify(apply(e))
		})
	}
}
