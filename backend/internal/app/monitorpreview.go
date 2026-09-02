package app

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/decode"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/screensrc"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Monitor previews: this machine reading its own screens,
// so a monitor is chosen by looking at it rather than by its number.
//
// The third kind of picture the frame channel carries, and the only one that decodes nothing:
// the capture element hands raw pictures straight to the render chain, and the same handle leaves
// (docs/viewer-architecture.md, "The frame channel").
//
// An effect opens one and a subscription never does:
// a channel that started a capture would be a channel deciding a window exists.
// A preview therefore outlives the window that asked for it, as a decode does,
// and the whole set is announced so the next shell converges on it (docs/ipc-api.md).
//
// The monitor index is the whole key:
// what PublishSettings.monitor holds and what the catalog enumerates outputs under.

// monitorPreviewLeg is what the log calls this carriage.
// No transport carries these frames, and the string reaches the receive package's log lines only.
const monitorPreviewLeg = "screen capture"

// StartMonitorPreview reads one of this machine's monitors into a picture the frame channel hands over.
//
// Idempotent, guarded by a read of what is running rather than by a flag:
// a monitor already being previewed is the state asked for,
// so a second call succeeds and starts nothing.
// The read comes before the validation, as StartReceive's does,
// so a repeat cannot fail on a precondition that moved under a preview that is up.
//
// Two refusals, both Umgebungsfehler.
// A monitor the enumeration does not carry names something that cannot exist.
// A session with no element that reads one output apart from another cannot do this at all,
// which the catalog states (Catalog.no_monitor_preview) so a shell reads it instead of asking.
func (a *App) StartMonitorPreview(monitor int) error {
	// Announced with the lock released, as the receive effects announce theirs.
	// It runs on the refusals too, announcing the set unchanged:
	// an event carries a whole state, so a duplicate costs nothing,
	// and the alternative is an effect that sometimes announces nothing.
	defer a.emitMonitorPreviewState()

	if a.previewingMonitor(monitor) {
		logger.Debugf("monitor %d is already being previewed", monitor)
		return nil
	}

	if _, enumerated := display.At(monitor); !enumerated {
		// Typed: an index naming no output of this machine is what the contract calls INVALID_ARGUMENT,
		// against a session that cannot read one screen apart from another (control/refusal.go).
		return control.Refuse("monitor %d is not one of this machine's outputs", monitor)
	}

	source, err := screensrc.PreviewSource(platform.Detect(), monitor)
	if err != nil {
		return err
	}

	a.settingsMu.Lock()
	chain := a.settings.Viewer.RenderChain
	a.settingsMu.Unlock()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if _, present := a.monitorPreviews[monitor]; present {
		// The same question under the lock: the read above keeps a repeat cheap,
		// this keeps two starts racing for one screen from opening it twice.
		return nil
	}

	receiver, err := a.decodes.Open(decode.MonitorID(monitor), receive.Stream{
		// What the receive package logs the pipeline under.
		// A screen has no name on any relay, so the output it reads names it.
		Name:      monitorName(monitor),
		Transport: monitorPreviewLeg,
		Source:    source,
		// Nothing encoded these frames: no decoder to autoplug, and a screen carries no second track.
		Raw: true,
	}, receive.Open{Chain: chain}, decode.Events{
		// The first frame turns a preview from opening into live, which the state reports.
		OnLive: a.emitMonitorPreviewState,
		OnEnd: func(message string) {
			a.monitorPreviewEnded(monitor, message)
		},
	})
	if err != nil {
		return err
	}
	assert.IsNotNil(receiver, "an opened preview yields a pipeline that can be stopped", monitor)
	assert.IsNotNil(a.monitorPreviews, "the running previews are held in a map the app built")

	a.monitorPreviews[monitor] = receiver
	logger.Infof("previewing monitor %d", monitor)
	return nil
}

// StopMonitorPreview closes one monitor's preview, and succeeds where nothing is previewing it:
// the stop names a state that holds, as StopReceive does on a closed decode.
func (a *App) StopMonitorPreview(monitor int) {
	defer a.emitMonitorPreviewState()

	a.procMu.Lock()
	receiver, present := a.monitorPreviews[monitor]
	delete(a.monitorPreviews, monitor)
	a.procMu.Unlock()

	if !present {
		return
	}
	// Outside the lock: a teardown blocks until the pipeline reaches NULL,
	// and every other method touching the map would wait behind it.
	receiver.Stop()
	logger.Infof("stopped previewing monitor %d", monitor)
}

// MonitorPreviewState is every monitor being previewed, read off the running pipelines.
//
// Nothing is cached, the rule ReceiveState follows:
// a pipeline that has produced no frame is still opening the screen,
// and a state assembled from what a caller believed it started would report it live.
func (a *App) MonitorPreviewState() []wire.PreviewedMonitor {
	// Read off the host, the one owner of which pipelines are running.
	// A preview that ended is left out: it carries its reason until monitorPreviewEnded collects it.
	states := a.decodes.Snapshot()

	out := make([]wire.PreviewedMonitor, 0, len(states))
	for id, state := range states {
		if id.Kind != decode.KindMonitor || state.Ended {
			continue
		}
		out = append(out, wire.PreviewedMonitor{
			Monitor: id.Monitor,
			Live:    state.Stats.Frames > 0,
		})
	}
	return out
}

// SubscribeMonitorFrames opens one consumer's view of a monitor preview's frames.
//
// It opens no capture, as SubscribeFrames opens no decode:
// StartMonitorPreview brings a preview up,
// and a subscription that started one would be the frame channel deciding a screen be read.
// A monitor nothing is previewing is a refusal rather than a wait,
// and the preview state is what a shell reads to know whether to ask.
func (a *App) SubscribeMonitorFrames(monitor int) (decode.Subscription, error) {
	a.procMu.Lock()
	receiver, present := a.monitorPreviews[monitor]
	a.procMu.Unlock()

	if !present {
		return nil, fmt.Errorf("nothing is previewing monitor %d", monitor)
	}
	return receiver.Subscribe()
}

// previewingMonitor reads the running map, not anything a caller believed it started,
// so it can stand in front of the validation StartMonitorPreview would otherwise run over a state
// that holds.
func (a *App) previewingMonitor(monitor int) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	_, present := a.monitorPreviews[monitor]
	return present
}

// monitorPreviewEnded drops a preview pipeline that ended on its own and says why.
//
// Nothing is torn down and nothing is retried: the pipeline stopped itself,
// and a screen that stopped being readable is a picture that goes away.
// The state drops it, which is the whole of the news.
func (a *App) monitorPreviewEnded(monitor int, message string) {
	a.procMu.Lock()
	_, present := a.monitorPreviews[monitor]
	delete(a.monitorPreviews, monitor)
	a.procMu.Unlock()

	// The host keeps a pipeline that ended, carrying the reason, until it is stopped.
	// This is what collects it, so the set holds what is running.
	a.decodes.Stop(decode.MonitorID(monitor))

	if !present {
		// A stop the user asked for took the receiver out and announced the set,
		// so the bus watch ending behind it has nothing left to report.
		return
	}
	logger.Warnf("the preview of monitor %d ended: %s", monitor, message)
	a.emitMonitorPreviewState()
}

// emitMonitorPreviewState announces the running previews,
// read back through MonitorPreviewState rather than handed in,
// so the announcement is what a read would answer.
// It takes procMu, so a caller holding that lock defers it rather than calling it in place.
func (a *App) emitMonitorPreviewState() {
	a.emit(wire.MonitorPreviewStateEvent(a.MonitorPreviewState()))
}

// monitorName is what a preview pipeline is logged under.
// A screen has no name on any relay, so the index it is enumerated under is the whole of it.
func monitorName(monitor int) string {
	return fmt.Sprintf("monitor %d", monitor)
}
