package session

import (
	"testing"

	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// newRenderSession builds a model on a store the caller holds, which is how a second
// run opens on exactly what the first one wrote.
func newRenderSession(t *testing.T, store *layout.Memory, names ...string) (*Session, *stubBackend) {
	t.Helper()

	streams := make([]roster.Stream, 0, len(names))
	for _, n := range names {
		streams = append(streams, roster.Stream{Name: n, Source: "videotestsrc"})
	}
	backend := &stubBackend{}
	s := New(roster.Config{Streams: streams}, backend.factory, backend.chains, store,
		roster.Discard, roster.DiscardReport, roster.DiscardCommand, func(f func()) { f() })
	testTimings(s)
	return s, backend
}

// TestRenderChainReadsThrough holds what the two writers leave behind: a stream reads
// its own chain where it has one and the window's default where it has not, and the
// default it follows moves under it.
func TestRenderChainReadsThrough(t *testing.T) {
	s, _, _ := newTestSession(t, layout.Layout{}, "a", "b")

	// Nothing chosen is the backend's own default, which it marks on the offer. The
	// name is read off that mark rather than written here, so this covers the reading
	// and not the stub's choice of which row to mark.
	def := s.DefaultRenderChain()
	if got := s.RenderChain(0); got != def {
		t.Errorf("RenderChain of an unchosen stream = %q, want the backend's default %q", got, def)
	}
	if got := s.RenderOverride(0); got != "" {
		t.Errorf("RenderOverride of an unchosen stream = %q, want none", got)
	}

	// A pin has to be a chain the default is not, or the pin and the default agreeing
	// would hide a pin that never landed.
	other := "cpu"
	if def == other {
		other = "gl"
	}
	s.SetRenderChain(0, other)
	if got := s.RenderChain(0); got != other {
		t.Errorf("RenderChain after a pin = %q, want %q", got, other)
	}
	if got := s.RenderChain(1); got != def {
		t.Errorf("a pin on one stream reached another: RenderChain = %q, want %q", got, def)
	}

	s.SetDefaultRenderChain(other)
	if got := s.RenderChain(1); got != other {
		t.Errorf("RenderChain after the default moved = %q, want %q", got, other)
	}

	// Clearing the pin hands the stream back to the default, wherever that now is.
	s.SetDefaultRenderChain(def)
	s.SetRenderChain(0, "")
	if got := s.RenderChain(0); got != def {
		t.Errorf("RenderChain after the pin was cleared = %q, want the default %q", got, def)
	}
}

// TestRenderChainRestartsWatchedStream holds the reason a chain change is not a
// setting that takes effect later: a chain is fixed when the pipeline is parsed, so
// the watched stream restarts on it and the new player opens on the new chain.
// Asking for the chain a stream already renders through moves nothing.
func TestRenderChainRestartsWatchedStream(t *testing.T) {
	s, backend, _ := newTestSession(t, layout.Layout{}, "a", "b")
	s.SetWatched(0, true)
	if len(backend.players) != 1 {
		t.Fatalf("watching started %d players, want 1", len(backend.players))
	}
	first := backend.players[0]

	// A chain the stream is not already on, since asking for the one it renders through
	// moves nothing and would read as a restart that did not happen.
	pin := "cpu"
	if s.RenderChain(0) == pin {
		pin = "gl"
	}
	s.SetRenderChain(0, pin)
	if len(backend.players) != 2 {
		t.Fatalf("a chain change started %d players in all, want a restart", len(backend.players))
	}
	if first.stops != 1 {
		t.Errorf("the player on the old chain stopped %d times, want 1", first.stops)
	}

	s.SetRenderChain(0, pin)
	if len(backend.players) != 2 {
		t.Errorf("the same chain twice started %d players, want the running one left alone", len(backend.players))
	}

	// An unwatched stream has no pipeline to replace, so its chain is only remembered.
	s.SetRenderChain(1, pin)
	if len(backend.players) != 2 {
		t.Errorf("a chain change on an unwatched stream started a player")
	}
}

// TestRenderChainSurvivesRestart runs the choice through the store and opens a second
// model on it, including the entry of a stream that run does not carry: it is the
// choice of a machine that is away, and the run it comes back in is the one that
// needs it.
func TestRenderChainSurvivesRestart(t *testing.T) {
	store := &layout.Memory{}
	first, _ := newRenderSession(t, store, "a", "b")
	first.SetDefaultRenderChain("gl")
	first.SetRenderChain(1, "cpu")

	if got := store.Render.Chain; got != "gl" {
		t.Fatalf("saved default = %q, want gl", got)
	}
	if got := store.Render.Streams["b"]; got != "cpu" {
		t.Fatalf("saved chain of b = %q, want cpu", got)
	}

	// A name no build offers is a hand-edited file to survive, not a bug on this side.
	store.Render.Streams["gone"] = "cpu"
	store.Render.Streams["a"] = "quantum"

	second, _ := newRenderSession(t, store, "a", "b")
	if got := second.DefaultRenderChain(); got != "gl" {
		t.Errorf("restored default = %q, want gl", got)
	}
	if got := second.RenderChain(second.indexOf("b")); got != "cpu" {
		t.Errorf("restored chain of b = %q, want cpu", got)
	}
	if got := second.RenderOverride(second.indexOf("a")); got != "" {
		t.Errorf("a chain this build does not offer was kept as %q, want it dropped", got)
	}

	second.SetRenderChain(second.indexOf("b"), "gl")
	if got := store.Render.Streams["gone"]; got != "cpu" {
		t.Errorf("the chain of an absent stream was written as %q, want it kept", got)
	}
}
