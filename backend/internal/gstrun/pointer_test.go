package gstrun

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/pointer"
)

// The child reports the pointer on the stream it reports its caps on, under a prefix of its own,
// and the parent reads it back with the parser that lives beside the writer.
// These hold that the two are one format: a line written here and parsed elsewhere drifts the first
// time a field is added.

// A run whose capture answers a position reports one while it plays, at the source's own rate
// rather than the pipeline's.
func TestARunReportsWhereThePointerIs(t *testing.T) {
	if !hasDisplay() {
		t.Skip("no X server on this session, so ximagesrc has no screen to read")
	}
	var out bytes.Buffer

	// Long enough to outlast several reader ticks, short enough that the test is not a wait.
	// The cancelled context is what stops the reporting and the run.
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	err := RunWithOptions(ctx,
		"ximagesrc use-damage=false ! video/x-raw,framerate=10/1 ! fakesink",
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
		t.Errorf("a run over a capture that answers reported no position: %q", out.String())
	}
}

// A capture that answers no position reports none, whatever the run asked for.
// Which captures answer is the source table's, and a run over one that does not is not a run
// reporting the origin.
func TestARunOverACaptureThatAnswersNothing(t *testing.T) {
	var out bytes.Buffer

	err := RunWithOptions(t.Context(),
		"videotestsrc num-buffers=5 ! fakesink", Options{Pointer: true}, &out)
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if _, ok := ParsePointer(line); ok {
			t.Errorf("a capture answering no position reported %q", line)
		}
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
		PointerPrefix + "0.5 0.25",
		PointerPrefix + "half 0.25 5 true",
		PointerPrefix + "0.5 0.25 5 maybe",
	} {
		if _, ok := ParsePointer(line); ok {
			t.Errorf("%q reads as a position, and it is not one", line)
		}
	}

	var out bytes.Buffer
	want := pointer.Spot{X: 0.25, Y: 0.5, At: time.Unix(0, 1700000000000000000), Visible: true}
	writePointer(&out, want)

	got, ok := ParsePointer(strings.TrimSuffix(out.String(), "\n"))
	if !ok {
		t.Fatalf("a line the writer's own format produced does not parse: %q", out.String())
	}
	if got.X != want.X || got.Y != want.Y || got.Visible != want.Visible {
		t.Errorf("the line parsed as %+v, want %+v", got, want)
	}
	if !got.At.Equal(want.At) {
		t.Errorf("the moment parsed as %v, want %v", got.At, want.At)
	}
}

// A reading is reported as a fraction of the captured picture, whatever size that picture is,
// so nothing downstream needs the size it was read at.
func TestAPositionIsAFractionOfTheCapturedPicture(t *testing.T) {
	hold := &pointerHold{}
	hold.size(800, 400)
	hold.take(200, 300, true)

	at, answered := hold.spot()
	if !answered {
		t.Fatal("a hold with a reading and a size answers nothing")
	}
	if at.X != 0.25 || at.Y != 0.75 {
		t.Errorf("read %v,%v of the picture, want 0.25,0.75", at.X, at.Y)
	}
}

// A position outside the captured picture is one the pointer is not at.
// ximagesrc reads the whole X root and the crop takes one output out of it,
// so the pointer on another head lands outside what this stream carries.
func TestAPositionOutsideTheCapturedPicture(t *testing.T) {
	for _, at := range []struct{ x, y int }{{-1, 10}, {10, -1}, {800, 10}, {10, 400}} {
		hold := &pointerHold{}
		hold.size(800, 400)
		hold.take(at.x, at.y, true)

		got, answered := hold.spot()
		if !answered {
			t.Fatalf("%d,%d: a hold with a reading and a size answers nothing", at.x, at.y)
		}
		if got.Visible {
			t.Errorf("%d,%d reads as over an 800x400 picture", at.x, at.y)
		}
	}
}

// A reading taken before the caps arrive has no picture to be a fraction of,
// and answers nothing rather than a fraction of nothing.
func TestAPositionBeforeTheCapsArrive(t *testing.T) {
	hold := &pointerHold{}
	hold.take(200, 300, true)

	if _, answered := hold.spot(); answered {
		t.Error("a hold that has seen no caps reported a fraction")
	}
}

// hasDisplay reports whether this session has an X server for a capture to read,
// which separates a machine that cannot report a position from a reporter that does not.
func hasDisplay() bool {
	r, ok := pointer.NewX11()
	if ok {
		r.Close()
	}
	return ok
}
