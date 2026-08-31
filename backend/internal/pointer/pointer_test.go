package pointer

import (
	"os"
	"testing"
	"time"
)

// The reader is the one part of the pointer channel touching a display server, so these run against
// a real one: a reader that compiled and reported nothing would be a channel carrying a position
// nobody moved.

// A session without an X server says so rather than failing, that being a Wayland session,
// where the position comes from the capture's own metadata instead.
func TestTheX11ReaderAnswersWhereThereIsADisplay(t *testing.T) {
	r, ok := NewX11()
	if !ok {
		if os.Getenv("DISPLAY") != "" {
			t.Errorf("DISPLAY is set to %q and no reader opened", os.Getenv("DISPLAY"))
		}
		t.Skip("no X server on this session, so there is nothing to read a pointer from")
	}
	defer r.Close()

	p, answered := r.Read()
	if !answered {
		t.Fatal("a reader that opened a display answered no position")
	}
	if p.X < 0 || p.Y < 0 {
		t.Errorf("the pointer is at (%d,%d), which is off the root window", p.X, p.Y)
	}
	if p.At.IsZero() {
		t.Error("the position carries no moment, so nothing can hold it against a frame")
	}
	if time.Since(p.At) > time.Second {
		t.Errorf("the position is dated %s ago, which is not when it was read", time.Since(p.At))
	}
}

// The reader is asked hundreds of times a second, so reading twice has to be two reads rather than
// two connections.
func TestReadingTwiceUsesOneConnection(t *testing.T) {
	r, ok := NewX11()
	if !ok {
		t.Skip("no X server on this session")
	}
	defer r.Close()

	for i := range 100 {
		if _, answered := r.Read(); !answered {
			t.Fatalf("read %d of 100 answered nothing", i)
		}
	}
}

// Closing twice is what a supervised child does when it is stopped while stopping, and the second
// close does not take the process with it.
func TestClosingTwiceIsSafe(t *testing.T) {
	r, ok := NewX11()
	if !ok {
		t.Skip("no X server on this session")
	}
	r.Close()
	r.Close()

	// A closed reader answers nothing rather than reading through a connection that is gone.
	if _, answered := r.Read(); answered {
		t.Error("a closed reader answered a position")
	}
}
