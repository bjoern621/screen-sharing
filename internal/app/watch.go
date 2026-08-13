package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/watch"
	"bjoernblessin.de/screenshare/internal/wire"
)

// relayPollInterval is the cadence the relay is asked at.
// A per-path bitrate is a byte delta between two answers, so only a steady interval turns it into a
// rate, which is half of why the poll is this process's rather than each shell's.
const relayPollInterval = 2 * time.Second

// startRelayPoll is idempotent through sync.Once, as startControl is.
// A second call joins the running loop instead of starting one that asks the relay twice as often.
func (a *App) startRelayPoll() {
	a.relayPollOnce.Do(func() { go a.pollRelay() })
}

// pollRelay fetches until relayStop closes.
//
// The first fetch precedes the first tick, so a shell connecting just after startup reads a real
// answer rather than the opening value.
//
// The wait comes after the fetch, never around it, so a fetch that outruns the interval slows the
// loop down instead of piling requests onto a host that is not answering.
func (a *App) pollRelay() {
	assert.Assert(relayPollInterval > 0, "a poll interval is positive")

	ticker := time.NewTicker(relayPollInterval)
	defer ticker.Stop()

	for {
		a.fetchRelay()

		select {
		case <-a.relayStop:
			return
		case <-ticker.C:
		}
	}
}

// stopRelayPoll is safe with no poll running: the channel closes once, and a loop started later
// finds it closed at its first select.
func (a *App) stopRelayPoll() {
	a.relayStopOnce.Do(func() { close(a.relayStop) })
}

// fetchRelay records the relay's answer and announces it.
//
// The poll loop is the only caller, so the recorded snapshot is the last one taken at a known
// cadence rather than whatever the last caller asked for.
// Everything else reads lastRelayStatus, GetRelayStatus included, so several shells asking what is
// live do not multiply the requests the relay sees and nothing slips a fetch between two polls to
// shorten the interval a byte delta is divided by.
//
// Shells do not poll for the snapshot: the contract has it pushed at this side's interval, for the
// same delta reason (api/proto/screenshare/v1/events.proto).
func (a *App) fetchRelay() relay.Status {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	status := a.relayStatusFor(s)
	a.relayLast.Store(&status)
	a.emit(wire.RelayStatusEvent(status))
	return status
}

// lastRelayStatus is the snapshot the last fetch produced, and an unreachable relay with no reason
// while none has been taken.
//
// The zero value is the honest answer rather than a placeholder: before anything has asked,
// "reachable" is the one thing this process must not claim.
// It is short-lived by construction, since the poll starts with the process and its first fetch
// replaces it before a shell finishes its handshake, which keeps "nobody has asked yet" off the
// screen as "the relay is down".
func (a *App) lastRelayStatus() relay.Status {
	if last := a.relayLast.Load(); last != nil {
		return *last
	}
	return relay.Status{}
}

// WatchKey identifies one viewer window: a stream received over one transport.
// The relay re-serves an ingested stream on all its listeners, so one stream can be watched over
// several transports at once and the stream name alone is no viewer identity.
type WatchKey struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
}

// Watching lists the open viewers, one entry per (stream, transport), and reaps the ones whose
// process has exited.
func (a *App) Watching() []WatchKey {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	keys := []WatchKey{}
	for key, watcher := range a.watchers {
		if watcher.Running() {
			keys = append(keys, key)
		} else {
			delete(a.watchers, key)
		}
	}
	return keys
}

// watching reads the processes rather than anything a caller believed it had opened, so a window
// that closed on its own reports not watching.
// StartWatch asks it ahead of any validation, which is what lets a repeat succeed over a state that
// already holds.
func (a *App) watching(key WatchKey) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	watcher, present := a.watchers[key]
	return present && watcher.Running()
}

// carriesStream refuses a leg that cannot deliver the live stream's format to the named receiving
// engine, naming the legs that would.
//
// The relay re-serves an ingested stream on the listeners whose protocol has a payload mapping for
// its bitstream and on no others: MPEG-TS carries H.264 and H.265, so an SRT viewer opened on a VP9
// or AV1 stream connects and receives nothing, which reads at the viewer as a broken stream.
//
// A stream the snapshot does not carry, or one whose tracks name no format this app knows, passes.
// The snapshot can be older than the stream, and refusing on absent information blocks a viewer
// that would have worked.
// It reads the recorded snapshot rather than fetching one, which also keeps a fetch from landing
// between two polls and dividing a byte delta by the wrong interval.
func (a *App) carriesStream(streamName, transportName, engine string) error {
	format := ""
	for _, path := range a.lastRelayStatus().Paths {
		if path.Name == streamName {
			format = path.Format
			break
		}
	}
	if format == "" {
		return nil
	}
	carried := transport.WatchNamesFor(engine, format)
	if slices.Contains(carried, transportName) {
		return nil
	}
	return fmt.Errorf("%s is %s, which %s cannot carry: watch it over %s",
		streamName, format, transportName, strings.Join(carried, " or "))
}

// StartWatch opens a viewer window for streamName over transportName.
// A pair already being watched is the state asked for and succeeds.
//
// The leg is chosen per viewer and is independent of the publish leg, so one stream can be watched
// over every leg the relay serves it on.
func (a *App) StartWatch(streamName, transportName string) error {
	assert.Assert(streamName != "", "a viewer names the stream it opens", transportName)
	assert.Assert(transportName != "", "a viewer names the leg it receives on", streamName)

	// Deferred, not called: emitViewerState reads the set through Watching and would deadlock against
	// procMu, which the rest of this function holds.
	// It runs on the failing paths too, announcing the set unchanged, which an event carrying a whole
	// state makes harmless and which beats an effect that sometimes announces nothing.
	defer a.emitViewerState()

	key := WatchKey{Name: streamName, Transport: transportName}

	// Read before anything is validated, for the reason StartReceive reads its own state first.
	// The viewer asked for is open, which is what the caller asked for, and a refusal here would
	// leave a shell whose answer went missing with nothing to do but wait.
	// Validating ahead of the question puts that refusal back under another name, since both the relay
	// snapshot and the settings move under a viewer that is already running.
	if a.watching(key) {
		logger.Debugf("'%s' over %s is already being watched", streamName, transportName)
		return nil
	}

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	s, err := a.settingsForCommand(s)
	if err != nil {
		return err
	}

	if err := a.carriesStream(streamName, transportName, capabilities.EngineFfmpeg); err != nil {
		return err
	}

	engine, err := watch.Select(transportName)
	if err != nil {
		return err
	}
	args, env, err := engine.Command(s, streamName, transportName)
	if err != nil {
		return err
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if watcher, present := a.watchers[key]; present && watcher.Running() {
		// The same question under the lock: the read above keeps a repeat cheap, this keeps two starts
		// racing for one pair from opening two windows.
		return nil
	}

	exe, err := ffmpeg.FindExe(engine.Exe())
	if err != nil {
		return err
	}

	// hideWindow false: SW_HIDE hides the viewer's own video window along with the console.
	proc, err := ffmpeg.Start(exe, args, false, false, "watch-"+streamName+"-"+transportName, env, nil, nil,
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
				logger.Warnf("viewer for '%s' over %s failed: %v\n%s\nfull log: %s", streamName, transportName, err, stderrTail, logPath)
			} else {
				logger.Infof("viewer for '%s' over %s closed (log: %s)", streamName, transportName, logPath)
			}
			// The exit event says why this one stopped, the state event beside it which viewers are left.
			// A viewer that closes on its own moves the set with nothing having been called, so the state is
			// announced here as well as at the two calls that change it.
			a.emit(wire.ViewerExitEvent(wire.WatchKey{StreamName: streamName, Transport: transportName}, message, logPath))
			a.emitViewerState()
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("watching '%s' over %s", streamName, transportName)
	a.watchers[key] = proc

	// No readiness is signalled from here.
	// A viewer is connected once the relay reports a reader on the path, which the polled snapshot
	// already carries and which no window system has a say in.
	// A probe for the viewer's own window would: Wayland has no portable form of one.
	return nil
}

// OpenInBrowser hands the relay's player page for streamName over transportName to the machine's
// default browser.
//
// It is the third way to watch and the only one this process does not own, so it is a documented
// departure from idempotency: a second call opens a second tab.
// The tab belongs to the browser, which reports nothing, so nothing is supervised, nothing joins
// the viewer set and there is no stop.
// A viewer event would carry state this backend cannot read back, which is a guess rather than a
// report.
//
// The carriage refusal is shared with StartWatch.
// A leg the stream's format does not cross carries nothing to a browser either, and a page that
// connects and shows nothing is what the check turns into a sentence.
func (a *App) OpenInBrowser(streamName, transportName string) error {
	assert.Assert(streamName != "", "a browser page names the stream it opens", transportName)
	assert.Assert(transportName != "", "a browser page names the leg it opens over", streamName)

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	// The page fetches the stream from the browser, so the URL carries the credential a viewer started
	// here would.
	s, err := a.settingsForCommand(s)
	if err != nil {
		return err
	}

	if err := a.carriesStream(streamName, transportName, transport.EngineBrowser); err != nil {
		return err
	}

	url, ok := transport.BrowserURL(transportName, s, streamName)
	if !ok {
		return fmt.Errorf("the relay serves no player page over %s: open %s over %s",
			transportName, streamName, strings.Join(transport.WatchNames(transport.EngineBrowser), " or "))
	}

	logger.Infof("opening '%s' over %s in the browser: %s", streamName, transportName, url)
	return openInShell(url)
}

// StopWatch closes the viewer for the pair.
// A pair with no viewer open is already in the state asked for, so nothing happens and nothing
// fails.
func (a *App) StopWatch(streamName, transportName string) {
	defer a.emitViewerState()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	key := WatchKey{Name: streamName, Transport: transportName}
	watcher, present := a.watchers[key]
	if present {
		watcher.Stop()
		delete(a.watchers, key)
		logger.Infof("stopped watching '%s' over %s", streamName, transportName)
	}
}

// watchKeys is the open viewers in the contract's shape.
//
// It is the one conversion between WatchKey and wire.WatchKey, which carry the same pair and differ
// only in which package declares them: the contract is built out of wire's.
func (a *App) watchKeys() []wire.WatchKey {
	open := a.Watching()

	keys := make([]wire.WatchKey, 0, len(open))
	for _, key := range open {
		keys = append(keys, wire.WatchKey{StreamName: key.Name, Transport: key.Transport})
	}
	return keys
}

// emitViewerState announces which external viewers are open.
//
// It reads the set through Watching rather than being handed one, so what is announced is what a
// read answers with rather than what a caller believed it had just done.
// It takes procMu, so a caller holding that lock defers it rather than calling it in place.
func (a *App) emitViewerState() {
	a.emit(wire.ViewerStateEvent(a.watchKeys()))
}
