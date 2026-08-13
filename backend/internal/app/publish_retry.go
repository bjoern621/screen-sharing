package app

import (
	"fmt"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// publishBackoff is the wait before each relaunch of a publish that ended on its own, one entry per
// attempt, so its length is the budget.
// It grows because a publish pipeline usually dies by itself over a relay restart or a capture source
// going away, which take seconds rather than milliseconds.
// It ends because a pipeline this machine cannot run fails the same way every time, and an encoder
// that takes the GPU down with it does so once per attempt.
var publishBackoff = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

// publishHealthy is how long a pipeline has to have run for its settings to count as viable on this
// machine.
//
// The discriminator the exit alone cannot give: a relay that is not up and an encoder that hangs the
// GPU both leave a child dead within seconds, under the same signal and the same status, so how far
// the pipeline got is what separates them.
// A publish that reaches this bound and dies later met something that moved underneath it, and meets
// the next outage on a full budget.
// One that does not is failing at launch, and its attempts are the whole of what the app will try.
const publishHealthy = 30 * time.Second

// publishRetry is a relaunch waiting to happen: the settings it will run, the attempts spent
// reaching it, and the timer that fires it.
//
// It stands in for the run while it waits, because a publish between attempts is still one the user
// asked for: the state reports publishing rather than stopped, and a stop reaches the timer rather
// than finding nothing to stop.
type publishRetry struct {
	settings settings.Settings
	attempts int
	timer    *time.Timer
}

// publishRetryAfter is the whole retry policy: how much of the budget the failure has spent, how
// long to wait before the relaunch, and whether there is one.
//
// err is the exit, nil for a pipeline that ended without failing and is therefore not retried.
// ran is how long that pipeline lasted, attempts how many retries preceded it.
//
// A pipeline that reached publishHealthy resets the count it inherited, so a working stream meets
// every outage on a full budget while one failing at launch walks the backoff once and stops.
func publishRetryAfter(err error, ran time.Duration, attempts int) (spent int, wait time.Duration, retry bool) {
	assert.Assert(attempts >= 0, "an exit carries the retries that preceded it", attempts)

	if err == nil {
		return 0, 0, false
	}
	spent = attempts
	if ran >= publishHealthy {
		spent = 0
	}
	if spent >= len(publishBackoff) {
		return spent, 0, false
	}
	return spent, publishBackoff[spent], true
}

// publishEnded takes the exit of run: it releases the run, decides whether the failure has another
// attempt coming, and announces whichever state the app lands in.
//
// Only an exit nobody asked for reaches a retry.
// A stop and a settings change kill the child themselves and replace what the app holds, so the run
// they ended is no longer the app's by the time this reads it.
func (a *App) publishEnded(run *publishRun, err error, stderrTail string, logPath string) {
	assert.IsNotNil(run, "an exit belongs to the run that produced it")

	if err == nil {
		logger.Infof("publish of '%s' ended (log: %s)", run.settings.Publish.Name, logPath)
	} else {
		logger.Warnf("publish of '%s' failed: %v\n%s\nfull log: %s", run.settings.Publish.Name, err, stderrTail, logPath)
	}

	message := ""
	if err != nil {
		message = err.Error()
		if stderrTail != "" {
			message += "\n" + stderrTail
		}
	}

	a.procMu.Lock()
	if a.run != run {
		// The exit names a run the app has already moved off: a stop was asked for, or a settings change
		// replaced the pipeline with one that is already running.
		// Reporting it would carry an exit and a log path for a pipeline the user left behind.
		a.procMu.Unlock()
		return
	}
	a.run = nil
	// The position was read over the capture that just ended, and the rectangle it was mapped against
	// went with that child: holding it across a backoff would draw a pointer over a stopped stream.
	a.pointerAt.clear()
	// The child copying to it is gone, so the preview goes too, across a retry as well: the relaunch
	// binds a port of its own.
	// A publish between attempts therefore reports no preview.
	a.stopPreviewLocked()

	spent, wait, retrying := publishRetryAfter(err, time.Since(run.startedAt), run.attempts)
	if retrying {
		a.scheduleRetryLocked(run.settings, spent, wait)
	}
	a.procMu.Unlock()

	if !retrying {
		if err != nil && spent > 0 {
			message = fmt.Sprintf("%s (gave up after %d retries)", message, spent)
		}
		a.emit(wire.PublishExitEvent(message, logPath))
	}
	a.emitPublishState()
}

// scheduleRetryLocked arms the relaunch of s as attempt spent+1, firing once wait has passed.
// procMu is held by the caller.
//
// The retry is placed before the timer is armed, so a fire that beats this call's return blocks on
// the lock and finds the retry it belongs to rather than a window with none.
func (a *App) scheduleRetryLocked(s settings.Settings, spent int, wait time.Duration) {
	assert.Assert(spent >= 0 && spent < len(publishBackoff), "a publish retry indexes the backoff", spent, len(publishBackoff))
	assert.Assert(a.retry == nil, "a publish schedules its retry with none pending", s.Publish.Name)

	r := &publishRetry{settings: s, attempts: spent + 1}
	a.retry = r
	r.timer = time.AfterFunc(wait, func() { a.firePublishRetry(r) })

	logger.Infof("retry %d of %d for the publish of '%s' in %s", r.attempts, len(publishBackoff), s.Publish.Name, wait)
}

// firePublishRetry relaunches the publish r holds, unless the app has moved off it.
// A retry firing after a stop, a settings change or a manual start lands nowhere, because each of
// those clears what it is held against.
func (a *App) firePublishRetry(r *publishRetry) {
	assert.IsNotNil(r, "a fired retry is the relaunch that armed it")

	a.procMu.Lock()
	if a.retry != r {
		a.procMu.Unlock()
		logger.Debugf("dropped a stale publish retry for '%s'", r.settings.Publish.Name)
		return
	}
	a.retry = nil
	err := a.launchLocked(r.settings, r.attempts)
	a.procMu.Unlock()

	if err != nil {
		// The child never started, so no exit is coming to carry the chain further, and it ends here
		// rather than on a budget nothing is spending.
		logger.Warnf("retry %d of the publish of '%s' did not start: %v", r.attempts, r.settings.Publish.Name, err)
		a.emit(wire.PublishExitEvent(err.Error(), ""))
	}
	a.emitPublishState()
}

// cancelRetryLocked drops a pending relaunch, and succeeds where none is pending.
// procMu is held by the caller.
//
// Stopping the timer is not what makes it safe: a fire already past its guard is blocked on procMu
// and finds the retry cleared here, which is what turns it into a no-op.
// The stop only keeps a timer that has not fired from firing.
func (a *App) cancelRetryLocked() {
	if a.retry == nil {
		return
	}
	a.retry.timer.Stop()
	a.retry = nil
}
