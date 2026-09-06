package app

import (
	"os"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/update"
	"bjoernblessin.de/screenshare/internal/wire"
)

// What this install knows about the release published beside it (internal/update).
//
// The app owns one manager and announces what it learns,
// so a check one shell asked for reaches the ones that did not, like every other state here.
// Nothing is read on a schedule: a reader asks, and an install that asks nothing at all says so
// before any button is drawn.

// newUpdates builds the manager for this install.
//
// The running binary decides two things: the directory an install would replace,
// and, on Windows, whether this copy came from the installer or from the archive.
// A binary that cannot be located leaves the manager on the path it is running from,
// which is what an unlocatable executable is on every platform this ships to.
func newUpdates(version, channel string, announce func(update.State)) *update.Manager {
	assert.IsNotNil(announce, "an install announces what it learns")

	exe, err := os.Executable()
	if err != nil {
		// Umgebungsfehler: the run goes on, and what it cannot do is replace its own files.
		logger.Warnf("cannot locate the running binary, so no update installs itself: %v", err)
		exe = os.Args[0]
	}

	return update.New(version, update.Channel(channel), exe, announce)
}

// UpdateState answers what this install knows, reaching nothing.
func (a *App) UpdateState() update.State {
	assert.IsNotNil(a.updates, "an app holds one answer about the published release")

	return a.updates.State()
}

// CheckUpdate reads the published release, and fetches it where this install replaces its own files.
//
// Returns as soon as the work is under way.
// Every step reaches every shell on the event stream, the download's progress included.
//
// No context, as StartMonitorPreview takes none: what it starts outlives the call,
// and a caller's context would end the download as the reply that started it is written.
// Refused where the install asks nothing at all, which a shell reads off the state first.
func (a *App) CheckUpdate() error {
	assert.IsNotNil(a.updates, "an app holds one answer about the published release")

	return a.updates.Check()
}

// InstallUpdate starts the staged release and leaves the running app to close.
//
// The applier outlives this process: it waits for it to exit, puts the files in place
// and starts the app again (internal/update, the applier).
// Refused where nothing is staged and verified.
func (a *App) InstallUpdate() error {
	assert.IsNotNil(a.updates, "an app holds one answer about the published release")

	return a.updates.Install()
}

// emitUpdateState puts one whole update state on the event stream.
func (a *App) emitUpdateState(state update.State) {
	a.emit(wire.UpdateStateEvent(state))
}
