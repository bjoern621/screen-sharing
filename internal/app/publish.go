package app

import (
	"fmt"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// publishRun is one publish session: the settings its pipeline was built from and the
// handle supervising the child that runs it.
//
// A run is replaced whole rather than mutated, which is what lets a callback say
// whether it still describes the publish the app holds. Applying a settings change to
// a live stream kills a child whose last progress sample and whose exit arrive after
// the replacement is already running, and reporting either would take the UI back to a
// stream that is gone.
type publishRun struct {
	settings settings.Settings
	handle   publish.Handle
	// startedAt dates the launch, and attempts counts the retries that preceded it.
	// The exit is weighed against both: how long the pipeline lasted says whether the
	// settings run on this machine, and what the failure has already cost says how much
	// budget is left for it (app_publish_retry.go).
	startedAt time.Time
	attempts  int
}

// PublishCommand returns the exact command line the given settings would run,
// without running it. Shown in the UI for transparency. The engine that owns
// the selected capture backend renders it (ffmpeg command or gst pipeline).
func (a *App) PublishCommand(s settings.Settings) (string, error) {
	return publish.Command(s)
}

// StartPublish validates s, persists it and starts the encoder child.
func (a *App) StartPublish(s settings.Settings) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	if err := settings.Save(s); err != nil {
		logger.Warnf("Cannot persist settings: %v", err)
	}
	return a.startPublish(s)
}

// startPublishHeld publishes on the settings the app holds, for a caller that
// has none of its own to pass: the native grid's publish button acts on what the
// form last wrote, and moves no setting by pressing it.
func (a *App) startPublishHeld() error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	return a.startPublish(s)
}

// startPublish starts the encoder child on s. The settings it runs on are the
// caller's business: this is the one place a publish begins, whether the form or
// the grid asked for it.
func (a *App) startPublish(s settings.Settings) error {
	a.procMu.Lock()
	err := a.startPublishLocked(s)
	a.procMu.Unlock()

	// Announced after the lock is released: reading the state takes both mutexes in
	// the order app.go fixes, and holding one of them here would invert it.
	a.emitPublishState()
	return err
}

// startPublishLocked is the decision itself, with procMu held.
//
// A start naming the pipeline that is already publishing is a request for a state that
// already holds, and a state that already holds is a success (docs/development-principles.md,
// "Effects across a process boundary"): it is what lets a shell whose answer went missing
// ask again rather than wait for one that is not coming. What it is not is a licence to run
// two encoders on one relay path - a start naming a different pipeline is still refused,
// which is the whole of what that refusal was ever for.
//
// Same is decided by publish.SamePipeline, which is where "these two settings are one
// stream" is defined for the pending flag as well. Two definitions of that would be two
// answers to whether a repeat is a repeat.
func (a *App) startPublishLocked(s settings.Settings) error {
	live, _ := a.livePublishLocked()
	if live == nil {
		return a.launchLocked(s, 0)
	}

	same, err := publish.SamePipeline(*live, s)
	if err != nil {
		// One of the two names a pipeline that cannot be built, so whether this start is a
		// repeat cannot be told. Refusing is the answer that cannot start a second encoder.
		logger.Warnf("cannot tell whether the running publish carries the settings this start asks for: %v", err)
		return fmt.Errorf("already publishing")
	}
	if !same {
		return fmt.Errorf("already publishing")
	}

	logger.Debugf("'%s' is already publishing on these settings", s.Publish.Name)
	return nil
}

// livePublishLocked is the publish in force: the settings its pipeline was built from and
// the relaunch pending on it. Both are nil with nothing in force, and the second alone is
// nil while the pipeline is carrying frames. procMu is held by the caller.
//
// It is one function because "what is publishing" is one fact. A start deciding whether it
// is a repeat and a state read deciding what to report are the two consumers, and two
// spellings of this would let them disagree about whether anything is publishing at all.
//
// A publish waiting out its backoff is in force: it is one the user asked for and has not
// stopped, it will come back on its own, and the one call that ends a running pipeline
// ends it too.
func (a *App) livePublishLocked() (*settings.Settings, *publishRetry) {
	if a.run != nil && a.run.handle.Running() {
		s := a.run.settings
		return &s, nil
	}
	if a.retry != nil {
		s := a.retry.settings
		return &s, a.retry
	}
	return nil, nil
}

// Republish validates s, persists it and restarts the running publish on it. It is how
// a settings change reaches a live stream: both engines run a child process built from
// an argv, so a stream carrying other values is another pipeline, and another pipeline
// is a new child.
//
// The settings come from the caller for the reason StartPublish's do: the form writes
// them on a debounce, and a restart on the settings the app happens to hold by then
// would restart the stream onto the edit before the one that was applied.
//
// A launch that fails after the teardown leaves nothing publishing: the pipeline that
// was carrying the stream is gone by then, and there is no earlier one to return to.
func (a *App) Republish(s settings.Settings) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	if err := settings.Save(s); err != nil {
		logger.Warnf("Cannot persist settings: %v", err)
	}

	err := a.restartPublish(s)
	a.emitPublishState()
	return err
}

// restartPublish replaces the running pipeline with one built from s.
//
// The command is rendered before anything is torn down, so a combination no engine can
// build refuses the restart and leaves the stream running what it has.
//
// procMu is held across the teardown and the launch, so nothing reads the window with
// no run in it and the outgoing child's callbacks find the run that replaced them.
//
// The outgoing child is killed and not waited for. The relay closes the publisher it
// holds when a new one connects to the same path, so the successor does not have to
// arrive after the old socket is gone, and viewers reconnect across the gap either way.
//
// A publish waiting out its backoff is restarted the same way, and the settings that
// were failing take their attempts with them: the pipeline the user just named is not
// the one those attempts were spent on.
func (a *App) restartPublish(s settings.Settings) error {
	if _, err := publish.Command(s); err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	running := a.run != nil && a.run.handle.Running()
	if !running && a.retry == nil {
		return fmt.Errorf("nothing is publishing, so there is no pipeline to apply the settings to")
	}
	if running {
		a.run.handle.Stop()
		a.run = nil
	}
	a.cancelRetryLocked()

	logger.Infof("restarting the publish of '%s' on the settings the form holds", s.Publish.Name)
	return a.launchLocked(s, 0)
}

// launchLocked starts the encoder child on s and takes the run it produced. attempts is
// how many retries the app has already spent reaching this launch, zero for one the user
// asked for.
//
// procMu is held by the caller, and the run is in place before the child can report
// anything: a callback that fires first blocks on that lock, so it finds the run it
// belongs to rather than a window with none in it.
func (a *App) launchLocked(s settings.Settings, attempts int) error {
	assert.Assert(a.run == nil || !a.run.handle.Running(), "a publish starts with no other one running", s.Publish.Name)
	assert.Assert(a.retry == nil, "a publish starts with no relaunch pending", s.Publish.Name)
	assert.Assert(attempts >= 0 && attempts <= len(publishBackoff), "a launch carries the retries it cost", attempts)

	pub, err := publish.For(s.Publish.Capture)
	if err != nil {
		return err
	}

	run := &publishRun{settings: s, startedAt: time.Now(), attempts: attempts}
	a.run = run
	handle, err := pub.Start(s, "publish", publish.Callbacks{
		OnStats: func(stats publish.Stats) {
			if !a.isCurrentRun(run) {
				return
			}
			a.emit(wire.PublishStatsEvent(stats))
		},
		OnExit: func(err error, stderrTail string, logPath string) {
			a.publishEnded(run, err, stderrTail, logPath)
		},
	})
	if err != nil {
		a.run = nil
		return err
	}
	run.handle = handle

	logger.Infof("publishing '%s' via %s (%s, %s, %d fps)", s.Publish.Name, s.Publish.Transport, s.Publish.Mode, s.Publish.Chroma, s.Publish.Fps)
	return nil
}

// isCurrentRun reports whether run is still the publish the app holds.
func (a *App) isCurrentRun(run *publishRun) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	return a.run == run
}

// StopPublish ends the publish, whether it is running or waiting out a backoff. A stop
// is the one answer the retry budget does not get a say in: the user asked for no
// stream, and a relaunch arriving seconds later would be one they did not ask for.
func (a *App) StopPublish() {
	a.procMu.Lock()
	if a.run != nil || a.retry != nil {
		if a.run != nil {
			a.run.handle.Stop()
			a.run = nil
		}
		a.cancelRetryLocked()
		logger.Infof("publishing stopped")
	}
	a.procMu.Unlock()

	a.emitPublishState()
}

// GetPublishState reports whether a publish is in force, the settings its pipeline was
// built from, and whether the settings the app now holds build a different one.
//
// It is what a window reads when it mounts and what every change announces, so a fresh
// window and a running one cannot be told different things. The two mutexes are taken
// in the order app.go fixes and neither is held for the comparison, which renders both
// pipelines.
//
// A publish waiting out its backoff reports the settings it will come back on. It is
// still the publish the user asked for and the only one they can stop, so it answers as
// publishing, with Retrying separating the stream that is carrying frames from the one
// that is between attempts.
func (a *App) GetPublishState() PublishState {
	a.settingsMu.Lock()
	held := a.settings
	a.settingsMu.Unlock()

	var state PublishState

	a.procMu.Lock()
	live, retry := a.livePublishLocked()
	if retry != nil {
		state.Retrying = true
		// The budget is set with the attempt because the two are one fact: "attempt 2 of
		// 3" is the whole of what either number says, and a budget reported beside a
		// stream that is carrying frames would name attempts nothing is spending. Both
		// are zero while nothing retries, which is what this shape promises the frontend
		// and what the contract asserts of it (wire.PublishState).
		state.Attempt = retry.attempts
		state.Budget = len(publishBackoff)
	}
	a.procMu.Unlock()

	if live == nil {
		return PublishState{}
	}
	state.Publishing = true
	state.Settings = live

	if same, err := publish.SamePipeline(*live, held); err != nil {
		// One of the two names a pipeline that cannot be built, so what the stream
		// carries cannot be held against what the form shows. What the form shows is
		// then not what is publishing, which is what pending says, and the reason is
		// already in front of the user: the command preview renders through the same
		// call and displays this error.
		logger.Warnf("cannot tell whether the running publish carries the settings the form holds: %v", err)
		state.Pending = true
	} else {
		state.Pending = !same
	}

	// Stated here because this is where the pair is set, and because it is what both
	// consumers go on to assume: the contract asserts it of the snapshot this becomes
	// (wire.PublishState), and the frontend renders "attempt n of m" off it. A state
	// that broke it should fail where it was built rather than at the far end of a
	// conversion that only copied it.
	assert.Assert(state.Retrying || (state.Attempt == 0 && state.Budget == 0),
		"an attempt and a budget belong to a retry", state.Attempt, state.Budget)
	return state
}

// emitPublishState tells both surfaces what the publish state became. The form's own
// toggle knows what it asked for; this carries the changes it did not make, so the
// native grid's publish button cannot leave the form showing a stopped stream, and an
// edit cannot leave it claiming the live stream carries it.
//
// The state is read once and announced twice, rather than read per surface: two reads
// of a state that moved between them would tell the two surfaces different things,
// which is the whole failure this function exists to prevent.
func (a *App) emitPublishState() {
	state := a.GetPublishState()
	a.emit(wire.PublishStateEvent(publishSnapshot(state)))
}

// Publishing reports whether a publish is in force, which a pipeline waiting out its
// backoff still is: the tray and the native grid offer to stop it, and a start while one
// is pending is refused.
func (a *App) Publishing() bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	return (a.run != nil && a.run.handle.Running()) || a.retry != nil
}
