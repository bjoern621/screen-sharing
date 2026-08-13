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

// relayPollInterval is how often the poll below asks the relay what is live.
// Per-path bitrates are byte deltas between two answers, so they are only meaningful against a
// steady interval - which is the other half of why the poll is this process's rather than each
// shell's.
const relayPollInterval = 2 * time.Second

// startRelayPoll begins the poll that keeps the recorded snapshot fresh.
//
// Idempotent, and sync.Once is what states it, the same way startControl is:
// a second call joins the loop already running rather than starting a second one asking the relay
// twice as often.
func (a *App) startRelayPoll() {
	a.relayPollOnce.Do(func() { go a.pollRelay() })
}

// pollRelay asks the relay what is live, forever, until the process stops.
//
// The first fetch happens before the first tick, so a shell that connects a moment after this
// process started reads a real answer rather than the opening value.
//
// A fetch that outruns the interval is not queued behind itself: the wait comes after the fetch,
// so an unreachable relay running into the client's own timeout slows the loop down instead of
// piling requests onto a host that is not answering.
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

// stopRelayPoll ends the poll, and is safe to call with none running: the channel is closed once,
// and a loop that has not started yet finds it closed at its first select.
func (a *App) stopRelayPoll() {
	a.relayStopOnce.Do(func() { close(a.relayStop) })
}

// fetchRelay asks the relay what is live, records the answer and announces it.
//
// The poll loop is its only caller, which is what makes the recorded snapshot the last one taken at
// a known cadence rather than whatever the last caller happened to ask for.
// Everything else in this process reads lastRelayStatus instead: the control service answers
// GetRelayStatus from it, so several shells asking what is live do not multiply the requests the
// relay sees, and nothing can slip an extra fetch in between two polls and shorten the interval the
// byte deltas are divided by.
//
// The announcement is how the snapshot reaches the shells.
// They do not poll for it - the contract has it pushed at this side's interval,
// for the same delta reason (api/proto/screenshare/v1/events.proto).
func (a *App) fetchRelay() relay.Status {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	status := a.relay.Fetch(s.Relay.Host, s.Relay.ApiPort)
	a.relayLast.Store(&status)
	a.emit(wire.RelayStatusEvent(status))
	return status
}

// lastRelayStatus is the snapshot the last fetch produced, and an unreachable relay with no reason
// given while none has been taken.
//
// That starting value is the honest one rather than a placeholder: before anything has asked the
// relay, this app knows nothing about what is live, and "reachable" is the one thing it must not
// claim.
// It is also short-lived by construction - the poll starts with the process and its first fetch
// replaces this before a shell has finished its handshake - which is what keeps "nothing has asked
// yet" from being read on screen as "the relay is down".
func (a *App) lastRelayStatus() relay.Status {
	if last := a.relayLast.Load(); last != nil {
		return *last
	}
	return relay.Status{}
}

// WatchKey identifies one viewer window: a stream received over a specific transport.
// A stream can be watched over several transports at once (the relay re-serves each ingested stream
// on all its listeners), so the stream name alone is not a unique viewer identity.
type WatchKey struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
}

// Watching lists the viewers currently open, one entry per (stream, transport).
// Dead viewers are reaped here.
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

// watching reports whether a viewer is open for the pair, which a window that has since closed on
// its own is not.
//
// It reads the processes rather than anything a caller believed it had opened,
// which is what lets it stand in front of the validation StartWatch would otherwise run over a
// state that already holds.
func (a *App) watching(key WatchKey) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	watcher, present := a.watchers[key]
	return present && watcher.Running()
}

// carriesStream reports whether the named transport can deliver the live stream to the named
// receiving engine, and the reason it cannot.
//
// The relay re-serves an ingested stream on the listeners whose protocol has a payload mapping for
// its bitstream, and on no others: MPEG-TS carries H.264 and H.265, so an SRT viewer opened on a
// VP9 or AV1 stream connects and receives nothing.
// Left to the viewer that reads as a broken stream, so the combination is refused here with the
// format named.
//
// A stream the relay does not report, or one whose tracks name no format this app knows,
// is not refused: the snapshot can be older than the stream, and refusing on absent information
// would block a viewer that would have worked.
// Which is also why this reads the recorded snapshot rather than fetching one of its own - a
// snapshot at most one poll old is what the refusal is written against, and a fetch here would land
// between two polls and divide a byte delta by the wrong interval.
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

// StartWatch opens a viewer window for streamName, receiving it over transportName.
// The transport is chosen per viewer and is independent of the publish transport,
// so the same stream can be watched over any transport the relay serves it on.
// The watch package selects the viewer engine (ffplay by default, SCREENSHARE_VIEWER=mpv switches
// to mpv).
func (a *App) StartWatch(streamName, transportName string) error {
	assert.Assert(streamName != "", "a viewer names the stream it opens", transportName)
	assert.Assert(transportName != "", "a viewer names the leg it receives on", streamName)

	// Announced after the lock is released, which is what the order of these two defers buys:
	// emitViewerState reads the set through Watching and would deadlock against the mutex the rest of
	// this function holds.
	// It runs on the failed paths below too, where it announces the set unchanged - every event
	// carries a whole state, so a duplicate is harmless, and the alternative is an effect that
	// sometimes announces nothing.
	defer a.emitViewerState()

	key := WatchKey{Name: streamName, Transport: transportName}

	// Read before anything is validated, for the reason StartReceive reads its own state first.
	// The viewer this asks for is open, which is what was asked for.
	// Refusing here made the call unsafe to repeat, and a call that cannot be repeated leaves a shell
	// whose answer went missing with no way to find out what happened - and validating ahead of the
	// question puts that same refusal back under another name, since the relay's snapshot and the
	// settings both move under a viewer that is already running.
	if a.watching(key) {
		logger.Debugf("'%s' over %s is already being watched", streamName, transportName)
		return nil
	}

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

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
		// The same question again, under the lock this time: the read above is what keeps a repeat cheap
		// and this is what keeps two starts racing for one pair from opening two windows.
		return nil
	}

	exe, err := ffmpeg.FindExe(engine.Exe())
	if err != nil {
		return err
	}

	// hideWindow must be false: SW_HIDE would hide the viewer's video window too.
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
			// The exit says why this one stopped; the state event beside it says which viewers are left.
			// A viewer that closed on its own moves the set with nothing having been called,
			// so the state has to be announced here as well as at the two calls that change it.
			a.emit(wire.ViewerExitEvent(wire.WatchKey{StreamName: streamName, Transport: transportName}, message, logPath))
			a.emitViewerState()
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("watching '%s' over %s", streamName, transportName)
	a.watchers[key] = proc

	// Readiness is not signalled from here.
	// The viewer is "connected" once the relay reports a reader on the path, which the frontend
	// already sees in its Live() snapshot.
	// That signal is independent of the window system, unlike a probe for the ffplay window (no
	// portable form exists under Wayland).
	return nil
}

// OpenInBrowser opens the relay's player page for streamName, served on transportName,
// in the machine's default browser.
//
// It is the third way to watch and the only one this process does not own.
// A tab is the browser's, so nothing is supervised, nothing is added to the viewer set and there is
// no stop: the state a viewer event would carry is one this backend cannot read back,
// and reporting it would be reporting a guess.
//
// What it does share with StartWatch is the refusal.
// A leg the stream's format does not cross carries nothing to a browser either,
// and a page that connects and shows nothing is the failure the carriage check exists to turn into
// a sentence.
func (a *App) OpenInBrowser(streamName, transportName string) error {
	assert.Assert(streamName != "", "a browser page names the stream it opens", transportName)
	assert.Assert(transportName != "", "a browser page names the leg it opens over", streamName)

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

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
// It is the one conversion between the two structs, which hold the same pair under almost the same
// names and differ only in which package declares them: the app's is what the Wails binding
// generates a TypeScript type from, and wire's is what the contract is built out of.
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
// read would answer with rather than what a caller believed it had just done.
// It takes procMu, so a caller holding that lock defers this rather than calling it in place.
func (a *App) emitViewerState() {
	a.emit(wire.ViewerStateEvent(a.watchKeys()))
}
