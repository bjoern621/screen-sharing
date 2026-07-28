package gstreamer

import (
	"runtime"

	"github.com/diamondburned/gotk4/pkg/glib/v2"

	"bjoernblessin.de/go-utils/util/assert"
)

// withOwnMainContext runs a pipeline's way down to StateNull under a main context
// of the calling thread's own, pushed as its thread-default, which leaves the
// sink's share of the teardown to the UI loop.
//
// gtk4paintablesink hands that share to the default main context: leaving PAUSED
// flushes the frames the paintable holds.
// The paintable is a GTK object of the thread that built it, the UI loop's, and
// the sink's Rust half answers one touched from another thread with a panic that
// takes the process with it.
//
// GLib only queues such a hand-off for a thread that cannot take the default
// context.
// A thread whose thread-default context is that context and that acquires it runs
// the closure inline instead, and the loop holds the context only while it runs.
// Session.Close stops every pipeline on a thread of its own once the loop has
// returned, and those threads took the context nobody held any more and flushed
// the paintable themselves.
//
// A context of this thread's own settles that wherever the teardown runs, without
// depending on who holds the default one: the hand-off fails the thread-default
// test and is queued for the loop.
// The pin is what makes the push mean anything, because a thread-default context
// belongs to an OS thread and an unpinned goroutine can leave the one it pushed
// on.
//
// The way up takes no such guard: the sink builds the paintable on the loop and
// blocks until it has it, which a context of this thread's own would leave waiting
// for a pass that never comes.
func withOwnMainContext(change func()) {
	assert.IsNotNil(change, "a guarded teardown runs a state change")

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ctx := glib.NewMainContext()
	ctx.PushThreadDefault()
	defer ctx.PopThreadDefault()

	change()
}
