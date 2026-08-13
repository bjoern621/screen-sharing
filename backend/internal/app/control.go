package app

import (
	"context"
	"errors"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The control contract's place in this process: the adapter the service reads and acts through, and
// the start and stop that fit it into the process lifecycle.
//
// The adapter exists because the contract's shapes and the app's are not one, and neither is wrong:
// the contract names a decode with a wire.WatchKey where the app takes a stream and a leg, and
// nests the publish state where the app holds it flat.
// Reconciling them here keeps the app free of the contract's shapes and the contract free of the
// app's, in one file rather than spread over the methods that would carry it.
//
// Nothing here decides anything.
// Every method reads through or forwards, because a decision made in an adapter is a decision the
// surface it adapts does not have.

// controlBackend serves the control contract off one app.
//
// It holds the app and nothing else.
// Every answer is read through on the call that asks for it, which is what keeps the picture a
// shell draws and the state this process holds from drifting apart.
//
// The adapter methods below state no preconditions of their own, which is the shape rather than an
// omission: each forwards, so the contract belongs to what it forwards to, and asserting the same
// thing twice would put a second answer beside it.
// The one condition they all rest on is the app they hold, set where the adapter is built.
type controlBackend struct{ app *App }

// Checked at the type rather than at the one call site, so a method that drifts out of shape fails
// at compile time instead of inside a lifecycle hook.
var _ control.Backend = controlBackend{}

// --- Reads ---

func (b controlBackend) Settings() settings.Settings      { return b.app.GetSettings() }
func (b controlBackend) StoreNotice() *screensharev1.Text { return b.app.StoreNotice() }
func (b controlBackend) Monitors() []display.Monitor      { return b.app.Monitors() }
func (b controlBackend) Platform() platform.Info          { return b.app.Platform() }

func (b controlBackend) Encoders(ctx context.Context) encoders.Availability {
	return b.app.probeEncoders(ctx)
}

func (b controlBackend) CachedEncoders() encoders.Availability { return b.app.cachedEncoders() }

func (b controlBackend) AudioDevices() []platform.AudioDevice { return b.app.audioDevices() }

func (b controlBackend) Pointer() (pointer.Position, bool) { return b.app.Pointer() }

func (b controlBackend) PublishState() wire.PublishSnapshot {
	return publishSnapshot(b.app.GetPublishState())
}

func (b controlBackend) RelayStatus() relay.Status { return b.app.lastRelayStatus() }

// Watching carries the open viewers into the contract's shape through the conversion the viewer
// state event already uses.
// A second conversion would be a second answer to "which viewers are open", and the read and the
// event would drift apart on it.
func (b controlBackend) Watching() []wire.WatchKey { return b.app.watchKeys() }

func (b controlBackend) TestStreamsRunning() int { return b.app.TestStreamsRunning() }

// MaxTestStreams is the bound StartTestStreams enforces, read rather than discovered by asking for
// too much.
// The contract refuses an over-large request above the call, with the code it names for a bounded
// resource, and a bound only an error could reveal would leave a saturated machine and a missing
// binary indistinguishable.
func (b controlBackend) MaxTestStreams() int { return maxTestStreams }

// --- Measurements ---

func (b controlBackend) MeasureUplink(ctx context.Context) (float64, error) {
	return b.app.measureUplink(ctx)
}

func (b controlBackend) MeasureEncodeRate(ctx context.Context, s settings.Settings) (encoderate.Rate, error) {
	return b.app.measureEncodeRate(ctx, s)
}

// --- Effects ---

func (b controlBackend) SaveSettings(s settings.Settings) error  { return b.app.SaveSettings(s) }
func (b controlBackend) StartPublish(s settings.Settings) error  { return b.app.StartPublish(s) }
func (b controlBackend) ApplyToStream(s settings.Settings) error { return b.app.Republish(s) }
func (b controlBackend) StopPublish()                            { b.app.StopPublish() }

func (b controlBackend) StartWatch(key wire.WatchKey) error {
	return b.app.StartWatch(key.StreamName, key.Transport)
}

func (b controlBackend) SubscribeFrames(key wire.WatchKey) (control.FrameStream, error) {
	return b.app.SubscribeFrames(key.StreamName, key.Transport)
}

// SubscribePreviewFrames rebuilds the interface value instead of handing the concrete subscription
// back: a nil pointer in a non-nil interface is not nil, so a refusal returned straight through
// would reach the service as a FrameStream it goes on to call.
func (b controlBackend) SubscribePreviewFrames() (control.FrameStream, error) {
	frames, err := b.app.SubscribePreviewFrames()
	if err != nil {
		return nil, err
	}
	return frames, nil
}

// SubscribeMonitorFrames rebuilds the interface value for the reason SubscribePreviewFrames does.
func (b controlBackend) SubscribeMonitorFrames(monitor int) (control.FrameStream, error) {
	frames, err := b.app.SubscribeMonitorFrames(monitor)
	if err != nil {
		return nil, err
	}
	return frames, nil
}

func (b controlBackend) StartMonitorPreview(monitor int) error {
	return b.app.StartMonitorPreview(monitor)
}

func (b controlBackend) StopMonitorPreview(monitor int) { b.app.StopMonitorPreview(monitor) }

func (b controlBackend) MonitorPreviewState() []wire.PreviewedMonitor {
	return b.app.MonitorPreviewState()
}

func (b controlBackend) StartReceive(key wire.WatchKey, toneMap bool) error {
	return b.app.StartReceive(key.StreamName, key.Transport, toneMap)
}

func (b controlBackend) StopReceive(key wire.WatchKey) {
	b.app.StopReceive(key.StreamName, key.Transport)
}

func (b controlBackend) SetReceiveAudio(key wire.WatchKey, volume float64, muted bool) error {
	return b.app.SetReceiveAudio(key.StreamName, key.Transport, volume, muted)
}

func (b controlBackend) ReceiveState() []wire.ReceiveStream { return b.app.ReceiveState() }
func (b controlBackend) AudioLevels() []wire.AudioLevel     { return b.app.AudioLevels() }

func (b controlBackend) StopWatch(key wire.WatchKey) {
	b.app.StopWatch(key.StreamName, key.Transport)
}

func (b controlBackend) OpenInBrowser(key wire.WatchKey) error {
	return b.app.OpenInBrowser(key.StreamName, key.Transport)
}

func (b controlBackend) StartTestStreams(count int) error { return b.app.StartTestStreams(count) }
func (b controlBackend) StopTestStreams()                 { b.app.StopTestStreams() }

func (b controlBackend) ForgetPortalConsent() error { return b.app.ForgetPortalConsent() }
func (b controlBackend) OpenLog(path string) error  { return b.app.OpenLog(path) }
func (b controlBackend) OpenLogsFolder() error      { return b.app.OpenLogsFolder() }

// publishSnapshot carries the publish state from this package's flat shape to the contract's nested
// one.
//
// It lives here rather than in wire because the shape it starts from is this package's: a
// conversion package that knew PublishState would make the contract depend on the surface it is
// meant to be independent of.
//
// Past this function the flat form's booleans can no longer state something the rules forbid.
// "A running stream has settings", "an attempt belongs to a retry" and "a retry belongs to a stream
// the user has not stopped" are all nil pointers on the far side rather than invariants somebody
// has to assert, and the producer states them as a postcondition where the flat state is built
// (GetPublishState), which is the one place that still can.
//
// A flat state that is publishing with no settings is that postcondition already broken, and it
// converts to an idle snapshot rather than to a live one with nothing in it: the contract holds a
// live stream to carrying what it was built from, and a shell drawing a stream configured entirely
// wrong is worse than one drawing none.
func publishSnapshot(state PublishState) wire.PublishSnapshot {
	if !state.Publishing || state.Settings == nil {
		return wire.PublishSnapshot{}
	}

	live := &wire.LiveSnapshot{Settings: *state.Settings, Pending: state.Pending, Preview: state.Preview}
	if state.Retrying {
		live.Retry = &wire.RetrySnapshot{Attempt: state.Attempt, Budget: state.Budget}
	}
	return wire.PublishSnapshot{Live: live}
}

// startControl serves the control contract on this platform's local socket.
//
// Idempotent, stated by the sync.Once, the way Service.Stop states it at the other end of the
// lifecycle.
// One process can hold the socket's name, so a second call while a service is opening or serving
// does nothing rather than failing to take a name this process already holds.
//
// Nothing about it is waited for: the listen and the serve both run on a goroutine of their own, so
// the caller reaches the rest of the boot at its own speed.
func (a *App) startControl() {
	a.controlOnce.Do(func() { go a.serveControl() })
}

// serveControl opens the endpoint and keeps the service it produced.
//
// An endpoint another backend holds ends this process, through fail.
// The endpoint is the whole discovery mechanism, so nothing would ever reach this backend: it would
// go on running the boot work, the relay poll and the synthetic set, against the same relay as the
// backend the shells are talking to, and it is this one's log a reader opens first.
// The shell survives the exit: it starts a backend only after a connect failed, and the endpoint it
// then waits for is served by the instance that was there first (Backend/BackendProcess.cs).
//
// Every other reason the endpoint will not open is an Umgebungsfehler that leaves this process
// running, on serve.go's reasoning: a backend keeps capturing and publishing with no shell
// attached.
func (a *App) serveControl() {
	assert.IsNotNil(a.events, "a served contract announces through a broker")
	assert.Assert(a.version != "", "a served contract names the build behind it")

	service, err := control.Start(control.New(controlBackend{app: a}, a.events, a.version))
	if errors.Is(err, control.ErrAddressInUse) {
		a.fail(err)
		return
	}
	if err != nil {
		logger.Warnf("control: not serving on %s: %v", control.Endpoint(), err)
		return
	}
	assert.IsNotNil(service, "an opened control socket yields a service that can be stopped")

	a.controlMu.Lock()
	stopped := a.controlStopped
	if !stopped {
		a.control = service
	}
	a.controlMu.Unlock()

	if stopped {
		// The shutdown ran while the socket was still opening and found nothing to stop, so what it
		// would have stopped is stopped here rather than left serving a process on its way out.
		service.Stop()
	}
}

// stopControl ends the control service, and a call with none running succeeds.
//
// Idempotent on every path into it.
// The handle is taken out before it is stopped, so a second call finds none.
// Service.Stop is a sync.Once of its own, so a handle stopped elsewhere is not stopped twice.
// The flag covers the shutdown that runs while the socket is still opening, where there is nothing
// to stop yet and something to stop a moment later (serveControl).
func (a *App) stopControl() {
	a.controlMu.Lock()
	service := a.control
	a.control = nil
	a.controlStopped = true
	a.controlMu.Unlock()

	if service != nil {
		service.Stop()
	}
}
