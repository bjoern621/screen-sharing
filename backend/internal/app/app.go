package app

import (
	"sync"
	"sync/atomic"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// App is the backend a shell reaches.
// control.go serves its state to the control contract, and changes travel the other way through one
// function (events.go).
// Methods are grouped by domain across settings.go, system.go, publish.go and watch.go, and this
// file holds the struct and the process lifecycle.
//
// Each mutex owns one part of the mutable state and none is held while another is taken:
// settingsMu the settings, procMu the children (the publish run and the watchers), membersMu the
// statements about this machine's membership, controlMu the control service's handle.
// A method that needs settings and children snapshots the settings under settingsMu, releases it,
// then takes procMu, so there is no lock order to deadlock on.
//
// The probe result and the last relay snapshot are atomic pointers rather than fields under a lock:
// each is written whole and read on a path that must not wait for the write.
// A form resolve reads what has been probed without waiting for a probe that is running, and the
// control service reads the relay snapshot without waiting for a fetch in flight.
type App struct {
	// Every state change reaches the shells here, whoever made it, so one that acted and one that did
	// not are told the same thing (events.go).
	events *events.Broker
	// Build stamp the control handshake answers with.
	// Handed in because the linker writes it into package main.
	version string

	settingsMu sync.Mutex
	settings   settings.Settings

	// Why the persisted settings could not be read, nil when they were.
	// Written once, in New, before anything can read it, and read through from there: a store that
	// failed at startup is the state the form opens in, and the file it names was moved aside rather
	// than replaced.
	storeNotice *screensharev1.Text

	relay *relay.Client
	// This app's side of the group key, token and index service, holding the token it last minted
	// (groups.go).
	// One client for the process, because the token is one credential: a second would trade the same
	// group key for a second token and double the requests the service sees per publish.
	groups groupService
	// Serialises the statements about this machine's membership: presence, join and leave are one
	// thing happening in one order, so a pass in flight when a leave arrives lands before the release
	// rather than after it (members.go).
	// Held across the call to the group service, the order there being what it buys.
	membersMu sync.Mutex
	// Snapshot the last fetch produced, nil until one has been taken.
	// Every fetch writes it and the control service reads it, so several shells asking what is live do
	// not multiply the requests the relay sees (watch.go).
	relayLast atomic.Pointer[relay.Status]
	// Presence this machine last stated, and the group the service answered with (members.go).
	// Written by the same poll that fetches the relay and read wherever a stopped stream has to say
	// whether membership is what stopped it, so it is held as the relay snapshot is: whole, and never
	// behind a lock a failure path would wait on.
	members atomic.Pointer[membership]
	// relayPollOnce starts the poll that keeps relayLast fresh, relayStopOnce ends it, and both guard
	// relayStop, which the loop selects on.
	// The poll belongs to this process rather than to whichever shell happens to be up: the contract
	// makes the snapshot the backend's to keep, and the byte-delta bitrates mean something only
	// against one steady interval (watch.go).
	relayPollOnce sync.Once
	relayStopOnce sync.Once
	relayStop     chan struct{}

	// receiveStatsOnce starts the sampling of the running decodes, receiveStatsStopOnce ends it, and
	// both guard receiveStatsStop, which the loop selects on.
	// A second loop rather than work folded into the relay poll, because the two measure different
	// things at different rates: the relay over the network every 2 s, the decodes out of this process
	// every 1 s (receivestats.go).
	receiveStatsOnce     sync.Once
	receiveStatsStopOnce sync.Once
	receiveStatsStop     chan struct{}

	// testStreamsOnce guards the boot set, so a second Start does not launch a second one.
	// The count is a desired number of slots, and a second launch would ask for that number again on
	// top of the set already running (teststreams.go).
	testStreamsOnce sync.Once

	// Serializes the probe: the caller that asks for the answer runs it, and one asking while it runs
	// waits rather than starting a second sweep.
	// A mutex and not a sync.Once, because a sweep that was cancelled has to be forgotten and a Once
	// keeps whatever its first run produced (system.go).
	encodersMu sync.Mutex
	// Probe result, nil until a probe has finished without being cancelled.
	// A pointer read atomically because the readers want different things: a caller that needs the
	// answer waits behind encodersMu, and a form resolve takes what is there and never waits
	// (system.go).
	encoders atomic.Pointer[encoders.Availability]

	// controlOnce makes starting the control service idempotent, controlMu guards the handle it
	// produced, and controlStopped records that the shutdown has run.
	// control.go states what each of them covers.
	controlOnce    sync.Once
	controlMu      sync.Mutex
	control        *control.Service
	controlStopped bool

	// The failure this process cannot go on from, read by whoever owns the process (Fatal).
	// Buffered so the reporting goroutine never blocks on a reader that has not arrived, and only the
	// first report is kept: what follows a fatal is the shutdown, and a second reason for the same
	// exit changes nothing about it.
	fatal chan error

	procMu sync.Mutex
	// Publish session in force, nil while nothing publishes.
	// It carries the settings its pipeline was built from, which is what a live stream is held against
	// when the form moves off them (publish.go).
	run *publishRun
	// Relaunch a pipeline that died on its own is waiting on, nil when none is pending.
	// Never set beside run: a retry lives between the exit that armed it and the launch that consumes
	// it (publish_retry.go).
	retry *publishRetry
	// Where the publishing machine's pointer was last seen.
	// Outside procMu because it is written at the reader's own rate, faster than any frame rate, and a
	// lock every publish path takes does not belong on that.
	pointerAt pointerState
	// Local decode of the stream this machine is sending, nil while nothing publishes and while a
	// publish runs without one.
	// A field of its own rather than an entry in receivers because no StreamRef names it: the frames
	// never crossed the relay, so no transport carried them (preview.go).
	preview  *previewRun
	watchers map[StreamRef]*ffmpeg.Proc
	// Decodes running inside this process, keyed as the watchers are, by a stream and the leg it is
	// received over: the relay re-serves each stream on all its listeners, so one stream can be
	// decoded over several at once (receive.go).
	receivers map[StreamRef]*receive.Receiver
	// Screens being read for the setup wizard, keyed by the index the output is enumerated under.
	// A map of their own rather than entries beside the decodes, because an output and not a StreamRef
	// names one: nothing encoded these frames and no transport carried them (monitorpreview.go).
	monitorPreviews map[int]*receive.Receiver
	// The synthetic set, one entry per slot it holds, keyed by the slot number the stream is named
	// after.
	// An entry is a child that is publishing or a relaunch waiting to start one, and a slot with no
	// entry is neither (teststreams.go).
	testStreams map[int]*testStream
	// How many slots the set is meant to hold, the desired state the exits converge on: a child that
	// dies below it is relaunched into its slot, one that dies at or above it is let go.
	testStreamsWanted int
}

// New builds the backend.
// version is the build stamp the control handshake answers with.
func New(version string) *App {
	assert.Assert(version != "", "a build names the version its shells report")

	s, err := settings.Load()
	var notice *screensharev1.Text
	if err != nil {
		// An unreadable store is an Umgebungsfehler, so the run goes on, on the defaults.
		// The form is about to open on values the user did not choose, so the fact travels to it rather
		// than staying in the log alone.
		// What travels is that fact and where the old values were moved to.
		// Why the file could not be read is the operating system's answer and stays in the log, where
		// the reader who can act on it is looking.
		logger.Warnf("settings not restored: %v", err)
		notice = settings.StoreNotice(
			screensharev1.TextCode_TEXT_CODE_SETTINGS_STORE_UNREADABLE, err)
	}

	return &App{
		events:           events.New(),
		version:          version,
		settings:         s,
		storeNotice:      notice,
		relay:            relay.New(),
		groups:           groupclient.New(),
		relayStop:        make(chan struct{}),
		receiveStatsStop: make(chan struct{}),
		fatal:            make(chan error, 1),
		watchers:         map[StreamRef]*ffmpeg.Proc{},
		receivers:        map[StreamRef]*receive.Receiver{},
		monitorPreviews:  map[int]*receive.Receiver{},
		testStreams:      map[int]*testStream{},
	}
}

// Fatal reports the failure that leaves this process with nothing left to do, and stays empty while
// there is none.
//
// A control endpoint another backend holds reaches it, because no shell would ever reach this one
// (control.go).
// The owner of the process reads this beside the signals it stops on and stops the same way for
// both, and Stop takes the children down whichever of the two ended the run.
func (a *App) Fatal() <-chan error { return a.fatal }

// fail keeps the first fatal failure and drops what follows it.
func (a *App) fail(err error) {
	assert.IsNotNil(err, "a fatal failure states what it was")

	select {
	case a.fatal <- err:
	default:
	}
}

// StoreNotice states why the persisted settings could not be restored, nil when they were.
// The form then opens on the defaults, and the statement carries where the old values were moved to.
func (a *App) StoreNotice() *screensharev1.Text {
	return a.storeNotice
}

// Start brings up everything that runs beside the process.
//
// Nothing here is waited for: the control service opens its socket on a goroutine of its own
// (control.go) and each poll runs on one of its own, so the process comes up at its own speed
// rather than at the socket's.
// Idempotent: the control service, both polls and the boot set are each guarded by a sync.Once, so a
// second Start opens no second socket, runs no second loop and launches no second synthetic set.
//
// The relay poll starts here rather than when a shell asks, because the contract makes the snapshot
// this side's to keep: a poll that ran only while somebody watched would answer GetRelayStatus with
// the opening value, unreachable and no reason given, for as long as nothing had asked, which reads
// on screen as a relay that is down.
//
// The synthetic set comes up here for the same reason and keeps itself up: the viewer roster is
// meant to carry streams whether or not this machine publishes, and a relay that is not up yet when
// this process starts is the normal case rather than a failure (teststreams.go).
func (a *App) Start() {
	a.startControl()
	a.startRelayPoll()
	a.startReceiveStatsPoll()
	a.testStreamsOnce.Do(func() { go a.startTestStreamsAtBoot() })
}

// Stop takes every child down, so no orphan ffmpeg keeps encoding after the process ends.
func (a *App) Stop() {
	// The contract closes before the children do, so an effect a shell asked for cannot start one of
	// them behind the teardown below.
	a.stopControl()
	a.stopRelayPoll()
	a.stopReceiveStatsPoll()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.run != nil {
		a.run.handle.Stop()
	}
	// A pending relaunch would start an encoder into a process on its way out.
	a.cancelRetryLocked()
	// The preview is a receive pipeline like the ones below and is stopped apart from them because it
	// is not in that map: it goes with the publish it belongs to (preview.go).
	a.stopPreviewLocked()
	// Handing the consent back before the process ends, rather than leaving the compositor to notice
	// the bus connection go.
	a.releaseSourcesLocked()
	for _, watcher := range a.watchers {
		watcher.Stop()
	}
	// The receive pipelines are the teardown this process waits on, and they are waited on together
	// rather than one after another.
	//
	// Each Stop blocks until its pipeline reaches NULL, bounded by receive's own timeout, so stopping
	// them in a row would bound this shutdown at that timeout times the number of pipelines, which
	// have nothing to do with each other.
	// Stopped together, the wait is the slowest pipeline's.
	//
	// Waiting at all is the point: what follows this function is the process exiting, and a pipeline
	// still running then is torn down by the operating system with its threads wherever they happen to
	// be, which on Windows is how a process ends up unkillable with the control pipe still in its
	// hands.
	// The count below says whether the exit about to happen is the clean one.
	//
	// The monitor previews join the group: receive pipelines keyed by an output rather than by a
	// stream, and nothing about stopping one differs (monitorpreview.go).
	pipelines := make([]*receive.Receiver, 0, len(a.receivers)+len(a.monitorPreviews))
	for _, receiver := range a.receivers {
		pipelines = append(pipelines, receiver)
	}
	for _, receiver := range a.monitorPreviews {
		pipelines = append(pipelines, receiver)
	}

	var stopping sync.WaitGroup
	var running atomic.Int32
	for _, receiver := range pipelines {
		stopping.Add(1)
		go func() {
			defer stopping.Done()
			if !receiver.Stop() {
				running.Add(1)
			}
		}()
	}
	stopping.Wait()
	if left := running.Load(); left > 0 {
		logger.Warnf("%d receive pipeline(s) were still running at shutdown; the streams they name are in the lines above", left)
	}
	// The wanted count drops to zero with the children, so a relaunch pending behind a dead child
	// cannot start a publisher into a process on its way out.
	a.stopTestStreamsLocked()
}
