package app

import (
	"fmt"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// publishRun is one publish session: the settings its pipeline was built from, and the handle
// supervising the child that runs it.
//
// A run is replaced whole rather than mutated, which is what lets a callback tell whether it still
// describes the publish the app holds.
// A settings change kills a child whose last progress sample and whose exit arrive after the
// replacement is already running, and reporting either would take a shell back to a stream that is
// gone.
type publishRun struct {
	settings settings.Settings
	handle   publish.Handle
	// monitor is the output the pipeline crops to, enumerated once at launch.
	// The crop is fixed in the child's argv, so this is what the frames carry however the outputs are
	// arranged afterwards, and it is what a pointer position is mapped against (pointer.go).
	// monitorKnown is false where the enumeration named no output at that index.
	monitor      display.Monitor
	monitorKnown bool
	// startedAt dates the launch and attempts counts the retries before it.
	// The exit is weighed against both: how long the pipeline lasted says whether these settings run
	// on this machine, and what the failure has cost says how much budget is left (publish_retry.go).
	startedAt time.Time
	attempts  int
	// delay is what the newest sample measured about this leg's own share of the path between a
	// screen and a window, held under procMu like the rest of the run.
	//
	// A departure from deriving on demand, and the reason is that there is nothing to derive from:
	// the child pushes a sample once a second and answers no question in between, so the alternative
	// to holding the newest reading is a budget that has the publishing stages only on the ticks the
	// two clocks happen to land on together.
	// The one consumer is the budget of a decode of this same stream (receivestats.go); the sample
	// itself goes out on the event stream unchanged and is nobody's copy.
	delay publishDelay
}

// publishDelay is the publishing side's share of the path, in milliseconds, as the newest sample
// measured it.
// Both are absent on an engine that measures neither, and Link alone is absent on a transport that
// states no delivery window (internal/ffmpeg, Stats).
type publishDelay struct {
	Transit *float64
	Link    *float64
}

// PublishCommand is the command line s would run, without running it.
// The engine owning the selected capture backend renders it: an ffmpeg command or a gst pipeline.
func (a *App) PublishCommand(s settings.Settings) (string, error) {
	return publish.Command(s)
}

// StartPublish persists s and starts the encoder child on it.
// Persisting is best effort: a store that cannot be written is an Umgebungsfehler and does not cost
// the stream.
func (a *App) StartPublish(s settings.Settings) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	if err := settings.Save(s); err != nil {
		logger.Warnf("Cannot persist settings: %v", err)
	}
	// The held settings moved, so every shell is told, not just the one that pressed the button.
	// A shell that hears nothing keeps the draft it was showing and writes it back over this one on its
	// next save (settings.go, SaveSettings).
	a.emit(wire.SettingsChangedEvent())
	return a.startPublish(s)
}

// startPublish is the one place a publish begins, whichever caller chose the settings.
func (a *App) startPublish(s settings.Settings) error {
	// Attached here rather than in the callers, so every publish carries the same credential
	// (groups.go).
	s, err := a.settingsForCommand(s)
	if err != nil {
		return err
	}

	a.procMu.Lock()
	err = a.startPublishLocked(s)
	a.procMu.Unlock()

	// Announced with the lock released: reading the state takes both mutexes in the order app.go
	// fixes, and holding one here would invert it.
	a.emitPublishState()
	return err
}

// startPublishLocked is the decision itself, with procMu held.
//
// A start naming the pipeline already publishing asks for a state that already holds, so it succeeds
// and starts nothing (docs/development-principles.md, "Effects across a process boundary"): a shell
// whose answer went missing can ask again rather than wait for one that is not coming.
// A start naming a different pipeline is still refused, which keeps two encoders off one relay path.
//
// publish.SamePipeline decides sameness, and decides it for the pending flag too.
// A second definition would be a second answer to whether a repeat is a repeat.
func (a *App) startPublishLocked(s settings.Settings) error {
	live, _ := a.livePublishLocked()
	if live == nil {
		return a.launchLocked(s, 0)
	}

	same, err := publish.SamePipeline(*live, s)
	if err != nil {
		// One of the two names a pipeline that cannot be built, so whether this start is a repeat is
		// unanswerable.
		// Refusing is the answer that cannot start a second encoder.
		logger.Warnf("cannot tell whether the running publish carries the settings this start asks for: %v", err)
		return fmt.Errorf("already publishing")
	}
	if !same {
		return fmt.Errorf("already publishing")
	}

	logger.Debugf("'%s' is already publishing on these settings", s.Publish.Name)
	return nil
}

// livePublishLocked is the publish in force: the settings its pipeline was built from, and the
// relaunch pending on it.
// Both nil with nothing in force, the second alone nil while the pipeline carries frames.
// procMu is held by the caller.
//
// One function because "what is publishing" is one fact.
// Its consumers are a start deciding whether it is a repeat and a state read deciding what to
// report, and two spellings would let them disagree about whether anything publishes at all.
//
// A publish waiting out its backoff is in force: the user asked for it and has not stopped it, it
// comes back on its own, and the call that ends a running pipeline ends it too.
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

// publish.ReleaseSources, counted instead of performed by a test.
var releaseSources = publish.ReleaseSources

// releaseSourcesLocked drops the screen source a capture backend holds between launches, where no
// publish is left in force to hold it for.
// procMu is held by the caller.
//
// The guard is what lets every path that ends a child call it.
// A relaunch and a retry both pass through such a path with the stream still in force, and a source
// dropped there pops the compositor's picker on the launch that follows, which is the whole of what
// holding it avoids (publish.ReleaseSources).
func (a *App) releaseSourcesLocked() {
	if live, _ := a.livePublishLocked(); live != nil {
		return
	}
	releaseSources()
}

// Republish persists s and puts the running publish on it.
// A change the running pipeline takes is written to the child and every viewer keeps watching; one
// it does not take replaces the child.
//
// The settings come from the caller, as StartPublish's do: the form writes them on a debounce, and a
// restart on whatever the app happens to hold by then would run the edit before the applied one.
//
// A launch that fails after the teardown leaves nothing publishing, because the pipeline carrying
// the stream is already gone and there is no earlier one to return to.
func (a *App) Republish(s settings.Settings) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	if err := settings.Save(s); err != nil {
		logger.Warnf("Cannot persist settings: %v", err)
	}
	// Announced for the reason StartPublish announces it.
	a.emit(wire.SettingsChangedEvent())

	err := a.restartPublish(s)
	a.emitPublishState()
	return err
}

// applyLiveLocked writes s to the running child where the change is one it takes, and reports
// whether it did.
// procMu is held by the caller.
//
// Two questions decide it, neither of them a list of field names: whether this engine's child takes
// values while it plays (publish.Live), and whether the change touches nothing outside what such a
// child takes (publish.LiveOnly).
// Both answers come off the one live table, so a form marking a control live and an apply that
// avoids the relaunch cannot disagree.
//
// The run keeps its handle, its start time and its attempts, because the child never restarted: what
// moved is the values it holds.
// Mutating the run in place says so, and keeps the child's callbacks pointing at the run they belong
// to.
//
// A failed write leaves the caller to relaunch.
// The socket is the only way to reach a playing child, so a write that failed is a child that cannot
// be told anything, and reporting the apply as done would leave the stream on values nobody chose.
func (a *App) applyLiveLocked(s settings.Settings) bool {
	applier, live := publish.Live(a.run.handle)
	if !live {
		return false
	}
	only, err := publish.LiveOnly(a.run.settings, s)
	if err != nil || !only {
		return false
	}
	if err := applier.ApplyLive(s); err != nil {
		logger.Warnf("the running pipeline did not take the change, so it is relaunched onto it: %v", err)
		return false
	}

	a.run.settings = s
	logger.Infof("applied the change to the running publish of '%s' without relaunching it", s.Publish.Name)
	return true
}

// restartPublish puts the running pipeline on s: a write where the child takes the change, a
// replacement where it does not.
//
// The write is tried first and what it saves is the teardown, so a stream a viewer is watching
// survives a bitrate edit instead of costing every viewer a reconnect.
//
// The command is rendered before anything is torn down, so a combination no engine can build refuses
// the restart and leaves the stream running what it has.
//
// procMu is held across the teardown and the launch, so nothing reads the window with no run in it,
// and the outgoing child's callbacks find the run that replaced them.
//
// The outgoing child is killed and not waited for.
// The relay drops the publisher it holds when a new one connects to the same path, so the successor
// need not arrive after the old socket is gone, and viewers reconnect across the gap either way.
//
// A publish waiting out its backoff restarts the same way, and the failing settings take their
// attempts with them: the pipeline just named is not the one those attempts were spent on.
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
	if running && a.applyLiveLocked(s) {
		return nil
	}
	if running {
		a.run.handle.Stop()
		a.run = nil
	}
	a.cancelRetryLocked()
	// The successor is another child on another port, so the preview goes with the pipeline it was
	// reading rather than being handed to one that will not send to it.
	a.stopPreviewLocked()

	logger.Infof("restarting the publish of '%s' on the settings the form holds", s.Publish.Name)
	return a.launchLocked(s, 0)
}

// launchLocked starts the encoder child on s and takes the run it produced.
// attempts is how many retries reaching this launch has cost, zero for one the user asked for.
//
// procMu is held by the caller, and the run is in place before the child can report anything: a
// callback that fires first blocks on that lock and finds the run it belongs to rather than a window
// with none.
func (a *App) launchLocked(s settings.Settings, attempts int) error {
	assert.Assert(a.run == nil || !a.run.handle.Running(), "a publish starts with no other one running", s.Publish.Name)
	assert.Assert(a.retry == nil, "a publish starts with no relaunch pending", s.Publish.Name)
	assert.Assert(attempts >= 0 && attempts <= len(publishBackoff), "a launch carries the retries it cost", attempts)

	pub, err := publish.For(s.Publish.Capture)
	if err != nil {
		return err
	}
	// At most one backend holds a source, and it is the one the publish in force captures with: a
	// stream that moved to another backend leaves the one it moved off holding a screen no child reads.
	publish.ReleaseSourcesExcept(s.Publish.Capture)

	// The preview comes up first, because the child is told the port it bound.
	// The publish owns its whole lifecycle: no effect on the contract opens one, and every path that
	// ends this child takes it down (preview.go).
	preview := a.startPreviewLocked(s)

	monitor, monitorKnown := display.At(s.Publish.Monitor)
	run := &publishRun{
		settings: s, startedAt: time.Now(), attempts: attempts,
		monitor: monitor, monitorKnown: monitorKnown,
	}
	a.run = run
	handle, err := pub.Start(s, "publish", preview, publish.Callbacks{
		OnStats: func(stats publish.Stats) {
			if !a.isCurrentRun(run) {
				return
			}
			a.takePublishDelay(run, stats)
			a.emit(wire.PublishStatsEvent(stats))
		},
		OnExit: func(err error, stderrTail string, logPath string) {
			a.publishEnded(run, err, stderrTail, logPath)
		},
		// The position belongs to the run that read it, so it is dropped once that run is not the one in
		// force and a pointer never outlives the capture it was over.
		OnPointer: func(p pointer.Position) {
			if !a.isCurrentRun(run) {
				return
			}
			a.pointerAt.take(p)
		},
	})
	if err != nil {
		a.run = nil
		// Nothing will send to the port just bound, and a preview left up is a pipeline waiting on a
		// child that never started.
		a.stopPreviewLocked()
		a.releaseSourcesLocked()
		return err
	}
	run.handle = handle

	logger.Infof("publishing '%s' via %s (%s, %s, %d fps)", s.Publish.Name, s.Publish.Transport, s.Publish.Mode, s.Publish.Chroma, s.Publish.Fps)
	return nil
}

// publishMonitorLocked is the output the running pipeline crops to, and false where no pipeline runs
// or the enumeration named none at that index.
// A publish waiting out its backoff has no pipeline, so it reports false: the rectangle belongs to a
// child, not to the settings it will be relaunched on.
// procMu is held by the caller.
func (a *App) publishMonitorLocked() (display.Monitor, bool) {
	if a.run == nil || !a.run.handle.Running() {
		return display.Monitor{}, false
	}
	return a.run.monitor, a.run.monitorKnown
}

func (a *App) isCurrentRun(run *publishRun) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()
	return a.run == run
}

// takePublishDelay records what one sample measured about the publish leg.
//
// Dropped for a run that is no longer the one in force, as a pointer position is: a reading belongs
// to the pipeline that took it, and a stale one would describe a stream that is no longer being
// sent.
// A sample measuring nothing writes nothing measured, so a run whose engine reports no delay leaves
// the budget's publishing stages absent rather than frozen on the last engine's figures.
func (a *App) takePublishDelay(run *publishRun, stats publish.Stats) {
	assert.IsNotNil(run, "a delay reading belongs to a run")

	a.procMu.Lock()
	defer a.procMu.Unlock()
	if a.run != run {
		return
	}
	run.delay = publishDelay{
		Transit: measuredMs(stats.TransitMs, stats.Missing.TransitMs),
		Link:    measuredMs(stats.LinkMs, stats.Missing.LinkMs),
	}
}

// measuredMs is one figure of a sample, and nil where the sample carries no measurement of it.
func measuredMs(value float64, missing bool) *float64 {
	if missing {
		return nil
	}
	return &value
}

// StopPublish ends the publish, running or waiting out a backoff, and succeeds where neither is in
// force.
// The retry budget gets no say: the user asked for no stream, and a relaunch seconds later is one
// nobody asked for.
func (a *App) StopPublish() {
	a.procMu.Lock()
	if a.run != nil || a.retry != nil {
		if a.run != nil {
			a.run.handle.Stop()
			a.run = nil
		}
		a.cancelRetryLocked()
		logger.Infof("publishing stopped")
		// The position belonged to the capture that just ended, so it goes with it: one held past the
		// stream is drawn over a picture that has stopped.
		a.pointerAt.clear()
	}
	// Outside the branch, so a preview with no publish behind it goes whether or not anything was
	// running to take it from.
	a.stopPreviewLocked()
	// The user asked for no stream, so the consent the compositor granted for one is given back.
	a.releaseSourcesLocked()
	a.procMu.Unlock()

	a.emitPublishState()
}

// GetPublishState reports whether a publish is in force, the settings its pipeline was built from,
// and whether the settings the app holds build a different one.
//
// A shell reads it on mount and every change announces it, so a fresh shell and a running one cannot
// be told different things.
// The two mutexes are taken in the order app.go fixes, and neither is held across the comparison,
// which renders both pipelines.
//
// A publish waiting out its backoff reports the settings it will come back on and answers as
// publishing, since it is the one the user asked for and the only one they can stop.
// Retrying is what separates a stream carrying frames from one between attempts.
func (a *App) GetPublishState() PublishState {
	a.settingsMu.Lock()
	held := a.settings
	a.settingsMu.Unlock()

	var state PublishState

	a.procMu.Lock()
	live, retry := a.livePublishLocked()
	// Read under the same lock as the publish it belongs to, so no state reports a preview beside a
	// stream that had already stopped when the preview was read.
	state.Preview = a.previewSnapshotLocked()
	if retry != nil {
		state.Retrying = true
		// The budget is set with the attempt because the two are one fact, "attempt 2 of 3", and a budget
		// beside a stream carrying frames would name attempts nothing is spending.
		// Both stay zero while nothing retries, which is what the contract asserts of the snapshot this
		// becomes (wire.PublishState).
		state.Attempt = retry.attempts
		state.Budget = len(publishBackoff)
		// Held from the exit that armed the relaunch, so every attempt says why rather than only the last.
		state.Cause = retry.cause
		state.Message = retry.message
	}
	// The one place both halves are in hand.
	// A launch brings the preview up and every path that ends the child takes it down, so a preview
	// standing beside nothing is a path that forgot the second half (preview.go).
	assert.Assert(state.Preview == nil || live != nil, "a local preview belongs to a publish")
	a.procMu.Unlock()

	if live == nil {
		return PublishState{}
	}
	state.Publishing = true
	state.Settings = live

	if same, err := publish.SamePipeline(*live, held); err != nil {
		// One of the two names a pipeline that cannot be built, so what the stream carries cannot be held
		// against what the form shows.
		// Pending says exactly that, and the reason is already on screen: the command preview renders
		// through the same call and shows this error.
		logger.Warnf("cannot tell whether the running publish carries the settings the form holds: %v", err)
		state.Pending = true
	} else {
		state.Pending = !same
	}

	// Stated where the pair is set, because it is what its consumers assume: the contract asserts it
	// of the snapshot this becomes (wire.PublishState), and a shell draws "attempt n of m" off it.
	// A state that broke it fails where it was built rather than at the far end of a conversion that
	// only copied it.
	assert.Assert(state.Retrying || (state.Attempt == 0 && state.Budget == 0),
		"an attempt and a budget belong to a retry", state.Attempt, state.Budget)
	assert.Assert(state.Retrying || (state.Cause == nil && state.Message == ""),
		"what ended a pipeline belongs to the relaunch that follows it", state.Message)
	return state
}

// emitPublishState announces what the publish state became, including the changes the shell reading
// it did not make: a stop from elsewhere cannot leave a form showing a running stream, and an edit
// cannot leave it claiming the live stream carries that edit.
//
// The state is read once and announced once, so no two readers are told what two reads of a state
// that moved between them would have said.
func (a *App) emitPublishState() {
	state := a.GetPublishState()
	a.emit(wire.PublishStateEvent(publishSnapshot(state)))
}
