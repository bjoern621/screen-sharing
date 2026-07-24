package main

import (
	"fmt"
	"strconv"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/publish"
)

// maxTestStreams bounds StartTestStreams: each test stream runs its own x264
// encoder, so a large count saturates the CPU without testing anything new.
const maxTestStreams = 9

// StartTestStreams launches count synthetic test-pattern publishers named
// test-1..test-<count>, pushing to the relay over RTSP. They exercise the
// viewing paths (native grid, WHEP grid, per-stream viewers) without a screen
// capture. A running set is replaced.
func (a *App) StartTestStreams(count int) error {
	if count <= 0 || count > maxTestStreams {
		return fmt.Errorf("test stream count must be 1..%d, got %d", maxTestStreams, count)
	}

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	exe, err := ffmpeg.FindExe(publish.TestStreamExe)
	if err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	a.stopTestStreamsLocked()

	for i := range count {
		name := "test-" + strconv.Itoa(i+1)
		args, err := publish.BuildTestStreamArgs(s, name, publish.TestPattern(i))
		if err != nil {
			a.stopTestStreamsLocked()
			return err
		}
		proc, err := ffmpeg.Start(exe, args, true, "teststream-"+name, nil, nil,
			func(err error, stderrTail string, logPath string) {
				message := ""
				if err != nil {
					message = err.Error()
					if stderrTail != "" {
						message += "\n" + stderrTail
					}
					logger.Errorf("test stream %s failed: %v\n%s\nfull log: %s", name, err, stderrTail, logPath)
				} else {
					logger.Infof("test stream %s closed (log: %s)", name, logPath)
				}
				runtime.EventsEmit(a.ctx, "teststream:exit", exitEvent{Message: message, LogPath: logPath})
			})
		if err != nil {
			a.stopTestStreamsLocked()
			return err
		}
		a.testStreams = append(a.testStreams, proc)
	}

	logger.Infof("started %d test streams", count)
	return nil
}

// StopTestStreams stops every synthetic publisher.
func (a *App) StopTestStreams() {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	a.stopTestStreamsLocked()
}

// stopTestStreamsLocked stops and forgets the synthetic publishers.
// The caller holds procMu.
func (a *App) stopTestStreamsLocked() {
	for _, proc := range a.testStreams {
		proc.Stop()
	}
	if len(a.testStreams) > 0 {
		logger.Infof("stopped %d test streams", len(a.testStreams))
	}
	a.testStreams = nil
}

// TestStreamsRunning reports how many synthetic publishers are alive. The
// frontend polls it, so a publisher that died on its own drops out of the
// count without an extra event round-trip.
func (a *App) TestStreamsRunning() int {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	n := 0
	for _, proc := range a.testStreams {
		if proc.Running() {
			n++
		}
	}
	return n
}
