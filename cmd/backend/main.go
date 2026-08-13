package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/app"
	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/publish"
)

// version is this build's stamp, which the control handshake answers with so a shell can name the
// backend it is talking to (docs/ipc-api.md, "Versioning").
//
// It lives here because this is what the linker writes into: a release build sets it with -ldflags
// "-X main.version=...".
// A build nobody stamped says so rather than claiming a number, since "dev" is a truthful answer to
// which build this is and an invented version is not.
var version = "dev"

// The backend is a process with no window.
// It opens the control socket, supervises whatever the shells ask it to run,
// and stays up until it is asked to stop (docs/ipc-api.md).
//
// Nothing here waits for a shell.
// A backend without one keeps publishing, and a shell without a backend says so rather than
// inventing a form, which is the same rule from the two ends: neither owes the other a
// compatibility shim for its absence.
func main() {
	assert.Assert(version != "", "a build states which build it is")

	// The one argument this process answers to, and it is not a command line so much as a re-entry:
	// a publish on the GStreamer engine spawns this same executable to play the pipeline
	// (publish.GstExe), because a pipeline in a process of its own keeps the crash isolation
	// gst-launch-1.0 gave and a pipeline this app owns can be asked what gst-launch could not
	// (internal/gstrun).
	//
	// Spawning the binary that is already running costs no second artifact to build,
	// ship or find on a user's machine, which is what a separate launcher would have added to every
	// packaging recipe.
	if len(os.Args) > 1 && os.Args[1] == publish.GstSubcommand {
		os.Exit(runPipeline(os.Args[2:]))
	}

	a := app.New(version)
	a.Start()

	// SIGTERM is how a supervisor ends this process and interrupt is how a terminal does.
	// Both mean the same thing here, so both reach the one shutdown that stops the children.
	// A process killed outside these leaves its children to the operating system,
	// which is why nothing is registered for that case.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// The third way out is the backend itself reporting that it has nothing left to do,
	// which is the control endpoint already being served by another instance.
	// It takes the same shutdown as a signal and differs in the status: a run that ended because it
	// was asked to succeeded, and one that ended because it could not serve did not,
	// which is what a task runner and a supervisor read.
	code := 0
	select {
	case <-stop:
	case err := <-a.Fatal():
		logger.Warnf("stopping: %v", err)
		code = 1
	}

	a.Stop()
	os.Exit(code)
}

// runPipeline plays one publish pipeline and reports how it ended, as an exit status.
//
// The arguments are the pipeline's own elements, exactly as gst-launch-1.0 took them,
// so what the form renders as the command is what runs and a reader can still paste the tail of it
// into gst-launch to see the same pipeline.
//
// Interrupt and terminate stop the pipeline rather than the process: the capture holds a portal
// session, a DRM lease or an X connection, and letting the runner set the pipeline to NULL is what
// hands those back instead of leaving them to process teardown.
func runPipeline(elements []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// What this run does beside playing, which leads the arguments so everything after them is the
	// pipeline itself: the control socket the parent writes to, and whether to report where the
	// pointer is.
	var options gstrun.Options
	for len(elements) > 0 {
		if path, ok := strings.CutPrefix(elements[0], publish.ControlFlag); ok {
			options.Control, elements = path, elements[1:]
			continue
		}
		if elements[0] == gstrun.PointerFlag {
			options.Pointer, elements = true, elements[1:]
			continue
		}
		break
	}

	if err := gstrun.RunWithOptions(ctx, strings.Join(elements, " "), options, os.Stdout); err != nil {
		// Standard error, where the supervisor's tail reads from: this is the wording a reader is shown
		// when a publish fails (publish/supervise.go).
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
