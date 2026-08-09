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
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// App is the backend the shell in front of it reaches. control.go serves its state to
// the control contract, and events flow the other way through one function
// (events.go). Methods are grouped by domain across settings.go, system.go,
// publish.go and watch.go; this file holds the struct and process lifecycle.
//
// Three mutexes guard the mutable state and none is held while another is taken:
// settingsMu guards settings, procMu guards the children (the publish run and the
// watchers), and controlMu guards the control service's handle. Methods that need
// settings and children snapshot settings under settingsMu first, release it, then
// take procMu, so there is no lock ordering to deadlock on.
//
// The probe result and the last relay snapshot are atomic pointers rather than
// fields under a lock, because each is written whole and is read on a path that must
// not wait for the write: a form resolve reads what has been probed without waiting
// for a probe that is running, and the control service reads the relay snapshot
// without waiting for a fetch that is in flight.
type App struct {
	// events announces every state change to the control shells, whoever made it, so a
	// shell that acted and one that did not learn it the same way (events.go).
	events *events.Broker
	// version is this build's stamp, which the control handshake answers with. It is
	// handed in because the linker writes it into package main.
	version string

	settingsMu sync.Mutex
	settings   settings.Settings

	// storeNotice states why the persisted settings could not be read, nil when
	// they were. It is written once, in New, before the frontend exists, and read
	// through from there: a store that failed at startup is the state the form opens
	// in, and the file it names has been moved aside rather than replaced.
	storeNotice *screensharev1.Text

	relay *relay.Client
	// relayLast is the snapshot the last fetch produced, nil until one has been taken.
	// Every fetch writes it and the control service reads it, so several shells asking
	// what is live do not multiply the requests the relay sees (watch.go).
	relayLast atomic.Pointer[relay.Status]
	// relayPollOnce starts the poll that keeps relayLast fresh and relayStopOnce ends
	// it, both guarding relayStop, which the loop selects on. The poll belongs to this
	// process rather than to whichever shell happens to be up: the contract states the
	// snapshot is the backend's to keep, and the byte-delta bitrates are only meaningful
	// against one steady interval (watch.go).
	relayPollOnce sync.Once
	relayStopOnce sync.Once
	relayStop     chan struct{}

	// encodersOnce runs the probe once per process, so the caller that asks for the
	// answer waits for it and every caller after that does not.
	encodersOnce sync.Once
	// encoders is the probe result, nil until the probe has finished. It is a pointer
	// read atomically because the two readers want different things: a caller that
	// needs the answer waits through encodersOnce, and a form resolve reads what is
	// there now and never waits (system.go).
	encoders atomic.Pointer[encoders.Availability]

	// controlOnce makes starting the control service idempotent, controlMu guards the
	// handle it produced, and controlStopped says the shutdown has already run. All
	// three belong to control.go, which states what each of them covers.
	controlOnce    sync.Once
	controlMu      sync.Mutex
	control        *control.Service
	controlStopped bool

	procMu sync.Mutex
	// run is the publish session in force, nil while nothing publishes. It carries the
	// settings its pipeline was built from, which is what a live stream is held against
	// when the form moves off them (publish.go).
	run *publishRun
	// retry is the relaunch a pipeline that died on its own is waiting on, nil when none
	// is pending. It and run are never both set: the retry exists exactly between the
	// exit that armed it and the launch that consumes it (publish_retry.go).
	retry *publishRetry
	// preview is the local decode of the stream this machine is sending, nil while
	// nothing publishes or while a publish runs without one. It is a field of its own
	// rather than an entry in receivers because it is not keyed by a WatchKey: the
	// frames never crossed the relay, so no transport carried them (preview.go).
	preview  *previewRun
	watchers map[WatchKey]*ffmpeg.Proc
	// receivers are the decodes running inside this process, keyed the way the watchers
	// are: a stream and the leg it is received over, because the relay re-serves each
	// stream on all its listeners and one stream can be decoded over several at once
	// (receive.go).
	receivers map[WatchKey]*receive.Receiver
	// testStreams is the synthetic set, one entry per slot the set holds, keyed by the
	// slot number the stream is named after. An entry is a child that is publishing or
	// a relaunch waiting to start one; a slot with no entry is neither (teststreams.go).
	testStreams map[int]*testStream
	// testStreamsWanted is how many slots the set is supposed to hold. It is the desired
	// state the exits converge on: a child that dies below it is relaunched into its
	// slot, and one that dies at or above it is let go.
	testStreamsWanted int
}

// New builds the backend. version is this build's stamp, which the control
// handshake answers with so a shell can name what it is talking to.
func New(version string) *App {
	assert.Assert(version != "", "a build names the version its shells report")

	s, err := settings.Load()
	var notice *screensharev1.Text
	if err != nil {
		// The form is about to open on values the user did not choose, so the fact
		// travels to it rather than staying in the log alone. What travels is the fact
		// and the path the old values were moved to; why the file could not be read is
		// the operating system's answer and stays in the log, where the one reader who
		// can act on it is looking.
		logger.Warnf("settings not restored: %v", err)
		notice = settings.StoreNotice(
			screensharev1.TextCode_TEXT_CODE_SETTINGS_STORE_UNREADABLE, err)
	}

	return &App{
		events:      events.New(),
		version:     version,
		settings:    s,
		storeNotice: notice,
		relay:       relay.New(),
		relayStop:   make(chan struct{}),
		watchers:    map[WatchKey]*ffmpeg.Proc{},
		receivers:   map[WatchKey]*receive.Receiver{},
		testStreams: map[int]*testStream{},
	}
}

// StoreNotice states why the persisted settings could not be restored, nil when they
// were. The form opens on the defaults in that case, and the file holding the old
// values has been moved aside rather than overwritten, so the statement carries where
// they are.
func (a *App) StoreNotice() *screensharev1.Text {
	return a.storeNotice
}

// Start brings up everything that runs beside the process.
//
// Nothing here waits for what it starts: the control service opens its socket on a
// goroutine of its own (control.go) and the relay poll runs on one of its own
// (watch.go), so the process is up at its own speed rather than at the socket's.
//
// The relay poll starts here rather than when a shell asks, because the contract says
// this side keeps the snapshot: a poll that only ran while somebody was watching would
// answer GetRelayStatus with the opening value - unreachable, no reason given - for as
// long as nothing had asked, which reads on screen as a relay that is down.
//
// The synthetic set comes up here for the same reason and keeps itself up: the viewer
// roster is meant to carry streams whether or not this machine publishes, and a relay
// that is not up yet when this process starts is the normal case rather than a failure
// (teststreams.go).
func (a *App) Start() {
	a.startControl()
	a.startRelayPoll()
	go a.startTestStreamsAtBoot()
}

// Stop kills every child so no orphan ffmpeg keeps encoding after the process ends.
func (a *App) Stop() {
	// The contract closes before the children do, so an effect a shell asked for
	// cannot start one of them behind the teardown below.
	a.stopControl()
	a.stopRelayPoll()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.run != nil {
		a.run.handle.Stop()
	}
	// A pending relaunch would start an encoder into a process on its way out.
	a.cancelRetryLocked()
	// The preview is a receive pipeline like the ones below, and is stopped here rather
	// than with them because it is not in that map: it goes with the publish it belongs
	// to (preview.go).
	a.stopPreviewLocked()
	for _, watcher := range a.watchers {
		watcher.Stop()
	}
	// The receive pipelines are the one teardown this process waits on, and they are
	// waited on together rather than one after another.
	//
	// Each of them blocks until its pipeline reaches NULL, bounded by receive's own
	// timeout, so stopping five streams in a row would bound this shutdown at five
	// times that where the pipelines have nothing to do with each other. Stopped
	// together the wait is one pipeline's, whichever is slowest.
	//
	// Waiting at all is the point: what follows this function is the process exiting,
	// and a pipeline still running then is torn down by the operating system with its
	// threads wherever they happen to be, which on Windows is how a process ends up
	// unkillable with the control pipe still in its hands. The count below is what
	// says whether the exit about to happen is the clean one.
	var stopping sync.WaitGroup
	var running atomic.Int32
	for _, receiver := range a.receivers {
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
	// The set goes off rather than down: a relaunch pending behind a dead child would
	// otherwise start a publisher into a process on its way out.
	a.stopTestStreamsLocked()
}
