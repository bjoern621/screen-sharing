package app

import (
	"sync"
	"sync/atomic"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/decode"
	"bjoernblessin.de/screenshare/internal/discordclient"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/events"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/update"
)

// App is the backend a shell reaches.
// State reaches the control contract through control.go, changes travel back through events.go.
// Methods sit per domain in settings.go, system.go, publish.go and watch.go.
// This file holds the struct and the process lifecycle.
//
// Each mutex owns one part of the mutable state and none is held while another is taken:
// settingsMu the settings, procMu the children, membersMu this machine's membership statements,
// controlMu the control service handle.
// A method needing settings and children snapshots under settingsMu, releases it, then takes procMu,
// so there is no lock order to deadlock on.
//
// Probe result and last relay snapshot are atomic pointers rather than fields under a lock:
// each is written whole and read on a path that must not wait for the write.
type App struct {
	// Every state change reaches the shells here,
	// so one that acted and one that did not are told the same thing (events.go).
	events *events.Broker
	// Build stamp the control handshake answers with.
	// Handed in: the linker writes it into package main.
	version string
	// What this install knows about the release published beside it (update.go).
	// One per process, so a check one shell asked for is the answer every shell reads.
	updates *update.Manager

	settingsMu sync.Mutex
	settings   settings.Settings

	// Why the persisted settings could not be read, nil when they were.
	// Written once in New, before anything can read it:
	// a store that failed at startup is the state the form opens in.
	// The file it names was moved aside rather than replaced.
	storeNotice *screensharev1.Text

	// This app's side of the group key, token and index service, holding the token last minted
	// (groups.go).
	// One client per process: a second would trade the same group key for a second token and double
	// the requests the service sees per publish.
	groups groupService
	// This app's side of the Discord manager, asked instead of groups while Discord mode is on
	// (discord.go).
	discord discordService
	// What the last Discord pass landed, nil until one has run.
	// Atomic like relayLast: written whole per pass, read on every command build.
	discordLast atomic.Pointer[discordSnapshot]
	// Serialises this machine's membership statements, so a pass in flight when a leave arrives lands
	// before the release rather than after it (members.go).
	// Held across the call to the group service, that order being what it buys.
	membersMu sync.Mutex
	// Snapshot the last fetch produced, nil until one has been taken.
	// One fetch feeds every reader,
	// so several shells asking what is live do not multiply the requests the relay sees (watch.go).
	relayLast atomic.Pointer[relay.Status]
	// Presence this machine last stated, and the group the service answered with (members.go).
	// Written by the relay poll and read wherever a stopped stream has to say whether membership
	// stopped it, so it is held whole, readable on a failure path with nothing to wait on.
	members atomic.Pointer[membership]
	// What this machine states on the Discord client beside it (richpresence.go).
	// Outside every mutex here: the relay poll owns it, opens it and closes it,
	// and no other goroutine reads it.
	presence richPresence
	// relayPollOnce starts the poll that keeps relayLast fresh, relayStopOnce ends it, and both guard
	// relayStop, which the loop selects on.
	// Owned by the process rather than by whichever shell is up: the snapshot is the backend's to keep,
	// and byte-delta bitrates mean something only against one steady interval (watch.go).
	relayPollOnce sync.Once
	relayStopOnce sync.Once
	relayStop     chan struct{}
	// relayDone closes when the loop has left, and relayPolling says whether one ever ran.
	// What lets a shutdown wait for the pass to finish taking this app's activity off Discord,
	// rather than leaving it to the socket closing with the process (richpresence.go).
	relayDone    chan struct{}
	relayPolling atomic.Bool

	// receiveStatsOnce starts the sampling of the running decodes, receiveStatsStopOnce ends it, and
	// both guard receiveStatsStop, which the loop selects on.
	// Its own loop rather than work folded into the relay poll: the relay over the network every 2 s,
	// the decodes out of this process every 1 s (receivestats.go).
	receiveStatsOnce     sync.Once
	receiveStatsStopOnce sync.Once
	receiveStatsStop     chan struct{}

	// testStreamsOnce guards the boot set, so a second Start does not launch a second one.
	// The count is a desired number of slots,
	// so a second launch would ask for that number again on top of the set running (teststreams.go).
	testStreamsOnce sync.Once

	// uplinkOnce guards the first-run line measurement,
	// so a second Start does not upload a second payload over the first (system.go).
	uplinkOnce sync.Once

	// Serializes the probe: the caller asking for the answer runs it, one asking while it runs waits
	// rather than starting a second sweep.
	// A mutex and not a sync.Once, because a cancelled sweep has to be forgotten and a Once keeps
	// whatever its first run produced (system.go).
	encodersMu sync.Mutex
	// Probe result, nil until a probe has finished without being cancelled.
	// Read atomically: a form resolve takes what is there without blocking,
	// while a caller needing the answer waits behind encodersMu (system.go).
	encoders atomic.Pointer[encoders.Availability]

	// controlOnce makes starting the control service idempotent, controlMu guards the handle it
	// produced, and controlStopped records that the shutdown has run.
	// control.go states what each of them covers.
	controlOnce    sync.Once
	controlMu      sync.Mutex
	control        *control.Service
	controlStopped bool

	// The failure this process cannot go on from, read by whoever owns the process (Fatal).
	// Buffered so the reporting goroutine never blocks on an absent reader,
	// and only the first report is kept: what follows a fatal is the shutdown.
	fatal chan error

	// The child every decode runs in, brought up on the first one and replaced after it exits.
	// A GPU reset aborts whichever process was submitting to the ring, so a decode here would cost
	// the control socket and every publish along with the picture (internal/decode).
	decodes *decode.Client

	procMu sync.Mutex
	// Publish session in force, nil while nothing publishes.
	// Carries the settings its pipeline was built from,
	// what a live stream is held against when the form moves off them (publish.go).
	run *publishRun
	// Relaunch a pipeline that died on its own is waiting on, nil when none is pending.
	// Never set beside run: a retry lives between the exit that armed it and the launch that consumes
	// it (publish_retry.go).
	retry *publishRetry
	// Where the publishing machine's pointer was last seen.
	// Outside procMu because it is written faster than any frame rate, and a lock every publish path
	// takes does not belong on that.
	pointerAt pointerState
	// Local decode of the stream this machine is sending,
	// nil while nothing publishes and while a publish runs without one.
	// A field of its own rather than an entry in receivers because no StreamRef names it: the frames
	// never crossed the relay (preview.go).
	preview  *previewRun
	watchers map[StreamRef]*ffmpeg.Proc
	// Decodes running on the host, keyed as the watchers are, by a stream and its receiving leg:
	// the relay re-serves each stream on all its listeners,
	// so one stream can be decoded over several at once (receive.go).
	receivers map[StreamRef]*decode.Handle
	// Screens being read for the setup wizard, keyed by the index the output is enumerated under.
	// A map of their own rather than entries beside the decodes: an output names one, not a StreamRef
	// (monitorpreview.go).
	monitorPreviews map[int]*decode.Handle
	// The synthetic set, one entry per slot,
	// keyed by the slot number the stream is named after.
	// An entry is a child that is publishing or a relaunch waiting to start one,
	// and a slot with no entry is neither (teststreams.go).
	testStreams map[int]*testStream
	// How many slots the set is meant to hold, the desired state the exits converge on:
	// a child dying below it is relaunched into its slot, one dying at or above it is let go.
	testStreamsWanted int
}

// New builds the backend.
// version is the build stamp the control handshake answers with.
func New(version, channel string) *App {
	assert.Assert(version != "", "a build names the version its shells report")

	s, err := settings.Load()
	var notice *screensharev1.Text
	if err != nil {
		// An unreadable store is an Umgebungsfehler, so the run goes on, on the defaults.
		// The form opens on values the user did not choose,
		// so that fact and where the old values went travel to it rather than staying in the log alone.
		// Why the file could not be read is the operating system's answer,
		// and stays in the log where the reader who can act on it is looking.
		logger.Warnf("settings not restored: %v", err)
		notice = settings.StoreNotice(
			screensharev1.TextCode_TEXT_CODE_SETTINGS_STORE_UNREADABLE, err)
	}

	a := &App{
		events:           events.New(),
		version:          version,
		settings:         s,
		storeNotice:      notice,
		groups:           groupclient.New(),
		discord:          discordclient.New(),
		relayStop:        make(chan struct{}),
		relayDone:        make(chan struct{}),
		receiveStatsStop: make(chan struct{}),
		fatal:            make(chan error, 1),
		decodes:          decode.NewClient(),
		watchers:         map[StreamRef]*ffmpeg.Proc{},
		receivers:        map[StreamRef]*decode.Handle{},
		monitorPreviews:  map[int]*decode.Handle{},
		testStreams:      map[int]*testStream{},
	}

	// After the struct, the manager announcing through the broker it is built beside.
	a.updates = newUpdates(version, channel, a.emitUpdateState)
	return a
}

// Fatal reports the failure that leaves this process with nothing left to do, and stays empty while
// there is none.
//
// Reached by a control endpoint another backend holds, never by a shell (control.go).
// The owner of the process reads this beside the signals it stops on and stops the same way for both,
// and Stop takes the children down whichever of the two ended the run.
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
// The form then opens on the defaults, and the statement names where the old values were moved to.
func (a *App) StoreNotice() *screensharev1.Text {
	return a.storeNotice
}

// Start brings up everything that runs beside the process.
//
// Nothing here is waited for: the control socket and each poll get a goroutine of their own,
// so the process comes up at its own speed rather than at the socket's.
// Idempotent: the control service, both polls and the boot set are each guarded by a sync.Once,
// so a second Start opens no second socket, runs no second loop and launches no second synthetic set.
//
// The relay poll starts here rather than when a shell asks, the snapshot being the backend's to keep:
// polling only while somebody watched would answer GetRelayStatus with the opening value,
// unreachable and no reason given, which reads on screen as a relay that is down.
//
// The synthetic set comes up here for the same reason and keeps itself up:
// the viewer roster carries streams whether or not this machine publishes,
// and a relay not up when this process starts is the normal case rather than a failure
// (teststreams.go).
func (a *App) Start() {
	a.startControl()
	a.startRelayPoll()
	a.startReceiveStatsPoll()
	a.testStreamsOnce.Do(func() { go a.startTestStreamsAtBoot() })
	a.uplinkOnce.Do(func() { go a.measureUplinkAtBoot() })
}

// Stop takes every child down, so no orphan ffmpeg keeps encoding after the process ends.
func (a *App) Stop() {
	// The contract closes before the children do,
	// so an effect a shell asked for cannot start one of them behind the teardown.
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
	// A receive pipeline like the ones below, stopped apart from them because it is not in that map:
	// it goes with the publish it belongs to (preview.go).
	a.stopPreviewLocked()
	// Consent handed back before the process ends,
	// rather than leaving the compositor to notice the bus connection go.
	a.releaseSourcesLocked()
	for _, watcher := range a.watchers {
		watcher.Stop()
	}
	// Every decode goes with the host running them, which takes its pipelines to NULL before it
	// exits and is waited for (internal/decode).
	// The monitor previews and the local preview go with it too, being pipelines on the same host.
	clear(a.receivers)
	clear(a.monitorPreviews)
	a.decodes.Close()

	// The wanted count drops to zero with the children, so a relaunch pending behind a dead child
	// cannot start a publisher into a process on its way out.
	a.stopTestStreamsLocked()
}
