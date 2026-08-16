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

// version is the build stamp the control handshake answers with (docs/ipc-api.md, "Versioning").
//
// Declared in main because that is what the linker writes into: -ldflags "-X main.version=...".
// "dev" is the truthful answer for a build nobody stamped.
var version = "dev"

// main runs the backend, a process with no window: it serves the control socket, supervises what
// the shells ask it to run, and stays up until it is stopped (docs/ipc-api.md).
//
// Nothing here waits for a shell.
// A backend without one keeps publishing, and neither side owes the other a shim for its absence.
func main() {
	assert.Assert(version != "", "a build states which build it is")

	// Re-entry rather than a command line: a GStreamer publish spawns this same executable to play
	// the pipeline.
	// A pipeline in a process of its own keeps gst-launch-1.0's crash isolation, and one this app
	// owns answers questions gst-launch could not (internal/gstrun).
	// Re-entering this binary adds no second artifact to build, ship and find.
	if len(os.Args) > 1 && os.Args[1] == publish.GstSubcommand {
		os.Exit(runPipeline(os.Args[2:]))
	}

	a := app.New(version)
	a.Start()

	// SIGTERM ends this process under a supervisor and interrupt under a terminal.
	// Both reach the one shutdown that stops the children.
	// A kill outside these two leaves the children to the operating system, so nothing is registered
	// for it.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Third way out: the backend reports it has nothing left to do, which is the control endpoint
	// already served by another instance.
	// Same shutdown as a signal, different status.
	// A run that ended because it was asked to succeeded, one that ended because it could not serve
	// did not, and a supervisor reads that.
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
// elements is the pipeline itself, exactly as gst-launch-1.0 takes it, so the command the form
// renders is the command that runs and its tail still pastes into gst-launch.
//
// Interrupt and terminate stop the pipeline, not the process: the capture holds a portal session, a
// DRM lease or an X connection, and only the runner setting the pipeline to NULL hands those back.
func runPipeline(elements []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// What this run does beside playing: the control socket the parent writes to, whether to report
	// the pointer position, and where to measure a frame's delay.
	// All of them lead the arguments, so everything after them is the pipeline.
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
		if element, ok := strings.CutPrefix(elements[0], gstrun.DelayFlag); ok {
			options.Delay, elements = element, elements[1:]
			continue
		}
		if element, ok := strings.CutPrefix(elements[0], gstrun.ShedFlag); ok {
			options.Shed, elements = element, elements[1:]
			continue
		}
		break
	}

	if err := gstrun.RunWithOptions(ctx, strings.Join(elements, " "), options, os.Stdout); err != nil {
		// Standard error is what the supervisor tails, so this wording is what a reader is shown when a
		// publish fails (publish/supervise.go).
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
