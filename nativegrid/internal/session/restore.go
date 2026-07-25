package session

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
)

// Restore applies the remembered arrangement to the streams the model opened
// with: the watch set, and the spotlight if its stream is part of it.
//
// It runs once the views are subscribed, so a restored watch draws exactly like a
// clicked one. Streams the launch roster does not carry are restored by SetRoster
// when they turn up.
func (s *Session) Restore() {
	for i := range s.entries {
		s.applyWanted(i)
	}
}

// applyWanted watches stream i if the remembered arrangement had it watched, and
// spotlights it if it was the spotlit one. The want is consumed: unwatching the
// stream afterwards must not bring it back on the next roster push.
func (s *Session) applyWanted(i int) {
	name := s.at(i).stream.Name
	if !s.wantWatched[name] {
		return
	}
	delete(s.wantWatched, name)
	logger.Infof("restoring the watch on %q", name)
	s.SetWatched(i, true)
	if s.wantSpot == name {
		s.wantSpot = ""
		s.ToggleSpot(i)
	}
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
