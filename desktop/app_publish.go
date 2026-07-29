package main

import (
	"fmt"

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
	if a.run != nil && a.run.handle.Running() {
		a.procMu.Unlock()
		return fmt.Errorf("already publishing")
	}
	err := a.launchLocked(s)
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
func (a *App) restartPublish(s settings.Stream) error {
	if _, err := publish.Command(s); err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.run == nil || !a.run.handle.Running() {
		return fmt.Errorf("nothing is publishing, so there is no pipeline to apply the settings to")
	}
	a.run.handle.Stop()
	a.run = nil

	logger.Infof("restarting the publish of '%s' on the settings the form holds", s.Name)
	return a.launchLocked(s)
}

// launchLocked starts the encoder child on s and takes the run it produced.
//
// procMu is held by the caller, and the run is in place before the child can report
// anything: a callback that fires first blocks on that lock, so it finds the run it
// belongs to rather than a window with none in it.
func (a *App) launchLocked(s settings.Stream) error {
	assert.Assert(a.run == nil || !a.run.handle.Running(), "a publish starts with no other one running", s.Name)

	pub, err := publish.For(s.Capture)
	if err != nil {
		return err
	}

	run := &publishRun{settings: s}
	a.run = run
	handle, err := pub.Start(s, "publish", publish.Callbacks{
		OnStats: func(stats publish.Stats) {
			if !a.isCurrentRun(run) {
				return
			}
			runtime.EventsEmit(a.ctx, "publish:stats", stats)
		},
		OnExit: func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
				logger.Errorf("publish of '%s' failed: %v\n%s\nfull log: %s", s.Name, err, stderrTail, logPath)
			} else {
				logger.Infof("publish of '%s' ended (log: %s)", s.Name, logPath)
			}
			// The event reports the run the app still holds, which is the run nobody
			// asked to end. A stop was asked for and a restart replaced the pipeline
			// with one that is already running, so either would carry an exit the user
			// has no reason to be shown and a log path for a pipeline they moved off.
			if !a.dropRun(run) {
				return
			}
			runtime.EventsEmit(a.ctx, "publish:exit", exitEvent{Message: message, LogPath: logPath})
			a.emitPublishState()
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

// dropRun releases run if it is still the publish the app holds, and reports whether
// it was. A run that has ended holds nothing worth keeping: the settings on it describe
// a pipeline that is gone, and the state event answers with them while they are there.
func (a *App) dropRun(run *publishRun) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.run != run {
		return false
	}
	a.run = nil
	return true
}

func (a *App) StopPublish() {
	a.procMu.Lock()
	if a.run != nil {
		a.run.handle.Stop()
		a.run = nil
		logger.Infof("publishing stopped")
	}
	a.procMu.Unlock()

	a.emitPublishState()
}

// GetPublishState reports whether a publish is running, the settings its pipeline was
// built from, and whether the settings the app now holds build a different one.
//
// It is what a window reads when it mounts and what every change announces, so a fresh
// window and a running one cannot be told different things. The two mutexes are taken
// in the order app.go fixes and neither is held for the comparison, which renders both
// pipelines.
func (a *App) GetPublishState() PublishStateEvent {
	a.settingsMu.Lock()
	held := a.settings
	a.settingsMu.Unlock()

	a.procMu.Lock()
	var running *settings.Stream
	if a.run != nil && a.run.handle.Running() {
		s := a.run.settings
		running = &s
	}
	a.procMu.Unlock()

	if running == nil {
		return PublishStateEvent{}
	}

	pending := false
	if same, err := publish.SamePipeline(*running, held); err != nil {
		// One of the two names a pipeline that cannot be built, so what the stream
		// carries cannot be held against what the form shows. What the form shows is
		// then not what is publishing, which is what pending says, and the reason is
		// already in front of the user: the command preview renders through the same
		// call and displays this error.
		logger.Warnf("cannot tell whether the running publish carries the settings the form holds: %v", err)
		pending = true
	} else {
		pending = !same
	}
	return PublishStateEvent{Publishing: true, Settings: running, Pending: pending}
}

// emitPublishState tells the frontend what the publish state became. The form's own
// toggle knows what it asked for; this carries the changes it did not make, so the
// native grid's publish button cannot leave the form showing a stopped stream, and an
// edit cannot leave it claiming the live stream carries it.
func (a *App) emitPublishState() {
	runtime.EventsEmit(a.ctx, "publish:state", a.GetPublishState())
}

func (a *App) Publishing() bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	return a.run != nil && a.run.handle.Running()
}
