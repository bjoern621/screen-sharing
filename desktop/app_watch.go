package main

import (
	"fmt"
	"os"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/relay"
	"bjoernblessin.de/screenshare/settings"
)

// watchCommand selects the viewer for streamName: its executable name, argument
// list, and environment overrides. It reads SCREENSHARE_VIEWER, defaulting to
// ffplay; the value "mpv" switches to mpv.
//
// The ffplay path pins SDL to the X11 (XWayland) backend on Linux, whose window
// the compositor renders reliably where the SDL Wayland backend may not. mpv
// needs no such override: its native Wayland output already shows a window.
func watchCommand(s settings.Stream, streamName string) (exe string, args, env []string, err error) {
	if os.Getenv("SCREENSHARE_VIEWER") == "mpv" {
		args, err = ffmpeg.BuildWatchArgsMpv(s, streamName)
		return "mpv", args, nil, err
	}

	args, err = ffmpeg.BuildWatchArgs(s, streamName)
	if goruntime.GOOS == "linux" {
		env = []string{"SDL_VIDEODRIVER=x11"}
	}
	return "ffplay", args, env, err
}

// Live returns the relay snapshot. The frontend polls this every 2 seconds;
// per-path bitrates are only meaningful with such a steady poll interval.
func (a *App) Live() relay.Status {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	return a.relay.Fetch(s.RelayHost, s.ApiPort)
}

// Watching lists the streams currently viewed. Dead viewers are reaped here.
func (a *App) Watching() []string {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	names := []string{}
	for name, watcher := range a.watchers {
		if watcher.Running() {
			names = append(names, name)
		} else {
			delete(a.watchers, name)
		}
	}
	return names
}

// StartWatch opens a viewer window for streamName. The player is ffplay by
// default; SCREENSHARE_VIEWER=mpv selects mpv instead, for comparing the two on
// a given machine without a rebuild.
func (a *App) StartWatch(streamName string) error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	exeName, args, env, err := watchCommand(s, streamName)
	if err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	watcher, present := a.watchers[streamName]
	if present && watcher.Running() {
		return fmt.Errorf("already watching %s", streamName)
	}

	exe, err := ffmpeg.FindExe(exeName)
	if err != nil {
		return err
	}

	// hideWindow must be false: SW_HIDE would hide the viewer's video window too.
	proc, err := ffmpeg.Start(exe, args, false, "watch-"+streamName, env, nil,
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
				logger.Errorf("viewer for '%s' failed: %v\n%s\nfull log: %s", streamName, err, stderrTail, logPath)
			} else {
				logger.Infof("viewer for '%s' closed (log: %s)", streamName, logPath)
			}
			runtime.EventsEmit(a.ctx, "watch:exit", watchExitEvent{
				Name: streamName, Message: message, LogPath: logPath,
			})
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("watching '%s'", streamName)
	a.watchers[streamName] = proc

	// Readiness is not signalled from here. The viewer is "connected" once the
	// relay reports a reader on the path, which the frontend already sees in its
	// Live() snapshot. That signal is independent of the window system, unlike a
	// probe for the ffplay window (no portable form exists under Wayland).
	return nil
}

func (a *App) StopWatch(streamName string) {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	watcher, present := a.watchers[streamName]
	if present {
		watcher.Stop()
		delete(a.watchers, streamName)
		logger.Infof("stopped watching '%s'", streamName)
	}
}
