package main

import (
	"context"
	"sync"

	"bjoernblessin.de/screenshare/encoders"
	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/publish"
	"bjoernblessin.de/screenshare/relay"
	"bjoernblessin.de/screenshare/settings"
)

// App is the Wails-bound backend. All exported methods are callable from the
// frontend; events flow the other way via runtime.EventsEmit. Methods are
// grouped by domain across app_settings.go, app_system.go, app_publish.go and
// app_watch.go; this file holds the struct and process lifecycle.
//
// Two mutexes guard the mutable state, never held together: settingsMu guards
// settings, procMu guards the ffmpeg children (pub and watchers). Methods that
// need both snapshot settings under settingsMu first, release it, then take
// procMu, so there is no lock ordering to deadlock on.
type App struct {
	ctx context.Context

	settingsMu sync.Mutex
	settings   settings.Stream

	relay *relay.Client

	encodersOnce sync.Once
	encoders     encoders.Availability

	procMu      sync.Mutex
	pub         publish.Handle
	watchers    map[WatchKey]*ffmpeg.Proc
	wall        *ffmpeg.Proc
	gridViewer  *ffmpeg.Proc
	testStreams []*ffmpeg.Proc
}

func NewApp() *App {
	return &App{
		settings: settings.Load(),
		relay:    relay.New(),
		watchers: map[WatchKey]*ffmpeg.Proc{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown kills every child so no orphan ffmpeg keeps encoding after quit.
func (a *App) shutdown(ctx context.Context) {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.pub != nil {
		a.pub.Stop()
	}
	for _, watcher := range a.watchers {
		watcher.Stop()
	}
	if a.wall != nil {
		a.wall.Stop()
	}
	if a.gridViewer != nil {
		a.gridViewer.Stop()
	}
	for _, proc := range a.testStreams {
		proc.Stop()
	}
}
