package watch

import (
	"slices"
	"testing"
)

// The literals are the wire format itself,
// written by the roster package in the nativegrid module, whose own tests assert the same strings.
func TestParseGridMessageWatchLeg(t *testing.T) {
	m, err := ParseGridMessage(`{"type":"watch-leg","stream":"alice","transport":"rtsp","options":{"rtspWatchLatencyMs":"400"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != GridWatchLeg {
		t.Fatalf("kind = %q, want %q", m.Kind, GridWatchLeg)
	}
	if m.Request.Stream != "alice" || m.Request.Transport != "rtsp" {
		t.Errorf("request = %+v, want alice over rtsp", m.Request)
	}
	if got := m.Request.Choice().Options["rtspWatchLatencyMs"]; got != "400" {
		t.Errorf("option = %q, want 400", got)
	}
}

func TestParseGridMessageWatchSet(t *testing.T) {
	m, err := ParseGridMessage(`{"type":"watch-set","watching":["alice","bob"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != GridWatchSet {
		t.Fatalf("kind = %q, want %q", m.Kind, GridWatchSet)
	}
	if want := []string{"alice", "bob"}; !slices.Equal(m.Status.Watching, want) {
		t.Errorf("watching = %v, want %v", m.Status.Watching, want)
	}
}

func TestParseGridMessageReadsAnEmptyWatchSet(t *testing.T) {
	m, err := ParseGridMessage(`{"type":"watch-set","watching":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Status.Watching) != 0 {
		t.Errorf("watching = %v, want nothing", m.Status.Watching)
	}
}

// A grid binary from another build can name a kind this one has no reader for.
// Skipping such a line costs the line and nothing else.
func TestParseGridMessageSkipsAnUnknownKind(t *testing.T) {
	m, err := ParseGridMessage(`{"type":"spotlight","stream":"alice"}`)
	if err != nil {
		t.Fatalf("unknown kind refused: %v", err)
	}
	if m.Kind != "spotlight" {
		t.Errorf("kind = %q, want spotlight", m.Kind)
	}
	if m.Request.Stream != "" {
		t.Errorf("request = %+v, want an empty one", m.Request)
	}
}

// The window logs to stderr and writes nothing but messages here,
// so a line carrying no kind came from somewhere else and moves no stream.
func TestParseGridMessageRefusesAnythingElse(t *testing.T) {
	lines := []string{
		"",
		"gst warning: something",
		"{}",
		`{"stream":"alice","transport":"rtsp"}`,
		`{"type":"watch-leg","transport":"rtsp"}`,
	}
	for _, line := range lines {
		if _, err := ParseGridMessage(line); err == nil {
			t.Errorf("%q was read as a message", line)
		}
	}
}
