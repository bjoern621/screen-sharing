package app

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// maxTestStreams bounds StartTestStreams: each test stream runs its own x264
// encoder, so a large count saturates the CPU without testing anything new.
const maxTestStreams = 9

// testStreamsAtBoot is the set this process brings up with itself and keeps alive, so
// the viewer roster carries streams on a machine that is publishing nothing. The tile
// grid is being built against them, and a relay with nothing on it shows the roster's
// empty state rather than the screen under construction.
const testStreamsAtBoot = 3

// EnvTestStreams overrides that count for one run, and 0 turns the boot set off
// entirely. It exists because the default is not free: three x264 encoders run for as
// long as the backend does, whether anybody is watching them or not.
const EnvTestStreams = "SCREENSHARE_TEST_STREAMS"

// testStream is one slot of the synthetic set: the child publishing it, or the relaunch
// that child's death armed. Exactly one of proc and timer is set, because a slot is
// either running or waiting to run again (teststreams_retry.go).
//
// The slot is the stream's identity rather than the process being: it names the stream
// (slot 0 is test-1) and picks the pattern, so a relaunch comes back as the same row on
// the roster instead of as a fourth stream.
type testStream struct {
	proc  *ffmpeg.Proc
	timer *time.Timer
	// startedAt dates the launch and attempts counts the relaunches spent on the streak
	// this one belongs to. The exit is weighed against both, the way a publish exit is:
	// a child that ran long enough proves the set works here and starts the next outage
	// on a full ladder.
	startedAt time.Time
	attempts  int
}

// testStreamName is what the relay carries slot i under.
func testStreamName(i int) string {
	assert.Assert(i >= 0, "a test stream holds a slot in the set", i)
	return "test-" + strconv.Itoa(i+1)
}

// StartTestStreams launches count synthetic test-pattern publishers named
// test-1..test-<count>, pushing to the relay over RTSP. They exercise the viewing paths
// (the viewer roster, the per-stream viewers, the receive pipelines) without a screen
// capture. A running set is replaced.
//
// The set is kept alive rather than only started: a publisher that dies on its own is
// relaunched into the slot it held (teststreams_retry.go), which is what makes the boot
// set survive a relay that comes up after this process does.
func (a *App) StartTestStreams(count int) error {
	if count <= 0 || count > maxTestStreams {
		return fmt.Errorf("test stream count must be 1..%d, got %d", maxTestStreams, count)
	}

	s, exe, env, err := a.testStreamLaunch()
	if err != nil {
		return err
	}

	// Announced after the lock is released, for the reason StartWatch states: the count
	// is read back through a method that takes the same mutex.
	defer a.emitTestStreamState()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	a.stopTestStreamsLocked()
	a.testStreamsWanted = count

	for i := range count {
		if err := a.launchTestStreamLocked(i, 0, s, exe, env); err != nil {
			a.stopTestStreamsLocked()
			return err
		}
	}

	logger.Infof("started %d test streams", count)
	return nil
}

// StopTestStreams stops every synthetic publisher, and the set stays off until it is
// asked for again: the wanted count goes to zero with the children, so the relaunches
// this stop cancels are not re-armed behind it.
func (a *App) StopTestStreams() {
	defer a.emitTestStreamState()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	a.stopTestStreamsLocked()
}

// startTestStreamsAtBoot brings the always-on set up beside the process.
//
// It runs on a goroutine of its own for the reason everything App.Start begins does:
// resolving the launcher touches the disk and each child opens its run log, and the
// contract is up at its own speed rather than at theirs.
func (a *App) startTestStreamsAtBoot() {
	count := testStreamsAtBootWanted()
	if count == 0 {
		logger.Infof("no test streams at boot (%s=0)", EnvTestStreams)
		return
	}

	if err := a.StartTestStreams(count); err != nil {
		// An Umgebungsfehler: no GStreamer on this machine, or settings whose RTSP
		// publish leg does not validate. The backend runs without the set, and the
		// roster shows what the relay actually carries rather than a promise.
		logger.Warnf("test streams not started: %v", err)
	}
}

// testStreamsAtBootWanted is how many the boot set holds, the default unless the
// environment names another count.
//
// A value that is not a count in range takes the default rather than stopping the
// process: this is a development knob, and a typo in it is not worth a backend that
// refuses to start.
func testStreamsAtBootWanted() int {
	set := os.Getenv(EnvTestStreams)
	if set == "" {
		return testStreamsAtBoot
	}

	count, err := strconv.Atoi(set)
	if err != nil || count < 0 || count > maxTestStreams {
		logger.Warnf("%s=%q is not a count of 0..%d, running %d test streams",
			EnvTestStreams, set, maxTestStreams, testStreamsAtBoot)
		return testStreamsAtBoot
	}
	return count
}

// testStreamLaunch reads everything a launch needs from outside procMu: the settings the
// argv is built from, the launcher a bundle ships and the environment pointing it at the
// plugins beside it.
//
// It is read out here because settingsMu is taken before procMu everywhere (app.go), so
// a launch under the lock could not snapshot the settings itself.
func (a *App) testStreamLaunch() (settings.Settings, string, []string, error) {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	exe, err := publish.FindGstExe()
	if err != nil {
		return s, "", nil, err
	}
	// The launcher a bundle ships runs against the plugins beside it, not against a
	// prefix that exists only on the machine that built the bundle.
	return s, exe, publish.GstChildEnv(), nil
}

// launchTestStreamLocked starts slot i's child as the attempts-th of its streak and
// records it. The caller holds procMu.
//
// The slot is recorded before the child is started, so an exit that beats this return
// blocks on the lock and finds the entry it belongs to rather than an empty slot.
func (a *App) launchTestStreamLocked(i int, attempts int, s settings.Settings, exe string, env []string) error {
	assert.Assert(i >= 0 && i < maxTestStreams, "a test stream holds a slot in the set", i, maxTestStreams)
	assert.Assert(attempts >= 0, "a launch counts the relaunches before it", attempts)
	assert.Assert(a.testStreams[i] == nil, "a slot launches with nothing in it", i)

	name := testStreamName(i)
	args, err := publish.BuildTestStreamArgs(s, name, publish.TestPattern(i))
	if err != nil {
		return err
	}

	slot := &testStream{startedAt: time.Now(), attempts: attempts}
	a.testStreams[i] = slot

	proc, err := ffmpeg.Start(exe, args, true, false, "teststream-"+name, env, nil, nil,
		func(err error, stderrTail string, logPath string) {
			a.testStreamEnded(i, slot, err, stderrTail, logPath)
		})
	if err != nil {
		delete(a.testStreams, i)
		return err
	}
	slot.proc = proc

	return nil
}

// stopTestStreamsLocked turns the set off: it wants none, kills the children and drops
// the relaunches that were pending. The caller holds procMu.
//
// The wanted count is written here rather than by the callers, because it is what makes
// a fired relaunch and a late exit land nowhere: both read it back under the lock.
func (a *App) stopTestStreamsLocked() {
	a.testStreamsWanted = 0

	running := 0
	for i, slot := range a.testStreams {
		if slot.timer != nil {
			slot.timer.Stop()
		}
		if slot.proc != nil {
			slot.proc.Stop()
			running++
		}
		delete(a.testStreams, i)
	}
	if running > 0 {
		logger.Infof("stopped %d test streams", running)
	}

	assert.Assert(len(a.testStreams) == 0, "a stopped set holds no slots", len(a.testStreams))
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

// TestStreamsRunning reports how many synthetic publishers are alive. It is what the
// state event announces and what the contract answers with, so a publisher that died
// on its own drops out of the count without an extra event round-trip, and one waiting
// out its relaunch is not counted as alive.
func (a *App) TestStreamsRunning() int {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	n := 0
	for _, slot := range a.testStreams {
		if slot.proc != nil && slot.proc.Running() {
			n++
		}
	}
	return n
}
