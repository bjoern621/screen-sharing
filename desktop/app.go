package main

import (
	"context"
	"sync"

	"fyne.io/systray"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/encoders"
	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/relay"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/webviewer"
)

// App is the Wails-bound backend. All exported methods are callable from the
// frontend; events flow the other way via runtime.EventsEmit. Methods are
// grouped by domain across app_settings.go, app_system.go, app_publish.go and
// app_watch.go; this file holds the struct and process lifecycle.
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
	// they were. It is written once, in NewApp, before the frontend exists, and read
	// through from there: a store that failed at startup is the state the form opens
	// in, and the file it names has been moved aside rather than replaced.
	storeNotice string

	relay     *relay.Client
	webviewer *webviewer.Server

	encodersOnce sync.Once
	encoders     encoders.Availability

	procMu sync.Mutex
	// run is the publish session in force, nil while nothing publishes. It carries the
	// settings its pipeline was built from, which is what a live stream is held against
	// when the form moves off them (app_publish.go).
	run         *publishRun
	watchers    map[WatchKey]*ffmpeg.Proc
	testStreams []*ffmpeg.Proc
	nativeGrid  *ffmpeg.Proc
	// nativeGridWatching is the last watch set nativeGrid reported, empty while no window is open.
	// It belongs to the process above it and is cleared with it,
	// so the app never reports the tiles of a window that is gone.
	nativeGridWatching []string
}

func NewApp() *App {
	s, err := settings.Load()
	notice := ""
	if err != nil {
		// The form is about to open on values the user did not choose, so the reason
		// travels to it rather than staying in the log alone.
		logger.Errorf("settings not restored: %v", err)
		notice = err.Error()
	}

	return &App{
		settings:    s,
		storeNotice: notice,
		relay:       relay.New(),
		watchers:    map[WatchKey]*ffmpeg.Proc{},
	}
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
