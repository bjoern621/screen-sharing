package session

import (
	"bjoernblessin.de/go-utils/util/assert"
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
//
// A push is also how a watch leg changes: the app answers a RequestWatchLeg
// with the roster that request produced, and a stream whose source fragment
// moved restarts on it.
func (s *Session) SetRoster(streams []roster.Stream) {
	assertPush(streams)

	// The presence the push replaces, kept by name because a stream coming back
	// is the reason to restart it.
	wasAbsent := make(map[string]bool, len(s.entries))
	for i := range s.entries {
		wasAbsent[s.entries[i].stream.Name] = !s.entries[i].present
		s.entries[i].present = false
	}
	for _, st := range streams {
		i := s.indexOf(st.Name)
		if i < 0 {
			s.applyWanted(s.add(st))
			continue
		}
		// The entry pointer lives to the end of this block and no further: a
		// stream joining the model moves the whole slice, which is why the
		// restarts below name the stream by index.
		e := s.at(i)
		e.present = true
		// A player runs on the fragment it was started with, so a stream that
		// moved to another watch leg only arrives over it after a restart. The
		// app pushes a changed fragment when someone asked for one, which is
		// what the restart answers; an unchanged fragment leaves the tile alone.
		moved := e.stream.Source != st.Source
		// A stream the relay dropped and lists again is the other restart.
		// Whatever took it away killed its pipeline, and the tile would otherwise
		// hold a failure message for a stream that is back on air.
		returned := wasAbsent[st.Name] && e.state == Failed
		e.stream = st
		watched := e.state.Watched()

		switch {
		case watched && moved:
			// The leg is another one, so what the attempts on the old one cost
			// says nothing about this stream: it starts over on a full budget,
			// like a watch someone opened by hand.
			logger.Infof("%q moved to %s, restarting it", st.Name, st.Transport)
			s.Restart(i)
		case watched && returned:
			logger.Infof("%q is listed again, restarting the failed watch", st.Name)
			s.restart(i)
		}
	}
	logger.Debugf("roster applied: %d live of %d known", len(streams), len(s.entries))
	s.notify(Change{Kind: RosterChanged, Index: noStream})
	s.persist.Schedule()
	s.watchSet.Schedule()
}

// add appends a stream and slots it into the display order, returning its stream
// index. The row a view appends lands at the end of its list while the slot can be
// anywhere, so the order change is not optional here.
func (s *Session) add(st roster.Stream) int {
	assertStream(st)
	assert.Assert(s.indexOf(st.Name) < 0, "a stream name names one stream", st.Name)

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

// assertPush holds what a roster push is: one entry per stream, each of them
// something the model can take on. roster.Parse turns a push that breaks either
// into an error at the pipe it arrives on, so one reaching here came from this
// side.
func assertPush(streams []roster.Stream) {
	seen := make(map[string]bool, len(streams))
	for _, st := range streams {
		assertStream(st)
		assert.Assert(!seen[st.Name], "a stream name names one stream", st.Name)

		seen[st.Name] = true
	}
}

// assertStream holds what the model needs of a stream: the name every lookup
// here keys on, and the fragment start hands the player factory.
func assertStream(st roster.Stream) {
	assert.Assert(st.Name != "", "a stream carries a name")
	assert.Assert(st.Source != "", "a stream carries a source fragment", st.Name)
}
