package app

import (
	"context"
	"errors"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/reach"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The control contract's place in this process: the adapter the service reads and acts through, and
// the start and stop that fit it into the process lifecycle.
//
// The adapter reconciles two shapes: the contract names a decode with a wire.StreamRef where the app
// takes a stream and a leg, and nests the publish state where the app holds it flat.
// Reconciling them in one file keeps the app free of the contract's shapes,
// and the contract free of the app's.
//
// Nothing here decides anything.
// Every method reads through or forwards: a decision made in an adapter is one the surface it adapts
// does not have.

// controlBackend serves the control contract off one app.
//
// Every answer is read through on the call that asks for it,
// so the picture a shell draws and the state this process holds cannot drift apart.
//
// The adapter methods below state no preconditions of their own:
// each forwards, so the contract belongs to what it forwards to,
// and asserting the same thing twice would put a second answer beside it.
// The one condition they all rest on is the app they hold, set where the adapter is built.
type controlBackend struct{ app *App }

// Checked at the type rather than at the one call site,
// so a method drifting out of shape fails at compile time instead of inside a lifecycle hook.
var _ control.Backend = controlBackend{}

// --- Reads ---

func (b controlBackend) Settings() settings.Settings      { return b.app.GetSettings() }
func (b controlBackend) StoreNotice() *screensharev1.Text { return b.app.StoreNotice() }
func (b controlBackend) Monitors() []display.Monitor      { return b.app.Monitors() }
func (b controlBackend) Platform() platform.Info          { return b.app.Platform() }
func (b controlBackend) Device() capabilities.Device      { return b.app.Device() }

func (b controlBackend) Encoders(ctx context.Context) encoders.Availability {
	return b.app.probeEncoders(ctx)
}

func (b controlBackend) CachedEncoders() encoders.Availability { return b.app.cachedEncoders() }

func (b controlBackend) AudioDevices() []platform.AudioDevice { return b.app.audioDevices() }

func (b controlBackend) PortalCapabilities() portal.Capabilities {
	return b.app.portalCapabilities()
}

func (b controlBackend) Pointer() (pointer.Spot, bool) { return b.app.Pointer() }

func (b controlBackend) StreamPointer(ref wire.StreamRef) (pointer.Spot, bool) {
	return b.app.StreamPointer(StreamRef{Name: ref.StreamName, Transport: ref.Transport})
}

func (b controlBackend) PublishState() wire.PublishSnapshot {
	return publishSnapshot(b.app.GetPublishState())
}

func (b controlBackend) RelayStatus() relay.Status { return b.app.lastRelayStatus() }

// Watching carries the open viewers into the contract's shape through the conversion the viewer
// state event uses.
// A second conversion would be a second answer to "which viewers are open",
// and the read and the event would drift apart on it.
func (b controlBackend) Watching() []wire.StreamRef { return b.app.watchRefs() }

func (b controlBackend) TestStreamState() (int, []wire.TestStreamSlot) {
	return b.app.TestStreamState()
}

// MembersState carries the presence loop's last reading, the one place a shell learns the group
// from: the read and the event are the same snapshot.
func (b controlBackend) MembersState() wire.MembersSnapshot { return b.app.MembersState() }

// Brokered carries Discord mode's membership into a draft, which no wire copy can (discord.go).
func (b controlBackend) Brokered(s settings.Settings) settings.Settings {
	return b.app.withBrokered(s)
}

// DiscordState reads what the last manager pass landed, stating nothing of its own (discord.go).
func (b controlBackend) DiscordState() wire.DiscordSnapshot {
	return b.app.discordState().wire()
}

// MaxTestStreams is the bound StartTestStreams enforces,
// read rather than discovered by asking for too much.
// A bound only an error could reveal would leave a saturated machine and a missing binary
// indistinguishable.
func (b controlBackend) MaxTestStreams() int { return maxTestStreams }

// --- Measurements ---

func (b controlBackend) MeasureUplink(ctx context.Context) (float64, error) {
	return b.app.measureUplink(ctx)
}

func (b controlBackend) MeasureEncodeRate(ctx context.Context, s settings.Settings) (encoderate.Rate, error) {
	return b.app.measureEncodeRate(ctx, s)
}

func (b controlBackend) CheckRelay(ctx context.Context, s settings.Settings) []reach.Result {
	return b.app.checkRelay(ctx, s)
}

// --- Effects ---

func (b controlBackend) SaveSettings(s settings.Settings) error  { return b.app.SaveSettings(s) }
func (b controlBackend) StartPublish(s settings.Settings) error  { return b.app.StartPublish(s) }
func (b controlBackend) ApplyToStream(s settings.Settings) error { return b.app.Republish(s) }
func (b controlBackend) StopPublish()                            { b.app.StopPublish() }

func (b controlBackend) StartWatch(ref wire.StreamRef) error {
	return b.app.StartWatch(ref.StreamName, ref.Transport)
}

// SubscribeFrames rebuilds the interface value for the reason SubscribePreviewFrames does, and this
// is the arm that refuses most often: nothing is decoding the pair a ref names until a tile opens.
func (b controlBackend) SubscribeFrames(ref wire.StreamRef) (control.FrameStream, error) {
	frames, err := b.app.SubscribeFrames(ref.StreamName, ref.Transport)
	if err != nil {
		return nil, err
	}
	return frames, nil
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

func (b controlBackend) StartReceive(ref wire.StreamRef, toneMap bool) error {
	return b.app.StartReceive(ref.StreamName, ref.Transport, toneMap)
}

func (b controlBackend) StopReceive(ref wire.StreamRef) {
	b.app.StopReceive(ref.StreamName, ref.Transport)
}

func (b controlBackend) SetReceiveAudio(ref wire.StreamRef, volume float64, muted bool) error {
	return b.app.SetReceiveAudio(ref.StreamName, ref.Transport, volume, muted)
}

func (b controlBackend) ReceiveState() []wire.ReceiveStream { return b.app.ReceiveState() }
func (b controlBackend) AudioLevels() []wire.AudioLevel     { return b.app.AudioLevels() }

func (b controlBackend) StopWatch(ref wire.StreamRef) {
	b.app.StopWatch(ref.StreamName, ref.Transport)
}

func (b controlBackend) OpenInBrowser(ref wire.StreamRef) error {
	return b.app.OpenInBrowser(ref.StreamName, ref.Transport)
}

func (b controlBackend) StartTestStreams(count int) error { return b.app.StartTestStreams(count) }
func (b controlBackend) StopTestStreams()                 { b.app.StopTestStreams() }

func (b controlBackend) ForgetPortalConsent() error { return b.app.ForgetPortalConsent() }

func (b controlBackend) CreateGroup(relay settings.Relay) (groupKey, groupID string, err error) {
	return b.app.CreateGroup(relay)
}

// LinkDiscord holds the call for the person's browser leg (discordlink.go).
func (b controlBackend) LinkDiscord(ctx context.Context, relay settings.Relay) error {
	return b.app.LinkDiscord(ctx, relay)
}

func (b controlBackend) OpenLog(path string) error { return b.app.OpenLog(path) }
func (b controlBackend) OpenLogsFolder() error     { return b.app.OpenLogsFolder() }

// publishSnapshot carries the publish state from this package's flat shape to the contract's nested
// one.
//
// Here rather than in wire: a conversion package that knew PublishState would make the contract
// depend on the surface it is meant to be independent of.
//
// Past this function the flat form's booleans cannot state what the rules forbid.
// "A running stream has settings", "an attempt belongs to a retry" and "a retry belongs to a stream
// the user has not stopped" are nil pointers on the far side rather than invariants somebody asserts,
// and GetPublishState states them as a postcondition where the flat state is built.
//
// A flat state that is publishing with no settings is that postcondition broken,
// and it converts to an idle snapshot rather than to a live one with nothing in it:
// the contract holds a live stream to carrying what it was built from,
// and a shell drawing a stream configured entirely wrong is worse than one drawing none.
func publishSnapshot(state PublishState) wire.PublishSnapshot {
	if !state.Publishing || state.Settings == nil {
		return wire.PublishSnapshot{}
	}

	live := &wire.LiveSnapshot{Settings: *state.Settings, Pending: state.Pending, Preview: state.Preview}
	// Read off the settings the pipeline was built from, so the figure crossing is the one the encoder
	// holds rather than one a later edit put in the form.
	if ceiling, bounded := publish.RateCeilingMbps(*state.Settings); bounded {
		live.RateCeilingMbps = &ceiling
	}
	if state.Retrying {
		live.Retry = &wire.RetrySnapshot{
			Attempt: state.Attempt,
			Budget:  state.Budget,
			Cause:   state.Cause,
			Message: state.Message,
		}
	}
	return wire.PublishSnapshot{Live: live}
}

// startControl serves the control contract on this platform's local socket.
//
// Idempotent through the sync.Once, the way Service.Stop is at the other end of the lifecycle.
// One process can hold the socket's name, so a second call while a service is opening or serving
// does nothing rather than failing to take a name this process already holds.
//
// Nothing is waited for: listen and serve both run on a goroutine of their own, so the caller reaches
// the rest of the boot at its own speed.
func (a *App) startControl() {
	a.controlOnce.Do(func() { go a.serveControl() })
}

// serveControl opens the endpoint and keeps the service it produced.
//
// An endpoint another backend holds ends this process, through fail.
// The endpoint is the whole discovery mechanism, so nothing would reach this backend:
// it would go on running the boot work, the relay poll and the synthetic set against the same relay
// as the backend the shells are talking to, and it is this one's log a reader opens first.
// The shell survives the exit: it starts a backend only after a connect failed,
// and waits on the endpoint the instance that was there first serves (Backend/BackendProcess.cs).
//
// Every other reason the endpoint will not open is an Umgebungsfehler that leaves this process
// running, on serve.go's reasoning: a backend keeps capturing and publishing with no shell attached.
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
		// The shutdown ran while the socket was still opening and found nothing to stop,
		// so it is stopped here rather than left serving a process on its way out.
		service.Stop()
	}
}

// stopControl ends the control service, and a call with none running succeeds.
//
// Idempotent on every path into it.
// The handle is taken out before it is stopped, so a second call finds none.
// Service.Stop is a sync.Once of its own, so a handle stopped elsewhere is not stopped twice.
// The flag covers the shutdown that runs while the socket is still opening, with nothing to stop yet
// and something to stop a moment later (serveControl).
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
