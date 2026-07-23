package main

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/relay"
)

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

// StartWatch opens an ffplay window for streamName.
func (a *App) StartWatch(streamName string) error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	args, err := ffmpeg.BuildWatchArgs(s, streamName)
	if err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	watcher, present := a.watchers[streamName]
	if present && watcher.Running() {
		return fmt.Errorf("already watching %s", streamName)
	}

	exe, err := ffmpeg.FindExe("ffplay")
	if err != nil {
		return err
	}

	// hideWindows must be false: SW_HIDE would hide ffplay's video window itself.
	proc, err := ffmpeg.Start(exe, args, false, "watch-"+streamName, nil,
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
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

	// The viewer window only appears once ffplay decoded the first frame
	// (SRT handshake + waiting for a keyframe, typically 1-3s). Poll for it
	// so the UI can show a connecting state until then.
	go a.emitWhenViewerReady(streamName, proc)

	return nil
}

// emitWhenViewerReady emits "watch:ready" once the ffplay window for
// streamName exists, or gives up when the viewer dies or 30s pass
// (watch:exit covers the death case in the UI).
func (a *App) emitWhenViewerReady(streamName string, proc *ffmpeg.Proc) {
	title := ffmpeg.WatchWindowTitle(streamName)

	for range 150 { // 150 * 200ms = 30s
		if !proc.Running() {
			return
		}
		if ffmpeg.WindowExists(title) {
			runtime.EventsEmit(a.ctx, "watch:ready", streamName)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	logger.Warnf("viewer window for '%s' did not appear within 30s", streamName)
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
