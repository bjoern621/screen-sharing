package main

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/publish"
	"bjoernblessin.de/screenshare/settings"
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
	settings settings.Stream
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
func (a *App) PublishCommand(s settings.Stream) (string, error) {
	return publish.Command(s)
}

// StartPublish validates s, persists it and starts the encoder child.
func (a *App) StartPublish(s settings.Stream) error {
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
func (a *App) startPublish(s settings.Stream) error {
	a.procMu.Lock()
	// A publish waiting out its backoff is one the user asked for and has not stopped,
	// so it holds the same ground a running one does.
	if (a.run != nil && a.run.handle.Running()) || a.retry != nil {
		a.procMu.Unlock()
		return fmt.Errorf("already publishing")
	}
	err := a.launchLocked(s, 0)
	a.procMu.Unlock()

	// Announced after the lock is released: reading the state takes both mutexes in
	// the order app.go fixes, and holding one of them here would invert it.
	a.emitPublishState()
	return err
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
func (a *App) Republish(s settings.Stream) error {
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
func (a *App) restartPublish(s settings.Stream) error {
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

	logger.Infof("restarting the publish of '%s' on the settings the form holds", s.Name)
	return a.launchLocked(s, 0)
}

// launchLocked starts the encoder child on s and takes the run it produced. attempts is
// how many retries the app has already spent reaching this launch, zero for one the user
// asked for.
//
// procMu is held by the caller, and the run is in place before the child can report
// anything: a callback that fires first blocks on that lock, so it finds the run it
// belongs to rather than a window with none in it.
func (a *App) launchLocked(s settings.Stream, attempts int) error {
	assert.Assert(a.run == nil || !a.run.handle.Running(), "a publish starts with no other one running", s.Name)
	assert.Assert(a.retry == nil, "a publish starts with no relaunch pending", s.Name)
	assert.Assert(attempts >= 0 && attempts <= len(publishBackoff), "a launch carries the retries it cost", attempts)

	pub, err := publish.For(s.Capture)
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
			runtime.EventsEmit(a.ctx, "publish:stats", stats)
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

	logger.Infof("publishing '%s' via %s (%s, %s, %d fps)", s.Name, s.Transport, s.Mode, s.Chroma, s.Fps)
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
func (a *App) GetPublishState() PublishStateEvent {
	a.settingsMu.Lock()
	held := a.settings
	a.settingsMu.Unlock()

	a.procMu.Lock()
	var live *settings.Stream
	state := PublishStateEvent{Budget: len(publishBackoff)}
	if a.run != nil && a.run.handle.Running() {
		s := a.run.settings
		live = &s
	} else if a.retry != nil {
		s := a.retry.settings
		live = &s
		state.Retrying = true
		state.Attempt = a.retry.attempts
	}
	a.procMu.Unlock()

	if live == nil {
		return PublishStateEvent{}
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
	return state
}

// emitPublishState tells the frontend what the publish state became. The form's own
// toggle knows what it asked for; this carries the changes it did not make, so the
// native grid's publish button cannot leave the form showing a stopped stream, and an
// edit cannot leave it claiming the live stream carries it.
func (a *App) emitPublishState() {
	runtime.EventsEmit(a.ctx, "publish:state", a.GetPublishState())
}

// Publishing reports whether a publish is in force, which a pipeline waiting out its
// backoff still is: the tray and the native grid offer to stop it, and a start while one
// is pending is refused.
func (a *App) Publishing() bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	return (a.run != nil && a.run.handle.Running()) || a.retry != nil
}
