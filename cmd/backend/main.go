package main

import (
	"os"
	"os/signal"
	"syscall"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/app"
)

// version is this build's stamp, which the control handshake answers with so a shell
// can name the backend it is talking to (docs/ipc-api.md, "Versioning").
//
// It lives here because this is what the linker writes into: a release build sets it
// with -ldflags "-X main.version=...". A build nobody stamped says so rather than
// claiming a number, since "dev" is a truthful answer to which build this is and an
// invented version is not.
var version = "dev"

// The backend is a process with no window. It opens the control socket, supervises
// whatever the shells ask it to run, and stays up until it is asked to stop
// (docs/ipc-api.md).
//
// Nothing here waits for a shell. A backend without one keeps publishing, and a shell
// without a backend says so rather than inventing a form, which is the same rule from
// the two ends: neither owes the other a compatibility shim for its absence.
func main() {
	a := app.New(version)
	a.Start()

	// SIGTERM is how a supervisor ends this process and interrupt is how a terminal
	// does; both mean the same thing here, so both reach the one shutdown that stops
	// the children. A process killed outside these leaves its children to the
	// operating system, which is why nothing is registered for that case.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// The third way out is the backend itself reporting that it has nothing left to do,
	// which is the control endpoint already being served by another instance. It takes
	// the same shutdown as a signal and differs in the status: a run that ended because
	// it was asked to succeeded, and one that ended because it could not serve did not,
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
