// Package idle defers work to the next pass of the UI loop.
//
// The loop itself is injected as a dispatch function, so the callers that own
// widgets stay independent of GTK's main loop and a test drives them
// synchronously.
package idle

import "bjoernblessin.de/go-utils/util/assert"

// Dispatch runs a function on the UI loop. coreglib.IdleAdd is the real one.
type Dispatch func(func())

// Coalescer runs one job on the next pass of the UI loop, no matter how often it
// was scheduled in between.
//
// Two callers need that. A burst of model changes must cost one relayout, and a
// relayout must not reparent widgets from inside a drag-and-drop callback, which
// wedges the operation and the pointer grab with it. Pending reports the window
// in which the job is due but has not run, where the widget bounds a hit test
// would read are the ones the job is about to change.
//
// Every method belongs to the UI loop, the thread the dispatch it was built with
// runs on. The pending mark is a plain field, so a Schedule from a pipeline
// thread races the pass that clears it and the job it asked for is lost. Nothing
// here can check the calling thread at a price worth paying, which leaves it a
// rule the callers keep: a player callback hops to the loop before it schedules.
type Coalescer struct {
	dispatch Dispatch
	job      func()
	pending  bool
	// running marks the job's own run. A job that schedules itself asks for the
	// next pass under a real loop and recurses until the stack ends under a
	// dispatch that runs inline, so it is refused rather than left to behave one
	// way in the window and another in a test.
	running bool
}

func New(dispatch Dispatch, job func()) *Coalescer {
	assert.IsNotNil(dispatch, "a coalescer needs a UI loop to defer to")
	assert.IsNotNil(job, "a coalescer needs a job")

	return &Coalescer{dispatch: dispatch, job: job}
}

// Schedule runs the job on the next loop pass, or does nothing while a run is
// already due. It is called on the UI loop, like every method here.
func (c *Coalescer) Schedule() {
	assert.Assert(!c.running, "a coalesced job asks for no pass of its own")

	if c.pending {
		return
	}
	c.pending = true
	c.dispatch(c.run)
}

// Flush runs a due job here and now. It is for a caller whose UI loop has
// already returned: the pass the job was scheduled on never comes, and the work
// still has to happen.
func (c *Coalescer) Flush() { c.run() }

// run is one due pass, whether the loop got to it or a Flush beat it there. The
// pending mark is taken first, so the two of them run the job once between them.
func (c *Coalescer) run() {
	if !c.pending {
		return
	}
	c.pending = false
	c.running = true
	c.job()
	c.running = false
}

// Pending reports whether a scheduled run has yet to happen. It is called on the
// UI loop, like every method here.
func (c *Coalescer) Pending() bool { return c.pending }
