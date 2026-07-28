package session

import (
	"testing"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

func TestRequestWatchLegAsksTheApp(t *testing.T) {
	sess, _, asked := newAskingSession(t, "alice")

	sess.RequestWatchLeg(0, "srt", map[string]string{"srtWatchLatencyMs": "400"})

	if len(asked.sent) != 1 {
		t.Fatalf("sent %d requests, want 1", len(asked.sent))
	}
	got := asked.sent[0]
	if got.Stream != "alice" || got.Transport != "srt" || got.Options["srtWatchLatencyMs"] != "400" {
		t.Errorf("request = %+v, want alice over srt at 400 ms", got)
	}
}

// A move to another leg carries the leg alone. The knobs on the controls are the
// ones the leg being left declares, and a request stating them under the new one
// names knobs that transport does not have, which the app refuses as a whole leg:
// the move would be lost with them.
func TestRequestWatchLegMovesWithoutKnobs(t *testing.T) {
	sess, _, asked := newAskingSession(t, "alice")

	sess.RequestWatchLeg(0, "rtsp", map[string]string{})

	if len(asked.sent) != 1 {
		t.Fatalf("sent %d requests, want 1", len(asked.sent))
	}
	got := asked.sent[0]
	if got.Transport != "rtsp" || len(got.Options) != 0 {
		t.Errorf("request = %+v, want rtsp with no knobs stated", got)
	}
}

// Asking changes nothing here. The leg moves when the app answers with a push,
// which is the only thing that knows what the request meant.
func TestRequestWatchLegLeavesTheModel(t *testing.T) {
	sess, backend, _ := newAskingSession(t, "alice")
	sess.SetWatched(0, true)

	sess.RequestWatchLeg(0, "rtsp", nil)

	if got := sess.Stream(0).Transport; got != "srt" {
		t.Errorf("transport = %q, want the leg the push has not changed yet", got)
	}
	if len(backend.players) != 1 || backend.players[0].stops != 0 {
		t.Error("asking for a leg restarted the player")
	}
}

// A watched stream runs on the fragment its player was started with, so the
// push that carries a new one restarts it.
func TestRosterRestartsAWatchedStreamOnANewFragment(t *testing.T) {
	sess, backend, _ := newAskingSession(t, "alice")
	sess.SetWatched(0, true)

	sess.SetRoster([]roster.Stream{{Name: "alice", Transport: "rtsp", Source: "rtspsrc"}})

	if len(backend.players) != 2 {
		t.Fatalf("started %d players, want the first and its replacement", len(backend.players))
	}
	if backend.players[0].stops != 1 {
		t.Errorf("the player on the old leg stopped %d times, want 1", backend.players[0].stops)
	}
	if got := sess.Stream(0); got.Transport != "rtsp" || got.Source != "rtspsrc" {
		t.Errorf("stream = %+v, want the pushed leg", got)
	}
	if sess.State(0) != Loading {
		t.Errorf("state = %s, want the restart to be connecting", sess.State(0))
	}
}

// A push that repeats the fragment leaves the tile alone: the roster is polled,
// and a stream that did not move must not blink every time it is.
func TestRosterKeepsAWatchedStreamOnAnUnchangedFragment(t *testing.T) {
	sess, backend, _ := newAskingSession(t, "alice")
	sess.SetWatched(0, true)

	sess.SetRoster([]roster.Stream{{Name: "alice", Transport: "srt", Source: "videotestsrc"}})

	if len(backend.players) != 1 {
		t.Fatalf("started %d players, want the one", len(backend.players))
	}
	if backend.players[0].stops != 0 {
		t.Error("an unchanged fragment restarted the player")
	}
}

// An idle stream takes the pushed fragment with no restart to do: nothing is
// running on the old one.
func TestRosterMovesAnIdleStream(t *testing.T) {
	sess, backend, _ := newAskingSession(t, "alice")

	sess.SetRoster([]roster.Stream{{
		Name:       "alice",
		Transport:  "rtsp",
		Source:     "rtspsrc",
		Transports: []string{"rtsp", "srt"},
		Options:    []roster.Option{{Key: "rtspWatchLatencyMs", Kind: roster.OptionInt, Value: "400"}},
	}})

	if len(backend.players) != 0 {
		t.Error("moving an idle stream started a player")
	}
	got := sess.Stream(0)
	if got.Source != "rtspsrc" || len(got.Options) != 1 {
		t.Errorf("stream = %+v, want the pushed leg and its knobs", got)
	}
}
