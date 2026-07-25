package session

import (
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// SetRoster applies a roster push from the app: the full set of live streams.
//
// Streams match by name. New ones join the display order at their remembered
// place and are watched if the remembered arrangement had them watched, which is
// how a stream that appears late still comes up. A vanished stream keeps its slot
// and drops out of sight through the Visible rule, so a stream that comes back
// finds its place.
func (s *Session) SetRoster(streams []roster.Stream) {
	for i := range s.entries {
		s.entries[i].present = false
	}
	for _, st := range streams {
		i := s.indexOf(st.Name)
		if i < 0 {
			s.applyWanted(s.add(st))
			continue
		}
		e := s.at(i)
		e.present = true
		// An idle stream takes the pushed source fragment: settings may have
		// changed since launch. A watched one keeps the source its player runs on
		// until it is unwatched.
		if !e.state.Watched() {
			e.stream = st
		}
	}
	logger.Debugf("roster applied: %d live of %d known", len(streams), len(s.entries))
	s.notify(Change{Kind: RosterChanged, Index: noStream})
	s.persist.Schedule()
}

// add appends a stream and slots it into the display order, returning its stream
// index. The row a view appends lands at the end of its list while the slot can be
// anywhere, so the order change is not optional here.
func (s *Session) add(st roster.Stream) int {
	s.entries = append(s.entries, entry{stream: st, present: true})
	i := len(s.entries) - 1
	s.placeInOrder(i)
	logger.Debugf("stream %q known as index %d", st.Name, i)

	s.notify(Change{Kind: StreamAdded, Index: i})
	s.notify(Change{Kind: OrderChanged, Index: noStream})
	return i
}

// indexOf is the stream index of a name, or -1 for a stream the model does not
// know.
func (s *Session) indexOf(name string) int {
	for i := range s.entries {
		if s.entries[i].stream.Name == name {
			return i
		}
	}
	return -1
}
