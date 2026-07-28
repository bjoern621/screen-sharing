package roster

import (
	"bytes"
	"testing"
)

// The literals are the wire format itself, which the app's watch package parses against the same strings.
// Comparing the whole line is the point:
// a renamed field or a dropped discriminator is a line the other half no longer reads.
func TestSenderWritesAWatchLegLine(t *testing.T) {
	var buf bytes.Buffer
	Sender(&buf)(Request{Stream: "alice", Transport: "rtsp", Options: map[string]string{"rtspWatchLatencyMs": "400"}})

	want := `{"type":"watch-leg","stream":"alice","transport":"rtsp","options":{"rtspWatchLatencyMs":"400"}}` + "\n"
	if buf.String() != want {
		t.Errorf("sent %s, want %s", buf.String(), want)
	}
}

func TestReporterWritesAWatchSetLine(t *testing.T) {
	var buf bytes.Buffer
	Reporter(&buf)(Status{Watching: []string{"alice", "bob"}})

	want := `{"type":"watch-set","watching":["alice","bob"]}` + "\n"
	if buf.String() != want {
		t.Errorf("sent %s, want %s", buf.String(), want)
	}
}

// Watching nothing is a set the app can read, not a missing field.
func TestReporterWritesAnEmptySetAsAList(t *testing.T) {
	var buf bytes.Buffer
	Reporter(&buf)(Status{})

	want := `{"type":"watch-set","watching":[]}` + "\n"
	if buf.String() != want {
		t.Errorf("sent %s, want %s", buf.String(), want)
	}
}
