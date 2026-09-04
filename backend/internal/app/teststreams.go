package app

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/wire"
)

// maxTestStreams bounds StartTestStreams: each test stream runs its own x264 encoder,
// so a large count saturates the CPU without testing anything further.
const maxTestStreams = 9

// testStreamsAtBoot is the set this process brings up with itself and keeps alive,
// so the viewer roster carries streams on a machine publishing nothing.
const testStreamsAtBoot = 3

// EnvTestStreams overrides that count for one run, and 0 turns the boot set off.
// The default is not free: one x264 encoder per slot runs for as long as the backend does,
// watched or not.
const EnvTestStreams = "MIRRORME_TEST_STREAMS"

// testStream is one slot of the synthetic set:
// the child publishing it, or the relaunch that child's death armed.
// Exactly one of proc and timer is set, a slot being either running or waiting to run again
// (teststreams_retry.go).
//
// The slot is the stream's identity rather than the process:
// it names the stream (slot 0 is test-1) and picks the pattern,
// so a relaunch comes back as the row the roster shows rather than as a stream beside it.
type testStream struct {
	proc  *ffmpeg.Proc
	timer *time.Timer
	// startedAt dates the launch,
	// attempts counts the relaunches spent on the streak this one belongs to.
	// The exit is weighed against both, the way a publish exit is:
	// a child that ran long enough proves the set works here,
	// and starts the next outage on a full ladder.
	startedAt time.Time
	attempts  int
	// cause is this app's statement about why the slot carries no publisher,
	// message the last child's own last words, both held from the exit that emptied the slot.
	// logPath is that child's run log, which outlives it.
	cause   *screensharev1.Text
	message string
	logPath string
}

// testStreamName is slot i's own name, which this machine's claim leads on the relay
// (settings.StreamName).
//
// The slot number leads, so the roster reads in the order the set was launched,
// and a relaunch comes back under the name it left.
// The surface's own label follows where it has one,
// which lets a viewer pick the stream it wants before anything has decoded:
// "test-2" says nothing about what is in it.
func testStreamName(i int) string {
	assert.Assert(i >= 0, "a test stream holds a slot in the set", i)

	name := "test-" + strconv.Itoa(i+1)
	if label := publish.TestSurfaceOf(i).Label; label != "" {
		name += "-" + label
	}
	return name
}

// StartTestStreams holds the synthetic set at count publishers,
// named test-1..test-<count> under this machine's claim and pushing to the relay over RTSP.
// They exercise the viewing paths without a screen capture:
// the viewer roster, the per-stream viewers, the receive pipelines.
//
// The count is the state the set converges on rather than a transition:
// slots at or above it go, slots below it holding nothing are launched,
// and a slot holding a publisher is left running.
// A second call with the same count starts nothing and stops nothing,
// where replacing the set would cost every viewer of it a reconnect for a state that held.
//
// The set is kept alive rather than only started:
// a publisher that dies on its own is relaunched into the slot it held (teststreams_retry.go),
// which makes the boot set survive a relay that comes up after this process does.
func (a *App) StartTestStreams(count int) error {
	if count <= 0 || count > maxTestStreams {
		return fmt.Errorf("test stream count must be 1..%d, got %d", maxTestStreams, count)
	}
	if !a.settingsInGroup() {
		return errNoGroup
	}

	s, exe, env, err := a.testStreamLaunch()
	if err != nil {
		return err
	}

	// Announced outside the lock, for the reason StartWatch states:
	// the count is read back through a method that takes the same mutex.
	defer a.emitTestStreamState()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	// The wanted count moves first,
	// so an exit firing while the surplus is killed reads the count this call asked for,
	// rather than the one it replaced (teststreams_retry.go).
	a.testStreamsWanted = count
	a.dropTestStreamsAboveLocked(count)

	launched := 0
	for i := range count {
		if a.testStreams[i] != nil {
			continue
		}
		if err := a.launchTestStreamLocked(i, 0, s, exe, env); err != nil {
			a.stopTestStreamsLocked()
			return err
		}
		launched++
	}

	assert.Assert(len(a.testStreams) == count, "the set holds a slot per wanted stream", len(a.testStreams), count)
	if launched > 0 {
		logger.Infof("test streams: %d of %d slots launched, the rest were already up", launched, count)
	}
	return nil
}

// dropTestStreamsAboveLocked takes every slot from count upward off the set,
// killing a child and dropping a pending relaunch alike.
// The caller holds procMu and has written the wanted count.
//
// The entry goes before the child's exit can arrive,
// so that exit finds a slot the app moved off and lands nowhere,
// rather than arming a relaunch into a set that does not hold it.
func (a *App) dropTestStreamsAboveLocked(count int) {
	assert.Assert(count >= 0, "a set is held at a count of slots", count)

	stopped := 0
	for i, slot := range a.testStreams {
		if i < count {
			continue
		}
		if slot.timer != nil {
			slot.timer.Stop()
		}
		if slot.proc != nil {
			slot.proc.Stop()
			stopped++
		}
		delete(a.testStreams, i)
	}
	if stopped > 0 {
		logger.Infof("stopped %d test streams above slot %d", stopped, count)
	}
}

// StopTestStreams stops every synthetic publisher, and the set stays off until it is asked for again:
// the wanted count goes to zero with the children,
// so the relaunches this stop cancels are not re-armed behind it.
func (a *App) StopTestStreams() {
	defer a.emitTestStreamState()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	a.stopTestStreamsLocked()
}

// startTestStreamsAtBoot brings the always-on set up beside the process.
//
// On a goroutine of its own for the reason everything App.Start begins is:
// resolving the launcher touches the disk and each child opens its run log,
// and the contract comes up at its own speed rather than at theirs.
func (a *App) startTestStreamsAtBoot() {
	count := testStreamsAtBootWanted()
	if count == 0 {
		logger.Infof("no test streams at boot (%s=0)", EnvTestStreams)
		return
	}

	// Presence before the first publisher, because a token names this machine's member id
	// and the group service closes a connection no live member holds (internal/membership).
	// The poll loop states the same thing on its own clock,
	// so this is the boot set naming the state it needs rather than racing that loop for it.
	a.statePresence()

	if err := a.StartTestStreams(count); err != nil {
		// An Umgebungsfehler: no GStreamer on this machine,
		// or settings whose RTSP publish leg does not validate.
		// The backend runs without the set,
		// and the roster shows what the relay carries rather than a promise.
		logger.Warnf("test streams not started: %v", err)
	}
}

// settingsInGroup reports whether the held settings state membership of a group.
// Read under settingsMu like every other use of them (app.go).
func (a *App) settingsInGroup() bool {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()
	// Brokered because Discord mode's membership never rides the stored copy (discord.go).
	return a.withBrokered(s).Relay.InGroup()
}

// joinedAGroup reports whether a settings write moved this machine into a group.
//
// What the boot set is launched again off (settings.go, SaveSettings):
// a machine that joins can publish what it was refused a moment earlier,
// and the set is always-on rather than something a user asks for per run.
func joinedAGroup(before, after settings.Relay) bool {
	return !before.InGroup() && after.InGroup()
}

// testStreamsAtBootWanted is how many slots the boot set holds:
// testStreamsAtBoot unless the environment names another count.
//
// A value that is not a count in range takes the default rather than stopping the process.
// A development knob, and a typo in it is not worth a backend that refuses to start.
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

// testStreamLaunch reads everything a launch needs from outside procMu:
// the settings the argv is built from, the launcher a bundle ships,
// and the environment pointing it at the plugins beside it.
//
// Out here because settingsMu is taken before procMu everywhere (app.go),
// so a launch under procMu could not snapshot the settings itself.
func (a *App) testStreamLaunch() (settings.Settings, string, []string, error) {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	// A synthetic stream publishes to the same relay as a real one, and is refused alike.
	s, err := a.settingsForCommand(s)
	if err != nil {
		return s, "", nil, err
	}

	exe, err := publish.FindGstExe()
	if err != nil {
		return s, "", nil, err
	}
	// The launcher a bundle ships runs against the plugins beside it,
	// not against a prefix that exists only on the machine that built the bundle.
	return s, exe, publish.GstChildEnv(), nil
}

// launchTestStreamLocked starts slot i's child as the attempts-th of its streak and records it.
// The caller holds procMu.
//
// The slot is recorded before the child is started,
// so an exit beating this return blocks on the lock,
// and finds the entry it belongs to rather than an empty slot.
func (a *App) launchTestStreamLocked(i int, attempts int, s settings.Settings, exe string, env []string) error {
	assert.Assert(i >= 0 && i < maxTestStreams, "a test stream holds a slot in the set", i, maxTestStreams)
	assert.Assert(attempts >= 0, "a launch counts the relaunches before it", attempts)
	assert.Assert(a.testStreams[i] == nil, "a slot launches with nothing in it", i)

	name := testStreamName(i)
	args, err := publish.BuildTestStreamArgs(s, name, publish.TestSurfaceOf(i))
	if err != nil {
		return err
	}

	slot := &testStream{startedAt: time.Now(), attempts: attempts}
	a.testStreams[i] = slot

	proc, err := ffmpeg.Start(exe, args, true, false, "teststream-"+name, env, nil, nil,
		func(err error, stderrTail string, logPath string) {
			a.testStreamEnded(i, slot, err, stderrTail, logPath)
		},
		// A test stream publishes to the relay like any other,
		// so its command line carries the same credentials.
		ffmpeg.WithRedactor(func(text string) string { return transport.Redact(s, text) }))
	if err != nil {
		delete(a.testStreams, i)
		return err
	}
	slot.proc = proc

	return nil
}

// stopTestStreamsLocked turns the set off:
// it wants none, kills the children and drops the relaunches that were pending.
// The caller holds procMu.
//
// The wanted count is written here rather than by the callers,
// it being what makes a fired relaunch and a late exit land nowhere: both read it back under the lock.
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

// emitTestStreamState announces the synthetic set.
//
// Read back through TestStreamState rather than handed a set,
// so what is announced is what a read would answer:
// a publisher that died between the call and this is out of the count.
// That read takes procMu, so a caller holding it defers this rather than calling it in place.
func (a *App) emitTestStreamState() {
	running, slots := a.TestStreamState()
	a.emit(wire.TestStreamStateEvent(running, slots...))
}

// TestStreamState is the synthetic set:
// how many publishers are alive, and a row per slot the set holds.
//
// One read for both, a count read apart from the rows being a second answer to one question.
// A publisher that died on its own is out of the count with nothing having been called,
// and its slot stays on the list saying why it carries none.
//
// The rows come back in slot order, the order the set was launched in and the names run in.
func (a *App) TestStreamState() (running int, slots []wire.TestStreamSlot) {
	// The name a row carries is the one a launch publishes under, claim and all,
	// so it is derived from the settings rather than from the slot number alone.
	// Snapshotted first because settingsMu is taken before procMu everywhere (app.go).
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	slots = make([]wire.TestStreamSlot, 0, len(a.testStreams))
	for i := range maxTestStreams {
		slot := a.testStreams[i]
		if slot == nil {
			continue
		}
		alive := slot.proc != nil && slot.proc.Running()
		if alive {
			running++
		}
		// A launch takes a fresh entry,
		// so what the last child left behind belongs to the slot waiting for the next one (teststreams_retry.go).
		assert.Assert(!alive || slot.cause == nil, "a slot a publisher fills carries no reason to have none", i)
		slots = append(slots, wire.TestStreamSlot{
			Slot: i,
			Name: s.WithStreamName(testStreamName(i)).StreamName(),
			// A slot waiting out its relaunch is one the set holds and no child is filling.
			Running: alive,
			// Counted from one, where the slot counts the relaunches behind it.
			Attempt: slot.attempts + 1,
			Cause:   slot.cause,
			Message: slot.message,
			LogPath: slot.logPath,
		})
	}

	assert.Assert(len(slots) == len(a.testStreams), "a row per slot the set holds", len(slots), len(a.testStreams))
	assert.Assert(running <= len(slots), "a live publisher fills a slot", running, len(slots))
	return running, slots
}
