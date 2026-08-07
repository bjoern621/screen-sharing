package app

import (
	"context"
	"sync"
	"sync/atomic"

	"fyne.io/systray"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/webviewer"
)

// App is the backend both surfaces in front of it reach. All exported methods are
// callable from the Wails frontend, and control.go serves the same state to the
// control contract's shells; events flow the other way through one function
// (events.go). Methods are grouped by domain across settings.go, system.go,
// publish.go and watch.go; this file holds the struct and process lifecycle.
//
// Three mutexes guard the mutable state and none is held while another is taken:
// settingsMu guards settings, procMu guards the children (the publish run and the
// watchers), and controlMu guards the control service's handle. Methods that need
// settings and children snapshot settings under settingsMu first, release it, then
// take procMu, so there is no lock ordering to deadlock on.
//
// The probe result and the last relay snapshot are atomic pointers rather than
// fields under a lock, because each is written whole and is read on a path that must
// not wait for the write: a form resolve reads what has been probed without waiting
// for a probe that is running, and the control service reads the relay snapshot
// without waiting for a fetch that is in flight.
type App struct {
	ctx context.Context

	// events announces every state change to the control shells. It is the same act
	// as the Wails runtime event beside it, and the two go out through one function,
	// so a change cannot reach one surface and not the other (events.go).
	events *events.Broker
	// version is this build's stamp, which the control handshake answers with. It is
	// handed in because the linker writes it into package main.
	version string

	settingsMu sync.Mutex
	settings   settings.Stream

	// storeNotice states why the persisted settings could not be read, nil when
	// they were. It is written once, in New, before the frontend exists, and read
	// through from there: a store that failed at startup is the state the form opens
	// in, and the file it names has been moved aside rather than replaced.
	storeNotice *screensharev1.Text

	relay *relay.Client
	// relayLast is the snapshot the last fetch produced, nil until one has been taken.
	// Every fetch writes it and the control service reads it, so several shells asking
	// what is live do not multiply the requests the relay sees (watch.go).
	relayLast atomic.Pointer[relay.Status]

	webviewer *webviewer.Server

	// trayIcon is the image the tray shows, handed in rather than embedded here:
	// go:embed reads no path above its own directory, and the icons sit under
	// desktop/build. Package main is what still sits above that, so the embed stays
	// there (tray_icon_windows.go, tray_icon_other.go) and the bytes travel to New.
	trayIcon []byte

	// encodersOnce runs the probe once per process, so the caller that asks for the
	// answer waits for it and every caller after that does not.
	encodersOnce sync.Once
	// encoders is the probe result, nil until the probe has finished. It is a pointer
	// read atomically because the two readers want different things: a caller that
	// needs the answer waits through encodersOnce, and a form resolve reads what is
	// there now and never waits (system.go).
	encoders atomic.Pointer[encoders.Availability]

	// controlOnce makes starting the control service idempotent, controlMu guards the
	// handle it produced, and controlStopped says the shutdown has already run. All
	// three belong to control.go, which states what each of them covers.
	controlOnce    sync.Once
	controlMu      sync.Mutex
	control        *control.Service
	controlStopped bool

	procMu sync.Mutex
	// run is the publish session in force, nil while nothing publishes. It carries the
	// settings its pipeline was built from, which is what a live stream is held against
	// when the form moves off them (publish.go).
	run *publishRun
	// retry is the relaunch a pipeline that died on its own is waiting on, nil when none
	// is pending. It and run are never both set: the retry exists exactly between the
	// exit that armed it and the launch that consumes it (publish_retry.go).
	retry       *publishRetry
	watchers    map[WatchKey]*ffmpeg.Proc
	testStreams []*ffmpeg.Proc
	nativeGrid  *ffmpeg.Proc
	// nativeGridWatching is the last watch set nativeGrid reported, empty while no window is open.
	// It belongs to the process above it and is cleared with it,
	// so the app never reports the tiles of a window that is gone.
	nativeGridWatching []string
}

// New builds the backend. version is this build's stamp, which the control
// handshake answers with so a shell can name what it is talking to.
func New(trayIcon []byte, version string) *App {
	assert.Assert(len(trayIcon) > 0, "the tray has an icon to show", len(trayIcon))
	assert.Assert(version != "", "a build names the version its shells report")

	s, err := settings.Load()
	var notice *screensharev1.Text
	if err != nil {
		// The form is about to open on values the user did not choose, so the fact
		// travels to it rather than staying in the log alone. What travels is the fact
		// and the path the old values were moved to; why the file could not be read is
		// the operating system's answer and stays in the log, where the one reader who
		// can act on it is looking.
		logger.Warnf("settings not restored: %v", err)
		notice = settings.StoreNotice(
			screensharev1.TextCode_TEXT_CODE_SETTINGS_STORE_UNREADABLE, err)
	}

	return &App{
		events:      events.New(),
		version:     version,
		settings:    s,
		storeNotice: notice,
		relay:       relay.New(),
		watchers:    map[WatchKey]*ffmpeg.Proc{},
		trayIcon:    trayIcon,
	}
}

// Hooks hands out the process lifecycle callbacks wails.Run takes.
//
// They travel as values from a package function rather than as exported methods
// because Wails binds every exported method on the struct it is given: exporting
// these would put startup and shutdown in the frontend's API, and hand it a
// context.Context parameter the binding generator has no model for.
func Hooks(a *App) (startup, shutdown func(context.Context)) {
	assert.IsNotNil(a, "lifecycle hooks belong to an app")

	return a.startup, a.shutdown
}

// StoreNotice states why the persisted settings could not be restored, nil when they
// were. The form opens on the defaults in that case, and the file holding the old
// values has been moved aside rather than overwritten, so the statement carries where
// they are.
func (a *App) StoreNotice() *screensharev1.Text {
	return a.storeNotice
}

// startup takes the window's context and brings up everything that runs beside it.
//
// Nothing here waits for what it starts. The tray drives its own loop, the viewer
// service reports a bind failure and is not retried, and the control service opens
// its socket on a goroutine of its own (control.go), so the window appears at the
// speed of the window rather than at the speed of the slowest of them.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	enableWebviewWebRTC()
	a.startWebViewer()
	a.startTray()
	a.startControl()
}

// shutdown kills every child so no orphan ffmpeg keeps encoding after quit.
func (a *App) shutdown(ctx context.Context) {
	// The tray runs its own loop; end it so it does not hold the process open.
	systray.Quit()

	// The contract closes before the children do, so an effect a shell asked for
	// cannot start one of them behind the teardown below.
	a.stopControl()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.run != nil {
		a.run.handle.Stop()
	}
	// A pending relaunch would start an encoder into a process on its way out.
	a.cancelRetryLocked()
	for _, watcher := range a.watchers {
		watcher.Stop()
	}
	for _, proc := range a.testStreams {
		proc.Stop()
	}
	if a.nativeGrid != nil {
		a.nativeGrid.Stop()
	}
	if a.webviewer != nil {
		a.webviewer.Stop()
	}
}
