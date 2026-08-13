// Package control serves the control contract: one ControlService implementation, over a local
// socket, in front of the backend that owns the product.
//
// docs/ipc-api.md states the rule: a shell draws what the backend describes and asks it to act,
// deciding nothing.
// This is the describing and acting side, and it carries no domain rules - the tables sit in
// capabilities, transport, gpupath and publish, their presentation in form, and both shaped for the
// wire in wire - so a method here dispatches rather than decides.
//
// Three kinds of method, which is that rule in executable form: a read computes and changes
// nothing, an effect does the one thing the user asked for, and Subscribe carries what changed.
// A method that is none of the three does not belong here.
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
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Backend is everything the service reaches to answer a read or run an effect.
//
// An interface rather than a concrete app, so the contract is servable in front of a test double
// with no window, no encoder and no relay.
// That is the property the boundary rule was chosen for: what a shell displays is computed
// somewhere a test can reach.
//
// Every method is a read of state the implementation owns or one named effect.
// None of them formats, greys or labels: that is form's work, and a backend answering with a
// sentence for the screen would author the vocabulary a second time.
type Backend interface {
	// --- Reads ---

	// Settings is what the backend holds, and the draft a form starts from.
	Settings() settings.Settings
	// StoreNotice says why the persisted settings could not be restored, nil where they were.
	StoreNotice() *screensharev1.Text
	Monitors() []display.Monitor
	// Platform names the OS and, on Linux, the display server.
	Platform() platform.Info
	// Encoders is what this machine can really encode, per engine.
	// The probe runs once and is held for the process lifetime: the first call costs seconds and every
	// one after it costs nothing.
	Encoders(ctx context.Context) encoders.Availability
	// CachedEncoders is what the probe found, and the zero value where nothing has probed.
	// A form resolves on every keystroke and a probe takes seconds, so a resolve reads what is known:
	// an unprobed engine is one nothing is greyed on, not one with nothing usable.
	CachedEncoders() encoders.Availability
	// AudioDevices is what this machine offers inside each audio kind, enumerated once and answered
	// from memory after that.
	// Read on a resolve for the reason CachedEncoders is: the enumeration is a subprocess.
	AudioDevices() []platform.AudioDevice
	PublishState() wire.PublishSnapshot
	// RelayStatus is the last snapshot the backend's own poll took, never a fresh fetch.
	RelayStatus() relay.Status
	// Watching lists the external viewers open.
	Watching() []wire.WatchKey
	// ReceiveState is every stream being decoded, read off the running pipelines rather than
	// remembered: the chain that ran is not always the one asked for.
	ReceiveState() []wire.ReceiveStream
	// TestStreamsRunning counts the synthetic publishers alive, which is not the count that was asked
	// for: one that died on its own drops out.
	TestStreamsRunning() int
	// MaxTestStreams bounds how many synthetic publishers run at once.
	//
	// Readable rather than only enforced, so an over-large request is refused where every other
	// request-earned refusal here is made: above the call, with RESOURCE_EXHAUSTED, the code the
	// contract names for a bounded resource.
	// A bound only reachable by asking for too much would come back as an untyped error, leaving a
	// saturated machine indistinguishable from a missing binary.
	MaxTestStreams() int
	// --- Effects ---
	//
	// The measurements are among them: each runs the real thing, takes seconds, and leaves behind a
	// result a later read sees, which is what an effect is.
	// The probe replaces what CachedEncoders answers with, and neither of the other two is anything a
	// shell may call on a keystroke.

	// MeasureUplink probes this machine's real upload throughput, in Mbit/s.
	MeasureUplink(ctx context.Context) (float64, error)
	// MeasureEncodeRate times the configured encoder on generated frames.
	MeasureEncodeRate(ctx context.Context, s settings.Settings) (encoderate.Rate, error)

	// SaveSettings persists the settings the shell holds and leaves a running stream alone: reaching a
	// live pipeline is ApplyToStream's business.
	SaveSettings(s settings.Settings) error
	// StartPublish persists the settings and starts the encoder on them.
	StartPublish(s settings.Settings) error
	// ApplyToStream restarts the running stream on new settings.
	ApplyToStream(s settings.Settings) error
	// StopPublish ends the stream, running or waiting out a backoff.
	StopPublish()
	// StartWatch opens an external viewer for one stream over one transport.
	StartWatch(key wire.WatchKey) error
	StopWatch(key wire.WatchKey)
	// StartReceive opens a decode for one stream on one leg inside the backend, and StopReceive closes
	// one.
	// The tile path's counterpart of the two above, and what they open is a decode and never a tile.
	//
	// toneMap rolls an HDR stream down into the range a standard display shows.
	// The rung is an element in the pipeline, so a decode already open on the other answer is rebuilt
	// to reach the state the call names.
	StartReceive(key wire.WatchKey, toneMap bool) error
	StopReceive(key wire.WatchKey)
	// SetReceiveAudio sets how loud one decode plays and whether it plays at all, and refuses where
	// nothing is decoding the pair.
	// Loudness belongs to the decode rather than to a window drawing it: one pipeline holds one audio
	// branch, so a per-window volume would be several controls over one element.
	SetReceiveAudio(key wire.WatchKey, volume float64, muted bool) error
	// AudioLevels is how loud every decode carrying audio is at this instant.
	// A read and not a stream: the cadence belongs to the service that ticks it, not to the backend
	// that measures.
	AudioLevels() []wire.AudioLevel
	// Pointer is where the publishing machine's pointer is at this instant, and false where the
	// publish in force reports none.
	//
	// That covers every cursor mode but the one sending the position instead of drawing it, and every
	// engine whose child cannot read one.
	// A read and not a stream, for the reason AudioLevels is one.
	Pointer() (pointer.Position, bool)
	// SubscribeFrames opens one consumer's view of a decode already running, and refuses where nothing
	// is decoding the pair.
	//
	// Here rather than on a second interface, because the frame service serves the same backend this
	// one does: a subscription draws from the decode StartReceive opened, and two interfaces onto it
	// would be two ideas of which decodes exist.
	SubscribeFrames(key wire.WatchKey) (FrameStream, error)
	// SubscribePreviewFrames opens one consumer's view of the running publish's local preview, and
	// refuses where nothing is publishing with one behind it.
	//
	// A method of its own rather than a key the one above could take, because the preview has no key:
	// what it draws never crossed the relay, so no transport names it, and a synthetic one would put a
	// protocol into the table every consumer reads (preview.go).
	SubscribePreviewFrames() (FrameStream, error)
	// StartMonitorPreview reads one of this machine's screens into a picture the frame channel can
	// hand over, and StopMonitorPreview closes one.
	// The wizard's counterpart of the receive pair, and what they open is a screen capture and never a
	// tile.
	StartMonitorPreview(monitor int) error
	StopMonitorPreview(monitor int)
	// MonitorPreviewState is every monitor being previewed, read off the running pipelines rather than
	// remembered: a preview that has produced no frame is still opening the screen.
	MonitorPreviewState() []wire.PreviewedMonitor
	// SubscribeMonitorFrames opens one consumer's view of a monitor preview already running, and
	// refuses where nothing is previewing that screen.
	//
	// A third method for the reason the preview has one of its own: the three name three different
	// kinds of thing, and one key would be a stream name for the first, nothing for the second and an
	// output index for the third.
	SubscribeMonitorFrames(monitor int) (FrameStream, error)
	// StartTestStreams launches synthetic publishers, replacing a running set.
	StartTestStreams(count int) error
	StopTestStreams()
	// OpenInBrowser opens the relay's player page for one stream in the machine's default browser, and
	// refuses a leg the relay serves no page on or the stream's format does not cross.
	// It opens no viewer this backend owns, so nothing it does reaches the viewer state.
	OpenInBrowser(key wire.WatchKey) error
	// ForgetPortalConsent drops the stored screen-capture consent.
	ForgetPortalConsent() error
	// OpenLog opens one run log in the machine's default application, and OpenLogsFolder the directory
	// holding them.
	OpenLog(path string) error
	OpenLogsFolder() error
}

// FrameStream is one consumer's running subscription to a decode's frames.
//
// An interface for the reason Backend is one: the frame service has to be servable in front of
// something with no GPU and no pipeline, and a concrete subscription would turn every test of this
// service into a test of the exporter behind it.
//
// The methods are the protocol.
// Events carries what the backend says and closes when the subscription ends, Err then states why,
// Release hands one slot back, SetRenderSize states how many pixels the consumer draws at, and
// Close ends the subscription from this side and frees the memory.
type FrameStream interface {
	Events() <-chan receive.Event
	Err() error
	Release(generation uint64, slot int, serial uint64)
	SetRenderSize(width, height int)
	Close()
}

// The contract version this build implements.
//
// The major is settled on Hello, before any other method is reached, so a mismatch reads as a
// sentence naming both versions instead of as a field that arrives silently empty.
// The minor is informational: a shell built against a lower minor works, one built against a higher
// minor may find a method missing.
const (
	ProtocolMajor = 1
	ProtocolMinor = 0
)

// Server is the ControlService implementation.
//
// It keeps no state beyond its collaborators.
// Every answer is read through to the backend, the tables or the broker on the call that asks for
// it, which is what stops two copies of one fact drifting apart.
type Server struct {
	screensharev1.UnimplementedControlServiceServer

	backend Backend
	events  *events.Broker
	// version is this build, for Hello.
	// Handed in rather than read here: the build stamp belongs to package main, which is what the
	// linker writes into.
	version string
}

// New serves backend, announcing its changes through broker.
func New(backend Backend, broker *events.Broker, version string) *Server {
	assert.IsNotNil(backend, "a control service serves a backend")
	assert.IsNotNil(broker, "a control service announces changes from a broker")
	assert.Assert(version != "", "a control service names the build a handshake answers with")

	return &Server{backend: backend, events: broker, version: version}
}
