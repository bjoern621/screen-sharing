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
	"bjoernblessin.de/screenshare/internal/decode"
	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/reach"
	"bjoernblessin.de/screenshare/internal/release"
	"bjoernblessin.de/screenshare/internal/update"
)

// version is the build stamp the control handshake answers with (docs/ipc-api.md, "Versioning").
//
// In main because that is what the linker writes into: -ldflags "-X main.version=...".
// "dev" answers for a build nobody stamped.
var version = "dev"

// channel is where this copy of the app came from, written by the recipe that packaged it:
// -ldflags "-X main.channel=..." (docs/updates.md).
// Empty for a build nobody packaged, which replaces nothing.
var channel = ""

// main runs the backend, a process with no window: serves the control socket,
// supervises what the shells ask it to run, and stays up until stopped (docs/ipc-api.md).
//
// Nothing here waits for a shell.
// A backend without one keeps publishing.
func main() {
	assert.Assert(version != "", "a build states which build it is")

	// A GStreamer publish spawns this same executable to play the pipeline.
	// A process of its own keeps gst-launch-1.0's crash isolation,
	// and one this app owns answers questions gst-launch could not (internal/gstrun).
	// Re-entry adds no second artifact to build and ship.
	if len(os.Args) > 1 && os.Args[1] == publish.GstSubcommand {
		os.Exit(runPipeline(os.Args[2:]))
	}

	// Every receive pipeline runs in one more run of this executable.
	// A GPU reset aborts whichever process was submitting to the ring, so a decode here would cost
	// the control socket and the publish supervision along with the picture (internal/decode).
	if len(os.Args) > 1 && os.Args[1] == decode.Subcommand {
		decode.Main(os.Args[2:])
		return
	}

	// Dials, prints and exits, without bringing up the app it shares an executable with (check.go).
	if len(os.Args) > 1 && os.Args[1] == reach.Subcommand {
		os.Exit(runCheck())
	}

	// Replaces this install with the release staged beside it, out of a copy of this binary.
	// The run that started it is exiting, and this one waits for that before it touches a file
	// (internal/update, the applier).
	if len(os.Args) > 1 && os.Args[1] == update.Subcommand {
		os.Exit(update.Main(os.Args[2:]))
	}

	// Before anything logs: a shell starts this process with no console (log.go).
	openLog()

	a := app.New(version, channel)
	a.Start()

	// Off the startup path, as the release read is:
	// a crashed earlier run is looked for and reported while the app comes up.
	go a.ReportLastCrash(ownLogTag)

	// Which build this is, beside the one that is published.
	// A tester runs whatever they downloaded once, so a report about a bug already fixed
	// carries the build it came from and the log says whether that build predates the fix.
	//
	// Off the startup path: a forge that does not answer would otherwise hold the app
	// behind a read nothing waits on.
	go logRelease()

	// SIGTERM ends this process under a supervisor, interrupt under a terminal.
	// Both reach the one shutdown that stops the children.
	// A kill outside these two leaves the children to the operating system.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Third way out: the control endpoint is already served by another instance.
	// Same shutdown as a signal, different status, which a supervisor reads:
	// a run asked to end succeeded, one that could not serve did not.
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
// elements is the pipeline exactly as gst-launch-1.0 takes it,
// so the command the form renders is the command that runs and its tail pastes into gst-launch.
//
// Interrupt and terminate stop the pipeline, not the process:
// the capture holds a portal session, a DRM lease or an X connection,
// and only the runner setting the pipeline to NULL hands those back.
func runPipeline(elements []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Options lead the arguments, so everything after them is the pipeline.
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
		// Supervisor tails stderr,
		// so this wording is what a reader is shown when a publish fails (publish/supervise.go).
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// logRelease writes which build is running and what the published one is.
//
// Every outcome is a log line and none of them stops anything.
// A machine behind a firewall, offline, or running a build nobody stamped is told
// what it is running and nothing about what it is not.
//
// The environment switches it off with the app's own check,
// so an install whose packager delivers updates reaches the forge on no path at all.
func logRelease() {
	if os.Getenv(update.EnvCheck) == "0" {
		logger.Infof("running %s; %s is off, so no release was read", version, update.EnvCheck)
		return
	}

	latest, err := release.Fetch(context.Background())
	if err != nil {
		logger.Infof("running %s; the published release could not be read: %v", version, err)
		return
	}

	switch release.Compare(version, latest.Tag) {
	case release.StateBehind:
		logger.Warnf("running %s; %s is published: %s", version, latest.Tag, latest.URL)
	case release.StateCurrent:
		logger.Infof("running %s, which is the published release", version)
	default:
		logger.Infof("running %s; the published release is %s", version, latest.Tag)
	}
}
