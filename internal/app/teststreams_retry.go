package app

import (
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/wire"
)

// testStreamBackoff is the wait before each relaunch of a synthetic publisher that died
// on its own, and its last entry is the wait the slot keeps: the set is always-on, so
// there is no attempt budget for it to spend.
//
// A publish gives up because settings this machine cannot run fail the same way every
// time (publish_retry.go). The synthetic set is the other case: it runs one fixed
// pipeline that either works here or never did, and the failure it waits out is
// usually the relay, which this process starts before and outlives. Giving up would
// leave the roster empty for the rest of the run over an outage that ended a minute in.
var testStreamBackoff = []time.Duration{
	2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second,
}

// testStreamHealthy is how long a publisher has to have run for its slot to count as
// working on this machine. One that reaches it met something that moved underneath it
// rather than a pipeline that cannot start, so the next outage begins on the first
// delay instead of the one the last streak walked up to.
const testStreamHealthy = 30 * time.Second

// testStreamWait reports how long slot i waits before the relaunch that follows
// attempts relaunches already spent. The ladder is walked once and then held, because
// there is no attempt at which the set stops being wanted.
func testStreamWait(attempts int) time.Duration {
	assert.Assert(attempts >= 0, "a relaunch counts the attempts before it", attempts)

	if attempts >= len(testStreamBackoff) {
		return testStreamBackoff[len(testStreamBackoff)-1]
	}
	return testStreamBackoff[attempts]
}

// testStreamEnded takes the exit of one slot's child: it releases the slot, arms the
// relaunch while the set still wants it, and reports the failure once per outage.
//
// slot is the entry the dead child was launched into, and it is held against what the
// map carries now for the reason publishEnded holds its run: a stop and a restart both
// replace the entry, so an exit arriving after either belongs to a stream the app has
// already moved off.
func (a *App) testStreamEnded(i int, slot *testStream, err error, stderrTail string, logPath string) {
	assert.IsNotNil(slot, "an exit belongs to the slot that produced it")

	name := testStreamName(i)
	if err == nil {
		logger.Infof("test stream %s closed (log: %s)", name, logPath)
	} else {
		logger.Warnf("test stream %s failed: %v\n%s\nfull log: %s", name, err, stderrTail, logPath)
	}

	message := ""
	if err != nil {
		message = err.Error()
		if stderrTail != "" {
			message += "\n" + stderrTail
		}
	}

	a.procMu.Lock()
	if a.testStreams[i] != slot {
		a.procMu.Unlock()
		return
	}
	delete(a.testStreams, i)

	spent := slot.attempts
	if time.Since(slot.startedAt) >= testStreamHealthy {
		spent = 0
	}
	relaunching := i < a.testStreamsWanted
	if relaunching {
		a.armTestStreamLocked(i, spent)
	}
	a.procMu.Unlock()

	// One sentence per outage rather than one per attempt. The ladder above never gives
	// up, so a relay that stays down would otherwise write a line into the session log
	// every thirty seconds for the rest of the run. spent being zero is what marks the
	// exit as the start of a streak: a slot that had already been relaunched is still
	// inside the outage its first exit reported.
	if err == nil || spent == 0 {
		// The exit says why this one stopped; the count beside it says how many are
		// left. A publisher that died on its own moves the count with nothing having
		// been called, which is the case the state event exists for.
		a.emit(wire.TestStreamExitEvent(message, logPath))
	}
	a.emitTestStreamState()
}

// armTestStreamLocked schedules slot i's relaunch as the attempt after spent, firing
// once the ladder's wait has passed. The caller holds procMu.
//
// The waiting slot is placed before the timer is armed, so a fire that beats this call's
// return blocks on the lock and finds the slot it belongs to rather than a window with
// none. A slot that is waiting is still a slot the set holds, which is what keeps a
// second launch out of it.
func (a *App) armTestStreamLocked(i int, spent int) {
	assert.Assert(spent >= 0, "a relaunch counts the attempts before it", spent)
	assert.Assert(a.testStreams[i] == nil, "a slot arms its relaunch with nothing in it", i)

	wait := testStreamWait(spent)
	slot := &testStream{attempts: spent + 1}
	a.testStreams[i] = slot
	slot.timer = time.AfterFunc(wait, func() { a.fireTestStreamRetry(i, slot) })

	logger.Infof("relaunch %d of test stream %s in %s", slot.attempts, testStreamName(i), wait)
}

// fireTestStreamRetry relaunches the slot the relaunch holds, unless the set has moved
// off it: a stop, a restart and a shutdown each replace or drop the entry this is held
// against.
//
// A relaunch that cannot start re-arms rather than ending the slot's life, for the same
// reason the ladder holds: a launcher that is missing now is the machine's state and not
// the set's, and the set is wanted until something says otherwise.
func (a *App) fireTestStreamRetry(i int, slot *testStream) {
	assert.IsNotNil(slot, "a fired relaunch is the slot that armed it")

	s, exe, env, err := a.testStreamLaunch()

	a.procMu.Lock()
	if a.testStreams[i] != slot {
		a.procMu.Unlock()
		logger.Debugf("dropped a stale test stream relaunch for %s", testStreamName(i))
		return
	}
	delete(a.testStreams, i)

	if err == nil {
		err = a.launchTestStreamLocked(i, slot.attempts, s, exe, env)
	}
	if err != nil {
		logger.Warnf("relaunch %d of test stream %s did not start: %v", slot.attempts, testStreamName(i), err)
		a.armTestStreamLocked(i, slot.attempts)
	}
	a.procMu.Unlock()

	a.emitTestStreamState()
}
