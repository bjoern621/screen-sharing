package gstrun

import (
	"bjoernblessin.de/screenshare/internal/pointer"
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// The child reports the pointer on the stream it reports its caps on, under a prefix of its own,
// and the parent reads it back with the parser that lives beside the writer.
// These hold that the two are one format: a line written here and parsed elsewhere drifts the first
// time a field is added.

// A run asked for the pointer reports positions while it plays, at its own rate rather than the
// pipeline's.
func TestARunReportsWhereThePointerIs(t *testing.T) {
	var out bytes.Buffer

	// Long enough to outlast several reader ticks, short enough that the test is not a wait.
	// The cancelled context is what stops the reporting and the run.
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	err := RunWithOptions(ctx,
		"videotestsrc ! video/x-raw,framerate=30/1 ! fakesink",
		Options{Pointer: true}, &out)
	if err != nil {
		t.Fatalf("running: %v", err)
	}

	positions := 0
	for _, line := range strings.Split(out.String(), "\n") {
		if _, ok := ParsePointer(line); ok {
			positions++
		}
	}
	if positions == 0 {
		if !hasDisplay() {
			t.Skip("no display server on this session, so there is no position to report")
		}
		t.Errorf("a run asked for the pointer reported none: %q", out.String())
	}
}

// A run that did not ask reports none.
// The other cursor modes draw the pointer into the frames or leave it out, so a child reporting
// anyway writes to a parent with nowhere to put it.
func TestARunThatDidNotAskReportsNoPointer(t *testing.T) {
	var out bytes.Buffer

	err := RunWithOptions(t.Context(),
		"videotestsrc num-buffers=5 ! fakesink", Options{}, &out)
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if _, ok := ParsePointer(line); ok {
			t.Errorf("a run that asked for no pointer reported %q", line)
		}
	}
}

// The parser reads what the writer writes, and reads nothing else as a position: a caps line,
// a meter line and a truncated or mistyped field are all not one.
func TestThePointerLineIsOneFormat(t *testing.T) {
	for _, line := range []string{
		"",
		"progressreport0 (00:00:02): 120 buffers",
		CapsPrefix + "video/x-raw",
		PointerPrefix + "12 34",
		PointerPrefix + "twelve 34 5 true",
		PointerPrefix + "12 34 5 maybe",
	} {
		if _, ok := ParsePointer(line); ok {
			t.Errorf("%q reads as a position, and it is not one", line)
		}
	}

	p, ok := ParsePointer(PointerPrefix + "12 34 1700000000000000000 true")
	if !ok {
		t.Fatal("a line the writer's own format produced does not parse")
	}
	if p.X != 12 || p.Y != 34 || !p.Visible {
		t.Errorf("the line parsed as %+v", p)
	}
	if p.At.UnixNano() != 1700000000000000000 {
		t.Errorf("the moment parsed as %d", p.At.UnixNano())
	}
}

// hasDisplay reports whether this session has an X server for the reader to open,
// which separates a machine that cannot report a position from a reporter that does not.
func hasDisplay() bool {
	r, ok := pointer.NewX11()
	if ok {
		r.Close()
	}
	return ok
}
