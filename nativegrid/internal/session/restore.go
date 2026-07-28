package session

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
)

// restoreStagger spaces the watches Restore opens.
// Every remembered stream starting at once puts N pipelines through connection
// setup and decoder autoplugging in the same instant, on one machine, and the
// tiles come up slower together than they do one after the other.
const restoreStagger = 300 * time.Millisecond

// Restore applies the remembered arrangement to the streams the model opened
// with: the watch set, and the spotlight if its stream is part of it.
//
// It runs once the views are subscribed, so a restored watch draws exactly like a
// clicked one. Streams the launch roster does not carry are restored by SetRoster
// when they turn up, one at a time and with nothing to stagger.
func (s *Session) Restore() {
	opened := 0
	for i := range s.entries {
		if !s.wantWatched[s.at(i).stream.Name] {
			continue
		}
		s.openWanted(i, time.Duration(opened)*s.stagger)
		opened++
	}
}

// applyWanted watches stream i right away if the remembered arrangement had it
// watched.
func (s *Session) applyWanted(i int) {
	if !s.wantWatched[s.at(i).stream.Name] {
		return
	}
	s.openWanted(i, 0)
}

// openWanted watches stream i once d has passed, and spotlights it if it was the
// spotlit one.
//
// The want is consumed here rather than in the deferred half: unwatching the
// stream afterwards must not bring it back on the next roster push, and a push
// arriving inside the stagger must not open the same stream a second time.
// The generation guard is what makes the delay safe: a stream someone watched or
// unwatched while the stagger ran is left the way they left it.
func (s *Session) openWanted(i int, d time.Duration) {
	e := s.at(i)
	name, gen := e.stream.Name, e.gen
	delete(s.wantWatched, name)
	spot := s.wantSpot == name
	if spot {
		s.wantSpot = ""
	}
	logger.Infof("restoring the watch on %q", name)
	s.after(d, func() {
		if s.at(i).gen != gen {
			logger.Debugf("dropped the restored watch on %q, it was acted on", name)
			return
		}
		s.SetWatched(i, true)
		if spot {
			s.ToggleSpot(i)
		}
	})
}

// write records what the window shows now, so the next run opens on it. The order
// is merged with the remembered one, keeping streams this run never saw. It is
// called from the coalescer after every change a restart should reproduce; a lost
// write costs the last change and nothing more.
func (s *Session) write() {
	assert.Assert(len(s.order) == len(s.entries), "the display order covers every stream", len(s.order), len(s.entries))

	var l layout.Layout
	shown := make([]string, 0, len(s.order))
	for _, i := range s.order {
		e := s.at(i)
		shown = append(shown, e.stream.Name)
		if e.state.Watched() {
			l.Watched = append(l.Watched, e.stream.Name)
		}
	}
	if s.spot != noSpot {
		l.Spot = s.at(s.spot).stream.Name
	}
	s.savedOrder = layout.MergeOrder(s.savedOrder, shown)
	l.Order = s.savedOrder

	s.store.Save(l)
	logger.Debugf("arrangement written: %d watched of %d, spotlight %q", len(l.Watched), len(shown), l.Spot)
}
