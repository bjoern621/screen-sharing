// Package session is the grid's model: which streams the window knows, which of
// them are watched, in what order, and which one is spotlit.
//
// It owns the players behind the watched streams and the arrangement carried
// across runs, and holds no widget. Views subscribe as Observers and read the
// model back, so the tile area and the sidebar cannot drift apart: there is one
// place a watch state is decided and one place it is remembered.
package session

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// noSpot is Session.spot while the grid shows no spotlit stream.
const noSpot = -1

// entry is everything the model knows about one stream.
type entry struct {
	stream  roster.Stream
	state   State
	message string // the failure message behind Failed, "" otherwise
	// gen counts the SetWatched calls this stream has seen. A player callback
	// carries the generation it was started for, so a report from a player an
	// unwatch or a retry has already replaced lands nowhere.
	gen     uint
	player  player.Player
	audio   bool // the stream turned out to carry audio
	present bool // the latest roster push still lists the stream
}

// Session is the model. Every method runs on the UI loop: the model is not
// guarded, and the player callbacks that arrive on pipeline threads hop there
// through the dispatch it was built with.
type Session struct {
	entries []entry
	// order is the display order as stream indexes, over all streams so an
	// unwatched stream keeps its place.
	order     []int
	spot      int
	factory   player.Factory
	store     layout.Store
	dispatch  idle.Dispatch
	observers []Observer
	// persist coalesces the writes of a burst of changes into one.
	persist *idle.Coalescer
	// savedOrder is the display order of the state file, by stream name: the
	// ranking a stream is placed by when it turns up (placeInOrder), and the list
	// the current order is folded back into.
	savedOrder []string
	// wantWatched holds the streams the state file had watched and this run has
	// not restored yet, so a stream that only appears in a later roster push is
	// still watched when it does.
	wantWatched map[string]bool
	// wantSpot is the state file's spotlit stream, cleared once restored.
	wantSpot string
}

// New builds the model for the streams the window opens with. The remembered
// arrangement is read here and applied by Restore, once the views are in place.
func New(streams []roster.Stream, factory player.Factory, store layout.Store, dispatch idle.Dispatch) *Session {
	assert.IsNotNil(factory, "a session decodes through a player factory")
	assert.IsNotNil(store, "a session remembers its arrangement in a store")
	assert.IsNotNil(dispatch, "a session hops player callbacks to the UI loop")

	saved := store.Load()
	s := &Session{
		spot:        noSpot,
		factory:     factory,
		store:       store,
		dispatch:    dispatch,
		savedOrder:  saved.Order,
		wantWatched: make(map[string]bool, len(saved.Watched)),
		wantSpot:    saved.Spot,
	}
	s.persist = idle.New(dispatch, s.write)
	for _, n := range saved.Watched {
		s.wantWatched[n] = true
	}
	for _, st := range streams {
		s.add(st)
	}
	logger.Infof("session opened with %d streams, %d remembered as watched", len(streams), len(s.wantWatched))
	return s
}

// Observe adds a view. A view added after the model already holds streams reads
// them back itself; only later changes reach it.
func (s *Session) Observe(o Observer) {
	assert.IsNotNil(o, "a view observes as a non-nil observer")

	s.observers = append(s.observers, o)
}

// Len is the number of streams the model knows, which is also the range of a
// valid stream index.
func (s *Session) Len() int { return len(s.entries) }

func (s *Session) Stream(i int) roster.Stream { return s.at(i).stream }

func (s *Session) State(i int) State { return s.at(i).state }

// Message is the failure message of a stream in the Failed state, "" otherwise.
func (s *Session) Message(i int) string { return s.at(i).message }

// Player is the stream's running player, nil while none runs. It is read rather
// than handed out, so a view never holds one across a retry.
func (s *Session) Player(i int) player.Player { return s.at(i).player }

// HasAudio reports whether the stream turned out to carry audio.
func (s *Session) HasAudio(i int) bool { return s.at(i).audio }

// Present reports whether the latest roster push still lists the stream.
func (s *Session) Present(i int) bool { return s.at(i).present }

// Visible reports whether the stream belongs on screen: the roster still lists
// it, or a tile is open on it. A vanished stream stays visible while watched, so
// its failure state stays on screen instead of silently disappearing; unwatching
// it then takes it away.
func (s *Session) Visible(i int) bool {
	e := s.at(i)
	return e.present || e.state.Watched()
}

// Watching counts the streams with a tile open, which the sidebar states beside
// its heading.
func (s *Session) Watching() int {
	n := 0
	for i := range s.entries {
		if s.entries[i].state.Watched() {
			n++
		}
	}
	return n
}

// Spot is the spotlit stream's index, or -1 while the grid shows every tile. A
// spotlit stream is always a watched one: unwatching the spotlit stream drops the
// spotlight with it.
func (s *Session) Spot() int {
	assert.Assert(s.spot == noSpot || s.at(s.spot).state.Watched(), "a spotlit stream is watched", s.spot)

	return s.spot
}

// ToggleSpot spotlights stream i, or drops back to the plain grid when i is
// already spotlit.
func (s *Session) ToggleSpot(i int) {
	assert.Assert(s.at(i).state.Watched(), "only a watched stream is spotlit", s.at(i).stream.Name)

	if s.spot == i {
		s.spot = noSpot
	} else {
		s.spot = i
	}
	logger.Debugf("spotlight now %d", s.spot)
	s.notify(Change{Kind: OrderChanged, Index: noStream})
	s.persist.Schedule()
}

// Close stops every running pipeline and writes a pending arrangement. It runs
// after the UI loop returns, so sources say goodbye to the relay instead of dying
// with the process, and the last change reaches the state file even though the
// loop that would have written it is gone.
func (s *Session) Close() {
	for i := range s.entries {
		if p := s.entries[i].player; p != nil {
			p.Stop()
			s.entries[i].player = nil
		}
	}
	if s.persist.Pending() {
		s.write()
	}
	logger.Infof("session closed")
}

// at is the model's one index guard: every accessor goes through it, so a stale
// index from a widget or a player callback fails here instead of corrupting a
// neighbor's state.
func (s *Session) at(i int) *entry {
	assert.Assert(i >= 0 && i < len(s.entries), "stream index in range", i, len(s.entries))

	return &s.entries[i]
}

func (s *Session) notify(c Change) {
	logger.Tracef("session change: %s (%d)", c.Kind, c.Index)
	for _, o := range s.observers {
		o.Changed(c)
	}
}
