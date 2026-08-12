package app

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/screensrc"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Monitor previews: this machine reading its own screens so one is chosen by looking at
// it rather than by its number.
//
// They are the third kind of picture the frame channel carries, and the shortest. A relay
// decode pulls a stream off the network and decodes it; the publish preview decodes a
// copy of what is being sent; a monitor preview decodes nothing at all - the capture
// element hands raw pictures to the render chain, and what leaves is the same handle in
// every case (docs/viewer-architecture.md, "The frame channel").
//
// They are opened by an effect and not by a subscription, which is the rule the whole
// contract turns on: a channel that started a screen capture would be a channel deciding
// that a window exists. So they outlive the window that asked for one, exactly as decodes
// do, and the set is announced whole so the next shell converges on it rather than
// guessing (docs/ipc-api.md).
//
// A preview is keyed by the monitor index and by nothing else. It is what
// PublishSettings.monitor holds and what the catalog enumerates outputs under, so a
// second field here would be a key with a spare part.

// previewLeg for a screen is a description and not a registry name, for the reason the
// publish preview's is one: no transport carries these frames, because nothing carried
// them anywhere. It reaches the receive package's log lines and nothing else.
const monitorPreviewLeg = "screen capture"

// StartMonitorPreview reads one of this machine's monitors into a picture the frame
// channel can hand over.
//
// Idempotent, and the guard is a read of what is running rather than a flag: a monitor
// already being previewed is the state this asks for, so a second call is a success that
// changes nothing. It is read before anything is validated for the reason StartReceive
// reads first - a repeat must not be able to fail on a precondition that moved under a
// preview which is already up.
//
// Two refusals, and they are different kinds. A monitor the enumeration does not carry is
// a request naming something that cannot exist. A session with no element that reads one
// output apart from another is a machine that cannot do this at all, which the catalog
// already says (Catalog.no_monitor_preview) so that a shell reads it instead of asking.
func (a *App) StartMonitorPreview(monitor int) error {
	// Announced after the lock is released, the way the receive effects announce theirs.
	// It runs on the refusals too, where it announces the set unchanged: every event
	// carries a whole state, so a duplicate costs nothing and the alternative is an
	// effect that sometimes announces nothing.
	defer a.emitMonitorPreviewState()

	if a.previewingMonitor(monitor) {
		logger.Debugf("monitor %d is already being previewed", monitor)
		return nil
	}

	if _, enumerated := display.At(monitor); !enumerated {
		return fmt.Errorf("monitor %d is not one of this machine's outputs", monitor)
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
		// The same question again, under the lock: the read above is what keeps a repeat
		// cheap, and this is what keeps two starts racing for one screen from opening it
		// twice.
		return nil
	}

	receiver, err := receive.New(receive.Stream{
		// The name is what this package's log lines report the pipeline under. A screen
		// has no name on any relay, so it is named after the output it reads.
		Name:      monitorName(monitor),
		Transport: monitorPreviewLeg,
		Source:    source,
		// Nothing encoded these frames, so the pipeline grows no decoder and no audio
		// branch: a screen has no second track and nothing to autoplug for.
		Raw: true,
	}, receive.Open{Chain: chain}, receive.Events{
		// A first frame is what turns a preview from opening into live, which the state
		// reports, so it is announced like every other change.
		OnLive: a.emitMonitorPreviewState,
		OnEnd: func(message string) {
			a.monitorPreviewEnded(monitor, message)
		},
	})
	if err != nil {
		return err
	}

	a.monitorPreviews[monitor] = receiver
	logger.Infof("previewing monitor %d", monitor)
	return nil
}

// StopMonitorPreview closes one monitor's preview. A monitor nothing is previewing is not
// a failure, for the reason StopReceive takes a decode that is already closed: a stop
// names the state the caller wants and that state already holds.
func (a *App) StopMonitorPreview(monitor int) {
	defer a.emitMonitorPreviewState()

	a.procMu.Lock()
	receiver, present := a.monitorPreviews[monitor]
	delete(a.monitorPreviews, monitor)
	a.procMu.Unlock()

	if !present {
		return
	}
	// Outside the lock: a teardown blocks on the pipeline reaching NULL, and every other
	// method that touches the map would wait behind it.
	receiver.Stop()
	logger.Infof("stopped previewing monitor %d", monitor)
}

// MonitorPreviewState is every monitor being previewed, read off the running pipelines
// rather than remembered.
//
// Nothing here is cached, which is the rule ReceiveState follows for the same reason: a
// pipeline that has produced no frame yet is still opening the screen, and a state
// assembled from what a caller believed it started would report it as live.
func (a *App) MonitorPreviewState() []wire.PreviewedMonitor {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	out := make([]wire.PreviewedMonitor, 0, len(a.monitorPreviews))
	for monitor, receiver := range a.monitorPreviews {
		out = append(out, wire.PreviewedMonitor{
			Monitor: monitor,
			Live:    receiver.Stats().Frames > 0,
		})
	}
	return out
}

// SubscribeMonitorFrames opens one consumer's view of a monitor preview's frames.
//
// It opens no capture, for the reason SubscribeFrames opens no decode: what brings a
// preview up is StartMonitorPreview, and a subscription that started one would be the
// frame channel deciding that a screen should be read. A monitor nothing is previewing is
// therefore a refusal rather than a wait, and a shell reads the preview state to know
// whether to ask at all.
func (a *App) SubscribeMonitorFrames(monitor int) (*receive.Subscription, error) {
	a.procMu.Lock()
	receiver, present := a.monitorPreviews[monitor]
	a.procMu.Unlock()

	if !present {
		return nil, fmt.Errorf("nothing is previewing monitor %d", monitor)
	}
	return receiver.Subscribe(), nil
}

// previewingMonitor reports whether a preview is open for this output.
//
// It reads the map rather than anything a caller believed it had started, which is what
// lets it stand in front of the validation StartMonitorPreview would otherwise run over a
// state that already holds.
func (a *App) previewingMonitor(monitor int) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	_, present := a.monitorPreviews[monitor]
	return present
}

// monitorPreviewEnded drops a preview pipeline that ended on its own and says why.
//
// Nothing is torn down here and nothing is retried: the pipeline has already stopped
// itself, and a screen that stopped being readable is a picture that goes away. The state
// says so by not carrying it, which is the whole of the news - there is no log to open
// and no viewer left waiting on it.
func (a *App) monitorPreviewEnded(monitor int, message string) {
	a.procMu.Lock()
	_, present := a.monitorPreviews[monitor]
	delete(a.monitorPreviews, monitor)
	a.procMu.Unlock()

	if !present {
		// A stop the user asked for takes the receiver out first, and the teardown then
		// ends the bus watch. There is nothing left to announce that the shell which
		// closed it does not already know.
		return
	}
	logger.Warnf("the preview of monitor %d ended: %s", monitor, message)
	a.emitMonitorPreviewState()
}

// emitMonitorPreviewState announces the running previews.
//
// It reads them back through MonitorPreviewState rather than being handed a set, so what
// is announced is what a read would answer with. It takes procMu, so a caller holding
// that lock defers this rather than calling it in place.
func (a *App) emitMonitorPreviewState() {
	a.emit(wire.MonitorPreviewStateEvent(a.MonitorPreviewState()))
}

// monitorName is what a preview pipeline is logged under. A screen is not a stream and
// has no name on any relay, so the index it is enumerated under is the whole of it.
func monitorName(monitor int) string {
	return fmt.Sprintf("monitor %d", monitor)
}
