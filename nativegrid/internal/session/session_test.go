package session

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"

	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// stubPlayer stands in for a decode backend: it starts, reports what a test tells
// it to, and counts its stops.
// No pipeline, no widgets.
// frames is what the stall sweep reads, so a test moves it or leaves it standing.
type stubPlayer struct {
	events player.Events
	stops  int
	frames uint64
}

func (p *stubPlayer) Paintable() *gdk.Paintable { return nil }
func (p *stubPlayer) SetVolume(float64)         {}
func (p *stubPlayer) SetMuted(bool)             {}
func (p *stubPlayer) Stats() player.Stats       { return player.Stats{Frames: p.frames} }
func (p *stubPlayer) Stop()                     { p.stops++ }

// stubBackend hands out stubPlayers and keeps them, so a test can fire their
// callbacks after the fact.
type stubBackend struct {
	players []*stubPlayer
	fail    error
}

func (b *stubBackend) factory(_ roster.Stream, ev player.Events) (player.Player, error) {
	if b.fail != nil {
		return nil, b.fail
	}
	p := &stubPlayer{events: ev}
	b.players = append(b.players, p)
	return p, nil
}

// newTestSession builds a model whose UI loop runs inline, so every change and
// every player callback has landed by the time a call returns.
func newTestSession(t *testing.T, l layout.Layout, names ...string) (*Session, *stubBackend, *layout.Memory) {
	t.Helper()

	streams := make([]roster.Stream, 0, len(names))
	for _, n := range names {
		streams = append(streams, roster.Stream{Name: n, Source: "videotestsrc"})
	}
	backend := &stubBackend{}
	store := &layout.Memory{Layout: l}
	s := New(streams, backend.factory, store, roster.Discard, roster.DiscardReport, func(f func()) { f() })
	testTimings(s)
	return s, backend, store
}

// testTimings takes the timers out of a model under test: a zero delay runs the
// deferred work inside the call that scheduled it, and a zero sweep interval
// leaves the stall sweep to the test.
// Nothing the model schedules then runs on a second thread.
// The retry budget is three attempts rather than the shipped one, which is what
// a test spends.
func testTimings(s *Session) {
	s.retryDelays = []time.Duration{0, 0, 0}
	s.stagger = 0
	s.sweepEvery = 0
}

// requests collects what the model asked the app for, in place of the pipe the
// window writes on.
type requests struct{ sent []roster.Request }

func (r *requests) send(req roster.Request) { r.sent = append(r.sent, req) }

// newAskingSession is a model whose requests are collected rather than sent.
func newAskingSession(t *testing.T, names ...string) (*Session, *stubBackend, *requests) {
	t.Helper()

	streams := make([]roster.Stream, 0, len(names))
	for _, n := range names {
		streams = append(streams, roster.Stream{Name: n, Transport: "srt", Source: "videotestsrc"})
	}
	backend := &stubBackend{}
	asked := &requests{}
	sess := New(streams, backend.factory, &layout.Memory{}, asked.send, roster.DiscardReport, func(f func()) { f() })
	testTimings(sess)
	return sess, backend, asked
}

// order names the streams in display order, which is what a view walks.
func order(s *Session) []string {
	var out []string
	for _, i := range s.Order() {
		out = append(out, s.Stream(i).Name)
	}
	return out
}

func TestPlaceInOrder(t *testing.T) {
	cases := []struct {
		name    string
		saved   []string
		streams []string
		want    []string
	}{
		{
			name:    "streams arriving out of order take their remembered slots",
			saved:   []string{"c", "a", "b"},
			streams: []string{"a", "b", "c"},
			want:    []string{"c", "a", "b"},
		},
		{
			name:    "an unremembered stream goes last",
			saved:   []string{"b"},
			streams: []string{"a", "b", "new"},
			want:    []string{"b", "a", "new"},
		},
		{
			name:    "nothing remembered keeps the arrival order",
			saved:   nil,
			streams: []string{"a", "b", "c"},
			want:    []string{"a", "b", "c"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, _ := newTestSession(t, layout.Layout{Order: c.saved}, c.streams...)
			if got := order(s); !slices.Equal(got, c.want) {
				t.Errorf("order = %v, want %v", got, c.want)
			}
		})
	}
}

// TestArrangementSurvivesRestart runs a reorder through the path a drag commits
// on, then opens a second model on what the first one left in the store.
func TestArrangementSurvivesRestart(t *testing.T) {
	first, _, store := newTestSession(t, layout.Layout{}, "a", "b", "c")
	first.SetWatched(0, true)
	first.Move(2, 0)

	if got := store.Layout.Order; !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Fatalf("saved order = %v, want [c a b]", got)
	}
	if got := store.Layout.Watched; !slices.Equal(got, []string{"a"}) {
		t.Errorf("saved watch set = %v, want [a]", got)
	}

	second, backend, _ := newTestSession(t, store.Layout, "a", "b", "c")
	if got := order(second); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Errorf("restored order = %v, want [c a b]", got)
	}
	second.Restore()
	if len(backend.players) != 1 {
		t.Fatalf("restore started %d players, want 1", len(backend.players))
	}
	if got := second.State(second.indexOf("a")); got != Loading {
		t.Errorf("restored stream state = %v, want loading", got)
	}
}

// TestWatchLifecycle walks one stream from idle to live and back, and checks that
// the player left behind is stopped and cannot report any more.
func TestWatchLifecycle(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	var changes []Change
	s.Observe(ObserverFunc(func(c Change) { changes = append(changes, c) }))

	s.SetWatched(0, true)
	if got := s.State(0); got != Loading {
		t.Fatalf("state after watch = %v, want loading", got)
	}
	first := backend.players[0]
	first.events.OnLive()
	if got := s.State(0); got != Live {
		t.Errorf("state after the first frame = %v, want live", got)
	}
	first.events.OnAudio()
	if !s.HasAudio(0) {
		t.Error("audio not reported")
	}

	s.SetWatched(0, false)
	if got := s.State(0); got != Idle {
		t.Errorf("state after unwatch = %v, want idle", got)
	}
	if first.stops != 1 {
		t.Errorf("player stopped %d times, want 1", first.stops)
	}
	if s.Player(0) != nil {
		t.Error("an idle stream keeps a player")
	}

	// The unwatched player is still holding its callbacks; its generation is
	// stale, so the report lands nowhere.
	first.events.OnEnd("late failure")
	if got := s.State(0); got != Idle {
		t.Errorf("a stale report reached the model: state = %v, want idle", got)
	}

	kinds := map[ChangeKind]int{}
	for _, c := range changes {
		kinds[c.Kind]++
	}
	if kinds[AudioReady] != 1 {
		t.Errorf("audio reported %d times, want 1", kinds[AudioReady])
	}
}

// TestFactoryFailureFails puts a stream in the failed state without a player,
// which is the retry path's starting point.
func TestFactoryFailureFails(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a")
	backend.fail = errors.New("no such element")

	s.SetWatched(0, true)
	if got := s.State(0); got != Failed {
		t.Fatalf("state = %v, want failed", got)
	}
	if s.Message(0) == "" {
		t.Error("a failed stream carries no message")
	}
	if s.Player(0) != nil {
		t.Error("a failed factory left a player behind")
	}

	backend.fail = nil
	s.SetWatched(0, true)
	if got := s.State(0); got != Loading {
		t.Errorf("state after retry = %v, want loading", got)
	}
	if s.Message(0) != "" {
		t.Error("a retried stream keeps the old message")
	}
}

// TestRosterKeepsWatchedStreamVisible covers the presence rule: a stream that
// leaves the relay keeps its place, and stays on screen while it is watched.
func TestRosterKeepsWatchedStreamVisible(t *testing.T) {
	s, _, _ := newTestSession(t, layout.Layout{}, "a", "b")
	s.SetWatched(0, true)

	s.SetRoster([]roster.Stream{{Name: "b", Source: "videotestsrc"}})
	if s.Present(0) {
		t.Error("a stream the roster dropped is still present")
	}
	if !s.Visible(0) {
		t.Error("a watched stream went invisible when the roster dropped it")
	}
	s.SetWatched(0, false)
	if s.Visible(0) {
		t.Error("an unwatched absent stream is still visible")
	}
	if got := order(s); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("order = %v, want the absent stream to keep its slot", got)
	}
}

// TestRosterRestoresLateStream watches a stream that only the second roster push
// carries, which is the case the remembered watch set is kept around for.
func TestRosterRestoresLateStream(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{Watched: []string{"late"}}, "a")
	s.Restore()
	if len(backend.players) != 0 {
		t.Fatalf("restore started %d players before the stream appeared", len(backend.players))
	}

	s.SetRoster([]roster.Stream{{Name: "a"}, {Name: "late", Source: "videotestsrc"}})
	i := s.indexOf("late")
	if i < 0 {
		t.Fatal("the pushed stream is unknown")
	}
	if got := s.State(i); got != Loading {
		t.Errorf("state of the late stream = %v, want loading", got)
	}

	// The want is consumed: unwatching must survive the next push.
	s.SetWatched(i, false)
	s.SetRoster([]roster.Stream{{Name: "a"}, {Name: "late", Source: "videotestsrc"}})
	if got := s.State(i); got != Idle {
		t.Errorf("state after unwatch and a push = %v, want idle", got)
	}
}

// TestSpotlightFollowsWatchSet checks the invariant Spot reads back: the
// spotlight never points at a stream nobody watches.
func TestSpotlightFollowsWatchSet(t *testing.T) {
	s, _, _ := newTestSession(t, layout.Layout{}, "a", "b")
	s.SetWatched(1, true)
	s.ToggleSpot(1)
	if s.Spot() != 1 {
		t.Fatalf("spot = %d, want 1", s.Spot())
	}
	s.SetWatched(1, false)
	if s.Spot() != -1 {
		t.Errorf("spot = %d, want none", s.Spot())
	}
}
