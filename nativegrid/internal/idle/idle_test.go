package idle

import "testing"

// loop stands in for the UI loop: it holds what was deferred to it until a test
// lets it run, which is the window a burst of schedules falls into.
type loop struct{ deferred []func() }

func (l *loop) dispatch(f func()) { l.deferred = append(l.deferred, f) }

func (l *loop) pass() {
	run := l.deferred
	l.deferred = nil
	for _, f := range run {
		f()
	}
}

// A burst of schedules is one run, which is the whole point of the type: a
// roster push moves several streams and the layout is written once.
func TestScheduleCoalesces(t *testing.T) {
	l := &loop{}
	runs := 0
	c := New(l.dispatch, func() { runs++ })

	c.Schedule()
	c.Schedule()
	if runs != 0 {
		t.Fatalf("the job ran %d times before the loop got a pass", runs)
	}
	if !c.Pending() {
		t.Error("a scheduled job is not reported as pending")
	}

	l.pass()

	if runs != 1 {
		t.Errorf("the job ran %d times, want the burst to cost one run", runs)
	}
	if c.Pending() {
		t.Error("a job that ran is still reported as pending")
	}
}

// Flush is the close path: the loop the job was deferred to has returned, and
// the work still has to happen. The pass it beat to it finds nothing left.
func TestFlushRunsADueJobOnce(t *testing.T) {
	l := &loop{}
	runs := 0
	c := New(l.dispatch, func() { runs++ })

	c.Schedule()
	c.Flush()
	if runs != 1 {
		t.Fatalf("the job ran %d times on the flush, want 1", runs)
	}
	if c.Pending() {
		t.Error("a flushed job is still reported as pending")
	}

	l.pass()
	if runs != 1 {
		t.Errorf("the job ran %d times, want the flushed pass to find nothing due", runs)
	}

	c.Flush()
	if runs != 1 {
		t.Errorf("the job ran %d times, want a flush with nothing due to do nothing", runs)
	}
}
