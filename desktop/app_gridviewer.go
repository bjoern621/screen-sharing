package main

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/watch"
)

// StartGridViewer opens the GTK grid window: a separate binary (cmd/gridviewer)
// that decodes the named streams natively, so it plays everything the
// gst-launch wall plays, and composites them as GTK widgets, so tiles carry
// chrome and a click spotlights a stream. A running viewer is replaced. The
// wall stays alongside as the dependency-free fallback.
func (a *App) StartGridViewer(streamNames []string, transportName string) error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	cfg, err := watch.BuildGridConfig(s, streamNames, transportName)
	if err != nil {
		return err
	}

	exe := os.Getenv(watch.EnvGridViewer)
	if exe == "" {
		exe, err = ffmpeg.FindExe(watch.GridViewerExe)
		if err != nil {
			return fmt.Errorf("%s not found: build it with 'task gridviewer' or set %s", watch.GridViewerExe, watch.EnvGridViewer)
		}
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.gridViewer != nil {
		a.gridViewer.Stop()
		a.gridViewer = nil
	}

	// hideWindow must be false: SW_HIDE would hide the video window too.
	proc, err := ffmpeg.Start(exe, []string{"-config", cfg}, false, "gridviewer", nil, nil,
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
				logger.Errorf("grid viewer failed: %v\n%s\nfull log: %s", err, stderrTail, logPath)
			} else {
				logger.Infof("grid viewer closed (log: %s)", logPath)
			}
			runtime.EventsEmit(a.ctx, "gridviewer:exit", exitEvent{Message: message, LogPath: logPath})
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("grid viewer watching %v over %s", streamNames, transportName)
	a.gridViewer = proc
	return nil
}

// StopGridViewer closes the GTK grid window.
func (a *App) StopGridViewer() {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.gridViewer != nil {
		a.gridViewer.Stop()
		a.gridViewer = nil
		logger.Infof("stopped the grid viewer")
	}
}

// GridViewerRunning reports whether the GTK grid window is open. The frontend
// polls it, which is also how a viewer closed via its window button is noticed.
func (a *App) GridViewerRunning() bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	return a.gridViewer != nil && a.gridViewer.Running()
}
