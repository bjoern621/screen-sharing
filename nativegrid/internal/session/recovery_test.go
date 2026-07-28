package session

import (
	"slices"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// last is the player the model is running now, which after a reconnect is not
// the one the test started with.
func last(b *stubBackend) *stubPlayer {
	return b.players[len(b.players)-1]
}

// TestEndSchedulesAReconnect covers the window between a pipeline ending and its
// retry: the tile stays open, and the retry is armed rather than taken.
func TestEndSchedulesAReconnect(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	// A delay no test waits out, so the model is read while the retry is pending.
	s.retryDelays = []time.Duration{time.Hour}
	s.SetWatched(0, true)

	backend.players[0].events.OnEnd("relay closed the connection")

	if got := s.State(0); got != Reconnecting {
		t.Fatalf("state after the pipeline ended = %v, want reconnecting", got)
	}
	if !s.State(0).Watched() {
		t.Error("a reconnecting stream lost its tile")
	}
	if s.Message(0) == "" {
		t.Error("a reconnecting stream carries no message")
	}
	if len(backend.players) != 1 {
		t.Errorf("started %d players, want the reconnect to wait out its delay", len(backend.players))
	}
	if s.at(0).retry == nil {
		t.Fatal("no reconnect armed")
	}

	s.SetWatched(0, false)
	if s.at(0).retry != nil {
		t.Error("unwatching left a reconnect armed")
	}
	if got := s.State(0); got != Idle {
		t.Errorf("state after unwatch = %v, want idle", got)
	}
}

// TestReconnectGivesUp spends the whole attempt budget and lands where a stream
// nobody publishes any more belongs.
func TestReconnectGivesUp(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	s.SetWatched(0, true)

	budget := len(s.retryDelays)
	for n := range budget {
		last(backend).events.OnEnd("source gone")
		if got := s.State(0); got != Loading {
			t.Fatalf("state after reconnect %d = %v, want loading", n+1, got)
		}
		if got := len(backend.players); got != n+2 {
			t.Fatalf("started %d players after reconnect %d, want %d", got, n+1, n+2)
		}
	}

	last(backend).events.OnEnd("source gone")
	if got := s.State(0); got != Failed {
		t.Fatalf("state after the budget ran out = %v, want failed", got)
	}
	if got := s.Message(0); !strings.Contains(got, "source gone") || !strings.Contains(got, "3") {
		t.Errorf("message = %q, want the pipeline's message and the attempts it took", got)
	}
	if got := len(backend.players); got != budget+1 {
		t.Errorf("started %d players, want %d: the model kept reconnecting past its budget", got, budget+1)
	}
}

// TestFramesRefillTheReconnectBudget covers the other half of the budget: a
// stream that recovers is not one attempt away from being given up on.
func TestFramesRefillTheReconnectBudget(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	s.SetWatched(0, true)

	last(backend).events.OnEnd("blip")
	last(backend).events.OnEnd("blip")
	last(backend).events.OnLive()
	if got := s.at(0).attempts; got != 0 {
		t.Fatalf("attempts after the stream went live = %d, want 0", got)
	}

	for range len(s.retryDelays) {
		last(backend).events.OnEnd("blip")
	}
	if got := s.State(0); got != Loading {
		t.Errorf("state = %v, want the refilled budget to still be reconnecting", got)
	}
}

// TestManualRetryRefillsTheReconnectBudget: the retry button is a fresh start,
// not the next attempt of the run that gave up.
func TestManualRetryRefillsTheReconnectBudget(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	s.SetWatched(0, true)
	last(backend).events.OnEnd("blip")

	s.SetWatched(0, true)

	if got := s.at(0).attempts; got != 0 {
		t.Errorf("attempts after a manual retry = %d, want 0", got)
	}
}

// TestRosterRestartsAReturnedStream is the relay flap: the stream goes away, its
// pipeline dies, and the push that lists it again takes the tile off its error
// message.
func TestRosterRestartsAReturnedStream(t *testing.T) {
	// No budget, so the pipeline ending is the failure straight away.
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	s.retryDelays = nil
	s.SetWatched(0, true)
	backend.players[0].events.OnEnd("relay restarted")
	if got := s.State(0); got != Failed {
		t.Fatalf("state = %v, want failed", got)
	}

	s.SetRoster(nil)
	if s.Present(0) {
		t.Fatal("the stream the roster dropped is still present")
	}

	s.SetRoster([]roster.Stream{{Name: "a", Source: "videotestsrc"}})
	if got := len(backend.players); got != 2 {
		t.Fatalf("started %d players, want the failed watch restarted on the return", got)
	}
	if got := s.State(0); got != Loading {
		t.Errorf("state after the return = %v, want loading", got)
	}

	// The stream is running again, so the next flap is nothing to restart on.
	s.SetRoster(nil)
	s.SetRoster([]roster.Stream{{Name: "a", Source: "videotestsrc"}})
	if got := len(backend.players); got != 2 {
		t.Errorf("started %d players, want a running watch left alone", got)
	}
}

// TestStallMarksAndClears drives the sweep by hand over a player whose frame
// count stands still and then moves again.
func TestStallMarksAndClears(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	stalls := 0
	s.Observe(ObserverFunc(func(c Change) {
		if c.Kind == StallChanged {
			stalls++
		}
	}))
	s.SetWatched(0, true)
	p := backend.players[0]
	p.frames = 10
	p.events.OnLive()

	s.sweep()
	if s.Stalled(0) {
		t.Fatal("stalled on the first reading, which has nothing to compare against")
	}
	for range stallSweeps - 1 {
		s.sweep()
		if s.Stalled(0) {
			t.Fatal("stalled before the frame count stood still long enough")
		}
	}
	s.sweep()
	if !s.Stalled(0) {
		t.Fatal("a frozen frame count is not reported as a stall")
	}
	if got := s.State(0); got != Live {
		t.Errorf("state = %v, want a stalled stream to stay live", got)
	}

	p.frames = 11
	s.sweep()
	if s.Stalled(0) {
		t.Error("frames arrived and the stream is still marked stalled")
	}
	if stalls != 2 {
		t.Errorf("reported %d stall changes, want one on and one off", stalls)
	}

	// Stall it again: closing the tile has to take the mark with it, whatever the
	// sweep last saw.
	s.sweep()
	s.sweep()
	if !s.Stalled(0) {
		t.Fatal("the stream stalled again and was not marked")
	}
	s.SetWatched(0, false)
	if s.Stalled(0) {
		t.Error("an unwatched stream is still marked stalled")
	}
}

// TestStallSkipsAStreamWithoutFrames: a stream that never went live has no frame
// count to stand still.
func TestStallSkipsAStreamWithoutFrames(t *testing.T) {
	s, _, _ := newTestSession(t, layout.Layout{}, "a")
	s.SetWatched(0, true)

	for range stallSweeps + 1 {
		s.sweep()
	}
	if s.Stalled(0) {
		t.Error("a loading stream is marked stalled")
	}
}

// TestWatchSetReported checks what leaves the process: the whole set, sorted, and
// only when it differs from the last report.
func TestWatchSetReported(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a", "b")
	var reported [][]string
	s.report = func(st roster.Status) { reported = append(reported, st.Watching) }

	s.SetWatched(1, true)
	s.SetWatched(0, true)
	// A push that changes nothing, and a reconnect that keeps the tile open,
	// report the set nobody has to hear about twice.
	s.SetRoster([]roster.Stream{{Name: "a", Source: "videotestsrc"}, {Name: "b", Source: "videotestsrc"}})
	last(backend).events.OnEnd("blip")
	s.SetWatched(1, false)

	want := [][]string{{"b"}, {"a", "b"}, {"a"}}
	if len(reported) != len(want) {
		t.Fatalf("reported %v, want %v", reported, want)
	}
	for n := range want {
		if !slices.Equal(reported[n], want[n]) {
			t.Errorf("report %d = %v, want %v", n, reported[n], want[n])
		}
	}
}

// TestRestoreStaggersTheOpens holds the launch behaviour: the remembered watches
// do not all negotiate at once, and the ones still queued are not opened a second
// time by a push arriving in between.
func TestRestoreStaggersTheOpens(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{Watched: []string{"a", "b", "c"}}, "a", "b", "c")
	// A gap no test waits out, so only the first watch has been opened below.
	s.stagger = time.Hour

	s.Restore()
	if got := len(backend.players); got != 1 {
		t.Fatalf("restore started %d players at once, want the first of them", got)
	}

	s.SetRoster([]roster.Stream{{Name: "a", Source: "videotestsrc"}, {Name: "b", Source: "videotestsrc"}, {Name: "c", Source: "videotestsrc"}})
	if got := len(backend.players); got != 1 {
		t.Errorf("started %d players, want a push inside the stagger to open nothing again", got)
	}
}
