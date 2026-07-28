package main

import (
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/publish"
	"bjoernblessin.de/screenshare/settings"
)

// PublishCommand returns the exact command line the given settings would run,
// without running it. Shown in the UI for transparency. The engine that owns
// the selected capture backend renders it (ffmpeg command or gst pipeline).
func (a *App) PublishCommand(s settings.Stream) (string, error) {
	pub, err := publish.For(s.Capture)
	if err != nil {
		return "", err
	}
	return pub.Command(s)
}

// StartPublish validates s, persists it and starts the encoder child.
func (a *App) StartPublish(s settings.Stream) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	if err := settings.Save(s); err != nil {
		logger.Warnf("Cannot persist settings: %v", err)
	}
	return a.startPublish(s)
}

// startPublishHeld publishes on the settings the app holds, for a caller that
// has none of its own to pass: the native grid's publish button acts on what the
// form last wrote, and moves no setting by pressing it.
func (a *App) startPublishHeld() error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	return a.startPublish(s)
}

// startPublish starts the encoder child on s. The settings it runs on are the
// caller's business: this is the one place a publish begins, whether the form or
// the grid asked for it.
func (a *App) startPublish(s settings.Stream) error {
	pub, err := publish.For(s.Capture)
	if err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.pub != nil && a.pub.Running() {
		return fmt.Errorf("already publishing")
	}

	proc, err := pub.Start(s, "publish", publish.Callbacks{
		OnStats: func(stats publish.Stats) {
			runtime.EventsEmit(a.ctx, "publish:stats", stats)
		},
		OnExit: func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
				logger.Errorf("publish of '%s' failed: %v\n%s\nfull log: %s", s.Name, err, stderrTail, logPath)
			} else {
				logger.Infof("publish of '%s' ended (log: %s)", s.Name, logPath)
			}
			runtime.EventsEmit(a.ctx, "publish:exit", exitEvent{Message: message, LogPath: logPath})
		},
	})
	if err != nil {
		return err
	}

	logger.Infof("publishing '%s' via %s (%s, %s, %d fps)", s.Name, s.Transport, s.Mode, s.Chroma, s.Fps)
	a.pub = proc
	a.emitPublishState(true)
	return nil
}

func (a *App) StopPublish() {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.pub != nil {
		a.pub.Stop()
		a.pub = nil
		logger.Infof("publishing stopped")
		a.emitPublishState(false)
	}
}

// emitPublishState tells the frontend what the publish state became. The form's
// own toggle knows what it asked for; this carries the changes it did not make,
// so the native grid's publish button cannot leave the form unlocked over a
// publish that is running.
func (a *App) emitPublishState(publishing bool) {
	runtime.EventsEmit(a.ctx, "publish:state", publishStateEvent{Publishing: publishing})
}

func (a *App) Publishing() bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	return a.pub != nil && a.pub.Running()
}
