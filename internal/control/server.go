// Package control serves the control contract: one ControlService implementation,
// over a local socket, in front of the backend that owns the product.
//
// The rule the contract encodes is docs/ipc-api.md: a shell shows what the backend
// describes and asks the backend to act, and decides nothing. This package is the
// side of that boundary which describes and acts. It holds no domain rules of its
// own - the tables live in capabilities, transport, gpupath and publish, the
// presentation of them in form, and the shaping of both onto the contract in wire -
// so a method here is a dispatch and not a decision.
//
// The methods divide into three kinds, and the division is the rule in executable
// form: reads compute and change nothing, effects do the one thing the user asked
// for, and Subscribe carries what changed. A method that is neither does not belong
// here.
package control

import (
	"context"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Backend is everything the service reaches to answer a read or to run an effect.
//
// It is an interface rather than a concrete app so the contract can be served in
// front of a test double with no window, no encoder and no relay, which is the
// property the boundary rule was chosen for: everything a shell displays was
// computed somewhere a test can reach.
//
// Every method here is either a read of state the implementation owns or one named
// effect. None of them formats, greys or labels anything: that is form's work, and a
// backend that returned a sentence for the screen would be a second author of the
// vocabulary.
type Backend interface {
	// --- Reads ---

	// Settings are the settings the backend holds, which are the starting draft for a
	// form.
	Settings() settings.Settings
	// StoreNotice states why the persisted settings could not be restored, nil when
	// they were.
	StoreNotice() *screensharev1.Text
	// Monitors lists this machine's display outputs.
	Monitors() []display.Monitor
	// Platform reports the OS and, on Linux, the display server.
	Platform() platform.Info
	// Encoders reports what this machine can really encode, per engine. The probe runs
	// once and is cached for the process lifetime, so the first call costs seconds and
	// every one after it costs nothing.
	Encoders(ctx context.Context) encoders.Availability
	// CachedEncoders is the probe result if one has been taken, and the zero value if
	// none has. It exists because a form is resolved on every keystroke and a probe
	// takes seconds: a resolve reads what is known now, and an unprobed engine is an
	// engine nothing is greyed on rather than an engine with nothing usable.
	CachedEncoders() encoders.Availability
	// PublishState is the running publish state.
	PublishState() wire.PublishSnapshot
	// RelayStatus is the latest relay snapshot. The backend owns the polling, so this
	// reads the last one rather than fetching a new one.
	RelayStatus() relay.Status
	// Watching lists the external viewers currently open.
	Watching() []wire.WatchKey
	// ReceiveState is every stream the backend is decoding, read off the running
	// pipelines rather than remembered: the chain that ran is not always the one that
	// was asked for.
	ReceiveState() []wire.ReceiveStream
	// TestStreamsRunning counts the synthetic publishers alive, which is not the count
	// that was asked for: one that died on its own drops out of it.
	TestStreamsRunning() int
	// MaxTestStreams is how many synthetic publishers the backend will run at once.
	//
	// It is readable rather than only enforced so an over-large request is refused
	// where every other request-earned refusal in this service is made: above the call,
	// with the code the contract names for a bounded resource. A bound that could only
	// be discovered by asking for too much would arrive as an untyped error, and a
	// missing binary would then be indistinguishable from a saturated machine.
	MaxTestStreams() int
	// --- Effects ---
	//
	// The measurements are among them. Each runs the real thing, takes seconds, and
	// leaves behind a result a later read sees, which is what an effect is: the probe
	// replaces what CachedEncoders answers with, and neither of the other two is
	// anything a shell may call on a keystroke.

	// MeasureUplink probes this machine's real upload throughput in Mbit/s.
	MeasureUplink(ctx context.Context) (float64, error)
	// MeasureEncodeRate times the configured encoder on generated frames.
	MeasureEncodeRate(ctx context.Context, s settings.Settings) (encoderate.Rate, error)

	// SaveSettings persists the settings the shell holds. It does not touch a running
	// stream: what reaches a live pipeline is ApplyToStream's business.
	SaveSettings(s settings.Settings) error
	// StartPublish persists the settings and starts the encoder on them.
	StartPublish(s settings.Settings) error
	// ApplyToStream restarts the running stream on new settings.
	ApplyToStream(s settings.Settings) error
	// StopPublish ends the stream, whether it is running or waiting out a backoff.
	StopPublish()
	// StartWatch opens an external viewer for one stream over one transport.
	StartWatch(key wire.WatchKey) error
	// StopWatch closes one open viewer.
	StopWatch(key wire.WatchKey)
	// StartReceive opens a decode for one stream on one leg, inside the backend, and
	// StopReceive closes one. They are the tile path's counterpart of the two above:
	// what they open is a decode and never a tile.
	StartReceive(key wire.WatchKey) error
	StopReceive(key wire.WatchKey)
	// SubscribeFrames opens one consumer's view of a decode that is already running,
	// and refuses where nothing is decoding the pair.
	//
	// It is on this interface and not on a second one because the frame service serves
	// the same backend the control service does: the decode a subscription draws from
	// is the one StartReceive opened, and two interfaces onto it would be two ideas of
	// which decodes exist.
	SubscribeFrames(key wire.WatchKey) (FrameStream, error)
	// StartTestStreams launches synthetic publishers, replacing a running set.
	StartTestStreams(count int) error
	// StopTestStreams stops every synthetic publisher.
	StopTestStreams()
	// ForgetPortalConsent drops the stored screen-capture consent.
	ForgetPortalConsent() error
	// OpenLog opens one run log in the machine's default application, and
	// OpenLogsFolder the directory holding them.
	OpenLog(path string) error
	OpenLogsFolder() error
}

// FrameStream is one consumer's running subscription to a decode's frames.
//
// It is an interface for the reason Backend is one: the frame service has to be
// servable in front of something with no GPU and no pipeline, and a concrete
// subscription would make every test of this service a test of the exporter behind it.
//
// The methods are the protocol. Events carries what the backend says and closes when
// the subscription ends, Err then says why, Release hands one slot back, SetRenderSize
// says how many pixels the consumer will draw at, and Close ends it from this side and
// frees the memory.
type FrameStream interface {
	Events() <-chan receive.Event
	Err() error
	Release(generation uint64, slot int, serial uint64)
	SetRenderSize(width, height int)
	Close()
}

// The contract version this build implements.
//
// The major settles on Hello before any other method is reached, so a mismatch is a
// sentence naming both versions rather than a field that silently arrives empty. The
// minor is informational: a shell built against a lower minor works, and one built
// against a higher minor may find a method missing.
const (
	ProtocolMajor = 1
	ProtocolMinor = 0
)

// Server is the ControlService implementation.
//
// It holds no state of its own beyond its collaborators. Every answer it gives is
// read through to the backend, the tables or the broker on the call that asks for
// it, which is the rule that keeps two copies of one fact from drifting apart.
type Server struct {
	screensharev1.UnimplementedControlServiceServer

	backend Backend
	events  *events.Broker
	// version is this build, for Hello. It is handed in rather than read here because
	// the build stamp belongs to package main, which is what the linker writes into.
	version string
}

// New returns a service in front of backend, announcing changes from broker.
func New(backend Backend, broker *events.Broker, version string) *Server {
	assert.IsNotNil(backend, "a control service serves a backend")
	assert.IsNotNil(broker, "a control service announces changes from a broker")

	return &Server{backend: backend, events: broker, version: version}
}
