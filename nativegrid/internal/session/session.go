// Package session is the grid's model: which streams the window knows, which of
// them are watched, in what order, and which one is spotlit.
//
// It owns the players behind the watched streams and the arrangement carried
// across runs, and holds no widget. Views subscribe as Observers and read the
// model back, so the tile area and the sidebar cannot drift apart: there is one
// place a watch state is decided and one place it is remembered.
package session

import (
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// noSpot is Session.spot while the grid shows no spotlit stream.
const noSpot = -1

// stopTimeout bounds the whole teardown in Close.
// Every pipeline stops at once, so this is the wait for the slowest one rather
// than the sum: a source that will not say goodbye must not hold the window open.
const stopTimeout = 2 * time.Second

// entry is everything the model knows about one stream.
type entry struct {
	stream  roster.Stream
	state   State
	message string // the pipeline's message behind Failed and Reconnecting, "" otherwise
	// gen counts the starts and stops this stream has seen.
	// A player callback and a scheduled retry carry the generation they were made
	// for, so the work of a player an unwatch or a restart has already replaced
	// lands nowhere.
	gen     uint
	player  player.Player
	audio   bool // the stream turned out to carry audio
	present bool // the latest roster push still lists the stream
	// attempts counts the reconnects since the stream was last opened by hand or
	// last went Live.
	// The backoff is indexed by it, and the attempt budget spent from it.
	attempts int
	// retry is the pending reconnect, nil while none is due.
	// It is kept to be cancelled: the generation drops a retry that fires anyway,
	// but a stopped timer does not wake the process at all.
	retry *time.Timer
	// frames is the rendered frame count of the last sweep, and still the number
	// of sweeps it has not moved for.
	frames  uint64
	still   int
	stalled bool
}

// Session is the model. Every method runs on the UI loop: the model is not
// guarded, and the player callbacks that arrive on pipeline threads hop there
// through the dispatch it was built with.
type Session struct {
	entries []entry
	// order is the display order as stream indexes, over all streams so an
	// unwatched stream keeps its place.
	order   []int
	spot    int
	factory player.Factory
	store   layout.Store
	// send asks the app for a watch leg. It is the model's only way out of the
	// process: what a leg means is the app's business, and the answer arrives as
	// a roster push like any other.
	send roster.Send
	// report states which streams have a tile open, for the app to act on.
	// It goes out whenever that set changes and never says the same thing twice.
	report    roster.Report
	dispatch  idle.Dispatch
	observers []Observer
	// persist coalesces the writes of a burst of changes into one.
	persist *idle.Coalescer
	// watchSet coalesces the reports of a burst of watch changes into one, and
	// reported is the last set that went out.
	watchSet *idle.Coalescer
	reported []string
	// retryDelays is the wait before each reconnect attempt, and its length the
	// attempt budget.
	// It is a field so a test can drive the backoff through the zero delay the
	// after seam takes as "run it here".
	retryDelays []time.Duration
	// stagger spaces the watches Restore opens, so N pipelines do not negotiate at
	// once at launch.
	stagger time.Duration
	// sweepEvery is how often the watched players are read for stall detection.
	// Zero leaves the sweep to the caller, which is how a test drives it with no
	// timer in the model.
	sweepEvery time.Duration
	// stallSweep is the pending sweep, nil while none is due.
	stallSweep *time.Timer
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
func New(streams []roster.Stream, factory player.Factory, store layout.Store, send roster.Send, report roster.Report, dispatch idle.Dispatch) *Session {
	assert.IsNotNil(factory, "a session decodes through a player factory")
	assert.IsNotNil(store, "a session remembers its arrangement in a store")
	assert.IsNotNil(send, "a session asks the app for a watch leg")
	assert.IsNotNil(report, "a session reports the streams it watches")
	assert.IsNotNil(dispatch, "a session hops player callbacks to the UI loop")

	saved := store.Load()
	s := &Session{
		spot:        noSpot,
		factory:     factory,
		store:       store,
		send:        send,
		report:      report,
		dispatch:    dispatch,
		retryDelays: retryBackoff,
		stagger:     restoreStagger,
		sweepEvery:  sweepInterval,
		savedOrder:  saved.Order,
		wantWatched: make(map[string]bool, len(saved.Watched)),
		wantSpot:    saved.Spot,
	}
	s.persist = idle.New(dispatch, s.write)
	s.watchSet = idle.New(dispatch, s.sendWatchSet)
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

// Message is what the pipeline said when it ended, for a stream in the Failed
// or Reconnecting state, and "" otherwise.
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
//
// The pipelines stop concurrently under one timeout.
// Each teardown blocks for as long as its source takes to answer, and a dense
// grid closing one tile at a time is a window that hangs for the sum of them.
func (s *Session) Close() {
	s.stopSweep()

	var wg sync.WaitGroup
	for i := range s.entries {
		s.cancelRetry(i)
		p := s.entries[i].player
		if p == nil {
			continue
		}
		s.entries[i].player = nil
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Stop()
		}()
	}
	stopped := make(chan struct{})
	go func() {
		wg.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(stopTimeout):
		logger.Warnf("pipelines still stopping after %s, closing anyway", stopTimeout)
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
