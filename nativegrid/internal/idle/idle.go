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
type Coalescer struct {
	dispatch Dispatch
	job      func()
	pending  bool
}

func New(dispatch Dispatch, job func()) *Coalescer {
	assert.IsNotNil(dispatch, "a coalescer needs a UI loop to defer to")
	assert.IsNotNil(job, "a coalescer needs a job")

	return &Coalescer{dispatch: dispatch, job: job}
}

// Schedule runs the job on the next loop pass, or does nothing while a run is
// already due.
func (c *Coalescer) Schedule() {
	if c.pending {
		return
	}
	c.pending = true
	c.dispatch(func() {
		c.pending = false
		c.job()
	})
}

// Pending reports whether a scheduled run has yet to happen.
func (c *Coalescer) Pending() bool { return c.pending }
