package app

import (
	"context"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/moq"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The control contract's place in this process: the adapter that lets the service
// read and act through this app, and the start and stop that fit it into the window's
// lifecycle.
//
// The adapter exists because the two surfaces name the same things differently and
// neither name is wrong. An App method is what the Wails binding generator can carry -
// no context.Context parameter, and the event payloads as return values - while a
// control.Backend method is what a service holding a request context and the
// contract's own state shapes needs. Reconciling the two here keeps the binding free
// of parameters it has no model for and the contract free of the binding's shapes,
// and it keeps the reconciliation in one file instead of spread over the methods that
// suffer it.
//
// Nothing here decides anything. Every method reads through or forwards, because a
// decision made in an adapter is a decision the surface it adapts does not have.

// controlBackend serves the control contract off one app.
//
// It holds the app and nothing else. Every answer is read through on the call that
// asks for it, which is the rule that keeps the picture a shell draws and the picture
// this window draws from drifting apart.
type controlBackend struct{ app *App }

// controlBackend implements the whole contract surface, checked here rather than at
// the one call site so that a method that drifts out of shape fails at the type and
// not inside a lifecycle hook.
var _ control.Backend = controlBackend{}

// --- Reads ---

func (b controlBackend) Settings() settings.Stream        { return b.app.GetSettings() }
func (b controlBackend) StoreNotice() *screensharev1.Text { return b.app.StoreNotice() }
func (b controlBackend) Monitors() []display.Monitor      { return b.app.Monitors() }
func (b controlBackend) Platform() platform.Info          { return b.app.Platform() }

func (b controlBackend) Encoders(ctx context.Context) encoders.Availability {
	return b.app.probeEncoders(ctx)
}

func (b controlBackend) CachedEncoders() encoders.Availability { return b.app.cachedEncoders() }

func (b controlBackend) PublishState() wire.PublishSnapshot {
	return publishSnapshot(b.app.GetPublishState())
}

func (b controlBackend) RelayStatus() relay.Status { return b.app.lastRelayStatus() }

// Watching carries the open viewers into the contract's shape, through the same
// conversion the viewer state event goes through. Two conversions would be two answers
// to "which viewers are open", and the read and the event would eventually give
// different ones.
func (b controlBackend) Watching() []wire.WatchKey { return b.app.watchKeys() }

func (b controlBackend) TestStreamsRunning() int { return b.app.TestStreamsRunning() }

// MaxTestStreams is the bound StartTestStreams enforces, read rather than discovered
// by asking for too much: the contract refuses an over-large request above the call
// with the code it names for a bounded resource, and a bound only an error could
// reveal would make a saturated machine and a missing binary indistinguishable.
func (b controlBackend) MaxTestStreams() int { return maxTestStreams }

func (b controlBackend) MoqCert(ctx context.Context, streamName string) (moq.Cert, string, error) {
	return b.app.moqCert(ctx, streamName)
}

// --- Measurements ---

func (b controlBackend) MeasureUplink(ctx context.Context) (float64, error) {
	return b.app.measureUplink(ctx)
}

func (b controlBackend) MeasureEncodeRate(ctx context.Context, s settings.Stream) (encoderate.Rate, error) {
	return b.app.measureEncodeRate(ctx, s)
}

// --- Effects ---

func (b controlBackend) SaveSettings(s settings.Stream) error  { return b.app.SaveSettings(s) }
func (b controlBackend) StartPublish(s settings.Stream) error  { return b.app.StartPublish(s) }
func (b controlBackend) ApplyToStream(s settings.Stream) error { return b.app.Republish(s) }
func (b controlBackend) StopPublish()                          { b.app.StopPublish() }

func (b controlBackend) StartWatch(key wire.WatchKey) error {
	return b.app.StartWatch(key.StreamName, key.Transport)
}

func (b controlBackend) StopWatch(key wire.WatchKey) {
	b.app.StopWatch(key.StreamName, key.Transport)
}

func (b controlBackend) StartTestStreams(count int) error { return b.app.StartTestStreams(count) }
func (b controlBackend) StopTestStreams()                 { b.app.StopTestStreams() }

func (b controlBackend) ForgetPortalConsent() error { return b.app.ForgetPortalConsent() }
func (b controlBackend) OpenLog(path string) error  { return b.app.OpenLog(path) }
func (b controlBackend) OpenLogsFolder() error      { return b.app.OpenLogsFolder() }

// publishSnapshot carries the publish state from the binding's flat shape to the
// contract's nested one.
//
// It lives here rather than in wire because the shape it starts from is the
// binding's: PublishStateEvent exists so the frontend can read the state as JSON, and
// a conversion package that knew about it would make the contract depend on the
// surface it is meant to be independent of.
//
// This is the one function in the process that can build a publish state the rules
// forbid, and it is where the flat form's four booleans stop being able to. Once past
// it, "a running stream has settings", "an attempt belongs to a retry" and "a retry
// belongs to a stream the user has not stopped" are all nil pointers rather than
// invariants somebody has to assert. The producer states them as a postcondition where
// the flat state is built (GetPublishState), which is the one place that still can.
//
// A flat state that is publishing with no settings would be that postcondition already
// broken, and it converts to an idle snapshot rather than to a live one with nothing in
// it: the contract holds a live stream to carrying what it was built from, and a shell
// drawing a stream configured entirely wrong is worse than one drawing none.
func publishSnapshot(state PublishStateEvent) wire.PublishSnapshot {
	if !state.Publishing || state.Settings == nil {
		return wire.PublishSnapshot{}
	}

	live := &wire.LiveSnapshot{Settings: *state.Settings, Pending: state.Pending}
	if state.Retrying {
		live.Retry = &wire.RetrySnapshot{Attempt: state.Attempt, Budget: state.Budget}
	}
	return wire.PublishSnapshot{Live: live}
}

// startControl serves the control contract on this platform's local socket.
//
// It is idempotent, and sync.Once is what states it - the same way Service.Stop does
// at the other end of the lifecycle. Only one process can hold the socket's name, so
// a second call while one service is opening or serving is a no-op rather than a
// second listener failing to take a name this process already has.
//
// Nothing about it is waited for. Opening the socket is work the caller is on its way
// to a window through, and a shell connecting has nothing to do with that window
// appearing, so the whole of it runs on a goroutine of its own. Serve is already
// asynchronous inside; what this adds is that the listen in front of it is too.
func (a *App) startControl() {
	a.controlOnce.Do(func() { go a.serveControl() })
}

// serveControl opens the socket and takes the service it produced.
//
// A socket that will not open is not fatal, and the reasoning is serve.go's: this
// backend keeps capturing and publishing with no shell attached, which is what the
// contract says a backend without a shell does. The usual cause is another instance
// of this app already holding the name, and quitting over that would close the window
// the user just opened in favour of one they cannot see.
func (a *App) serveControl() {
	service, err := control.Start(control.New(controlBackend{app: a}, a.events, a.version))
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
		// The shutdown ran while the socket was still opening, so it found nothing to
		// stop. What it would have stopped is stopped here instead, rather than left
		// serving a process on its way out.
		service.Stop()
	}
}

// stopControl ends the control service, and is safe to call with none running.
//
// It is idempotent three times over, because there are three ways it can be called
// twice. The handle is taken before it is stopped, so a second call finds none;
// Service.Stop is itself a sync.Once, so a handle stopped elsewhere is not stopped
// twice; and the flag covers the shutdown that runs while the socket is still
// opening, which is the one case where there is nothing to stop yet and something to
// stop a moment later.
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
