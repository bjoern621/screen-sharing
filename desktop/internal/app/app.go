package app

import (
	"context"
	"sync"

	"fyne.io/systray"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/webviewer"
)

// App is the Wails-bound backend. All exported methods are callable from the
// frontend; events flow the other way via runtime.EventsEmit. Methods are
// grouped by domain across settings.go, system.go, publish.go and watch.go;
// this file holds the struct and process lifecycle.
//
// Two mutexes guard the mutable state, never held together: settingsMu guards
// settings, procMu guards the children (the publish run and the watchers). Methods
// that need both snapshot settings under settingsMu first, release it, then take
// procMu, so there is no lock ordering to deadlock on.
type App struct {
	ctx context.Context

	settingsMu sync.Mutex
	settings   settings.Stream

	// storeNotice states why the persisted settings could not be read, empty when
	// they were. It is written once, in New, before the frontend exists, and read
	// through from there: a store that failed at startup is the state the form opens
	// in, and the file it names has been moved aside rather than replaced.
	storeNotice string

	relay     *relay.Client
	webviewer *webviewer.Server

	// trayIcon is the image the tray shows, handed in rather than embedded here:
	// go:embed reads no path above its own directory, and the icons sit under
	// desktop/build. Package main is what still sits above that, so the embed stays
	// there (tray_icon_windows.go, tray_icon_other.go) and the bytes travel to New.
	trayIcon []byte

	encodersOnce sync.Once
	encoders     encoders.Availability

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

func New(trayIcon []byte) *App {
	assert.Assert(len(trayIcon) > 0, "the tray has an icon to show", len(trayIcon))

	s, err := settings.Load()
	notice := ""
	if err != nil {
		// The form is about to open on values the user did not choose, so the reason
		// travels to it rather than staying in the log alone.
		logger.Warnf("settings not restored: %v", err)
		notice = err.Error()
	}

	return &App{
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

// StoreNotice states why the persisted settings could not be restored, empty when
// they were. The form opens on the defaults in that case, and the file holding the
// old values has been moved aside rather than overwritten, so the sentence names
// where they are.
func (a *App) StoreNotice() string {
	return a.storeNotice
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	enableWebviewWebRTC()
	a.startWebViewer()
	a.startTray()
}

// shutdown kills every child so no orphan ffmpeg keeps encoding after quit.
func (a *App) shutdown(ctx context.Context) {
	// The tray runs its own loop; end it so it does not hold the process open.
	systray.Quit()

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
