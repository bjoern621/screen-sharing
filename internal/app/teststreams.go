package app

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/wire"
)

// maxTestStreams bounds StartTestStreams: each test stream runs its own x264
// encoder, so a large count saturates the CPU without testing anything new.
const maxTestStreams = 9

// StartTestStreams launches count synthetic test-pattern publishers named
// test-1..test-<count>, pushing to the relay over RTSP. They exercise the
// viewing paths (native grid, web grid, per-stream viewers) without a screen
// capture. A running set is replaced.
func (a *App) StartTestStreams(count int) error {
	if count <= 0 || count > maxTestStreams {
		return fmt.Errorf("test stream count must be 1..%d, got %d", maxTestStreams, count)
	}

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	exe, err := publish.FindGstExe()
	if err != nil {
		return err
	}
	// The launcher a bundle ships runs against the plugins beside it, not against a
	// prefix that exists only on the machine that built the bundle.
	env := publish.GstChildEnv()

	// Announced after the lock is released, for the reason StartWatch states: the count
	// is read back through a method that takes the same mutex.
	defer a.emitTestStreamState()

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
		proc, err := ffmpeg.Start(exe, args, true, false, "teststream-"+name, env, nil, nil,
			func(err error, stderrTail string, logPath string) {
				message := ""
				if err != nil {
					message = err.Error()
					if stderrTail != "" {
						message += "\n" + stderrTail
					}
					logger.Warnf("test stream %s failed: %v\n%s\nfull log: %s", name, err, stderrTail, logPath)
				} else {
					logger.Infof("test stream %s closed (log: %s)", name, logPath)
				}
				// The exit says why this one stopped; the count beside it says how many are
				// left. A publisher that died on its own moves the count with nothing having
				// been called, which is the case the state event exists for.
				a.emit(wire.TestStreamExitEvent(message, logPath))
				a.emitTestStreamState()
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
	defer a.emitTestStreamState()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	a.stopTestStreamsLocked()
}

// emitTestStreamState announces how many synthetic publishers are alive.
//
// It counts through TestStreamsRunning rather than being handed a number, so what is
// announced is what a read would answer with: a publisher that died between the call
// and this is already out of the count. That read takes procMu, so a caller holding it
// defers this rather than calling it in place.
func (a *App) emitTestStreamState() {
	a.emit(wire.TestStreamStateEvent(a.TestStreamsRunning()))
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
