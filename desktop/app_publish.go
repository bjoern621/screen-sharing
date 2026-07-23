package main

import (
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/settings"
)

// PublishCommand returns the exact ffmpeg command line for the given settings
// without running it. Shown in the UI for transparency.
func (a *App) PublishCommand(s settings.Stream) (string, error) {
	args, err := ffmpeg.BuildPublishArgs(s)
	if err != nil {
		return "", err
	}

	return "ffmpeg " + strings.Join(args, " "), nil
}

// StartPublish validates s, persists it and starts the encoder child.
func (a *App) StartPublish(s settings.Stream) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	if err := settings.Save(s); err != nil {
		logger.Warnf("Cannot persist settings: %v", err)
	}

	args, err := ffmpeg.BuildPublishArgs(s)
	if err != nil {
		return err
	}

	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.pub != nil && a.pub.Running() {
		return fmt.Errorf("already publishing")
	}

	proc, err := ffmpeg.Start(exe, args, true, "publish", // hide ffmpeg's console window
		func(stats ffmpeg.Stats) {
			runtime.EventsEmit(a.ctx, "publish:stats", stats)
		},
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
			}
			runtime.EventsEmit(a.ctx, "publish:exit", exitEvent{Message: message, LogPath: logPath})
		})
	if err != nil {
		return err
	}

	logger.Infof("publishing '%s' via %s (%s, %s, %d fps)", s.Name, s.Transport, s.Mode, s.Chroma, s.Fps)
	a.pub = proc
	return nil
}

func (a *App) StopPublish() {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.pub != nil {
		a.pub.Stop()
		a.pub = nil
		logger.Infof("publishing stopped")
	}
}

func (a *App) Publishing() bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	return a.pub != nil && a.pub.Running()
}
