package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/relay"
	"bjoernblessin.de/screenshare/watch"
)

// EnvNativeGrid overrides where the native grid binary is found, for runs
// where it does not sit next to the app binary (wails dev runs from a temp
// dir; task dev sets this to the freshly built one).
const EnvNativeGrid = "SCREENSHARE_NATIVEGRID"

// NativeGridExe is the binary built from the nativegrid module, looked up next
// to the app binary first and then on PATH.
const NativeGridExe = "screenshare-nativegrid"

// rosterPollInterval paces pushRoster's relay polls, the same cadence as the
// frontend's Live() poll.
const rosterPollInterval = 2 * time.Second

// messageBuffer holds the watch-leg changes and commands the window sent while
// pushRoster was mid-poll. A burst is a person turning a knob, so the buffer only
// has to outlast one poll.
const messageBuffer = 8

// StartNativeGrid opens the native grid window: a separate GTK4 binary,
// separate because the webview process is GTK3 and the two toolkits cannot
// share a process. The window's sidebar offers every stream the relay reports
// live, received over the named transport, the watch leg every tile in that
// window starts on and unrelated to how each stream was published; picking
// streams happens in the window, not here. The -config argument carries the
// roster known at spawn, which can be empty, and pushRoster keeps it current
// over the child's stdin. A running window is replaced.
//
// The window can move a single stream to another watch leg, which it asks for
// on its stdout; pushRoster answers with the roster that choice produces. The
// choices belong to the window and are not written to the settings: they last
// as long as the process that made them.
//
// The same stdout carries the commands the window's sidebar sends, which act on
// this app rather than on a stream (runGridCommand), and what the window has a
// tile open on, which the app keeps for NativeGridWatching and the
// "nativegrid:watching" event. All three kinds are one message per line, told
// apart by the type each line names.
func (a *App) StartNativeGrid(transportName string) error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	status := a.relay.Fetch(s.RelayHost, s.ApiPort)
	if !status.Reachable {
		return fmt.Errorf("relay not reachable: %s", status.Error)
	}
	live := liveStreams(status)

	cfg, err := watch.BuildGridConfig(s, live, transportName, nil, watch.GridApp{Publishing: a.Publishing()})
	if err != nil {
		return err
	}

	exe := os.Getenv(EnvNativeGrid)
	if exe == "" {
		exe, err = ffmpeg.FindExe(NativeGridExe)
		if err != nil {
			return fmt.Errorf("%s not found: build it with 'task nativegrid' or set %s", NativeGridExe, EnvNativeGrid)
		}
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.nativeGrid != nil {
		a.nativeGrid.Stop()
		a.nativeGrid = nil
		a.setNativeGridWatchingLocked(nil)
	}

	// asks carries the window's watch-leg changes and commands to the one
	// goroutine that answers them, which is the one that pushes. done releases the
	// reader once that goroutine is gone, so a window still asking after the poll
	// ended cannot wedge the pipe it reads.
	asks := make(chan watch.GridMessage, messageBuffer)
	done := make(chan struct{})

	// self is the window the two callbacks below belong to,
	// which they need to tell their own reports from those of the window that replaced them.
	// It is filled after Start has already begun delivering lines,
	// so the assignment and every read of it happen under procMu.
	var self *ffmpeg.Proc

	// hideWindow must be false: SW_HIDE would hide the grid window too.
	proc, err := ffmpeg.Start(exe, []string{"-config", cfg}, false, true, "nativegrid", nil, nil,
		func(line string) {
			m, err := watch.ParseGridMessage(line)
			if err != nil {
				logger.Warnf("native grid: %v", err)
				return
			}
			switch m.Kind {
			// Both kinds are answered with a push, and the pusher owns the state
			// they change, so both cross to it rather than being run here.
			case watch.GridWatchLeg, watch.GridCommandKind:
				select {
				case asks <- m:
				case <-done:
				}
			case watch.GridWatchSet:
				a.setNativeGridWatching(&self, m.Status.Watching)
			default:
				logger.Debugf("native grid sent a %q message, which this app has no reader for", m.Kind)
			}
		},
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
				logger.Errorf("native grid failed: %v\n%s\nfull log: %s", err, stderrTail, logPath)
			} else {
				logger.Infof("native grid closed (log: %s)", logPath)
			}
			// A window that exited watches nothing.
			// Start reports the exit only once its stdout is drained, so this cannot undo a later report.
			a.setNativeGridWatching(&self, nil)
			runtime.EventsEmit(a.ctx, "nativegrid:exit", exitEvent{Message: message, LogPath: logPath})
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("native grid opened with %v over %s", streamNames(live), transportName)
	a.nativeGrid = proc
	self = proc
	go a.pushRoster(proc, transportName, live, asks, done)
	return nil
}

// watchingEvent is the payload of the "nativegrid:watching" event:
// the streams the native grid window has a tile open on, empty once no window is open.
type watchingEvent struct {
	Streams []string `json:"streams"`
}

// setNativeGridWatching records what one window reports it has a tile open on.
// A report from a window the app no longer holds is dropped:
// a replaced window can still deliver a line its reader had buffered,
// and its tiles are gone whatever the line says.
//
// from is the window the report came from,
// taken by pointer because StartNativeGrid fills it after the child has begun writing.
func (a *App) setNativeGridWatching(from **ffmpeg.Proc, watching []string) {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if *from == nil || *from != a.nativeGrid {
		return
	}
	a.setNativeGridWatchingLocked(watching)
}

// setNativeGridWatchingLocked replaces the watch set and tells the frontend when it changed.
// The event is emitted under the lock, so two windows cannot reach the frontend
// in the other order than they changed the set and leave it showing the one that is gone.
// The caller holds procMu.
func (a *App) setNativeGridWatchingLocked(watching []string) {
	if slices.Equal(a.nativeGridWatching, watching) {
		return
	}
	a.nativeGridWatching = watching
	logger.Infof("native grid watching %v", watching)
	runtime.EventsEmit(a.ctx, "nativegrid:watching", watchingEvent{Streams: gridWatching(watching)})
}

// gridWatching copies the watch set into the form the frontend receives it in,
// which is a list even where nothing is watched.
func gridWatching(watching []string) []string {
	out := make([]string, len(watching))
	copy(out, watching)
	return out
}

// liveStreams returns the ready paths as the grid's roster entries, sorted so
// two rosters compare by value. The bitstream format travels with the name
// because it decides which watch legs the window can offer that stream on.
func liveStreams(status relay.Status) []watch.LiveStream {
	var live []watch.LiveStream
	for _, path := range status.Paths {
		if path.Ready {
			live = append(live, watch.LiveStream{Name: path.Name, Format: path.Format})
		}
	}
	slices.SortFunc(live, func(a, b watch.LiveStream) int { return strings.Compare(a.Name, b.Name) })
	return live
}

// streamNames is the roster in the form a log line reads well as.
func streamNames(live []watch.LiveStream) []string {
	names := make([]string, 0, len(live))
	for _, l := range live {
		names = append(names, l.Name)
	}
	return names
}

// pushRoster keeps one grid window's state current: it polls the relay and
// pushes whenever the set of live streams or the publish state changes, and it
// answers the watch-leg changes and commands the window sends. Everything writes
// one JSON config per stdin line, so the window has a single way in.
//
// The loop ends with the child: a dead process stops the poll, a failed write
// means the child is gone, and either releases the reader behind asks. An
// unreachable relay keeps the last pushed roster, so the window's rows stay
// explainable while the relay is down.
//
// The per-stream choices live here and nowhere else. This goroutine is the only
// one that touches them, so they need no lock, and they leave with the window
// that made them: they are deviations from the settings for one run, not
// settings of their own. The reason a command was refused is kept the same way:
// it belongs to the window that asked, and the poll drops it as soon as the
// publish state moves, so a stale reason cannot outlast what it explains.
func (a *App) pushRoster(proc *ffmpeg.Proc, transportName string, live []watch.LiveStream, asks <-chan watch.GridMessage, done chan<- struct{}) {
	defer close(done)

	ticker := time.NewTicker(rosterPollInterval)
	defer ticker.Stop()

	choices := map[string]watch.WatchChoice{}
	app := watch.GridApp{Publishing: a.Publishing()}
	for {
		select {
		case <-ticker.C:
			if !proc.Running() {
				return
			}
			a.settingsMu.Lock()
			s := a.settings
			a.settingsMu.Unlock()

			// The publish state is the app's own, so it is compared before the
			// relay is asked anything and pushed whether or not the relay
			// answers. A window whose publish button waited for a reachable
			// relay would show the state of neither.
			changed := false
			if publishing := a.Publishing(); publishing != app.Publishing {
				app = watch.GridApp{Publishing: publishing}
				changed = true
			}
			if status := a.relay.Fetch(s.RelayHost, s.ApiPort); status.Reachable {
				if next := liveStreams(status); !slices.Equal(next, live) {
					live = next
					changed = true
					logger.Infof("native grid roster now %v", streamNames(live))
				}
			}
			if !changed {
				continue
			}
			if !a.pushGrid(proc, transportName, live, choices, app) {
				return
			}

		case m := <-asks:
			// The stdout reader forwards these two kinds and answers the rest itself.
			switch m.Kind {
			case watch.GridWatchLeg:
				a.applyGridRequest(m.Request, live, transportName, choices)
			case watch.GridCommandKind:
				app = a.runGridCommand(m.Command)
			default:
				assert.Never("unexpected queued grid message kind", m.Kind)
			}
			if !a.pushGrid(proc, transportName, live, choices, app) {
				return
			}
		}
	}
}

// runGridCommand runs one command the window sent and returns the app state its
// answer carries. A command that failed is answered with the reason rather than
// with the state that did not change: the window has no other way to learn why
// the button it pressed did nothing, and the app's own window may be behind it.
//
// A command this build does not know is reported and skipped. The two halves
// ship together, so an unknown one means a grid binary from another build.
func (a *App) runGridCommand(c watch.GridCommand) watch.GridApp {
	var err error
	switch c.Name {
	case watch.GridShowSettings:
		a.showSettings()
	case watch.GridStartPublish:
		err = a.startPublishHeld()
	case watch.GridStopPublish:
		a.StopPublish()
	default:
		logger.Warnf("native grid sent the command %q, which this app cannot run", c.Name)
	}

	app := watch.GridApp{Publishing: a.Publishing()}
	if err != nil {
		logger.Warnf("native grid command %q failed: %v", c.Name, err)
		app.PublishError = err.Error()
	}
	return app
}

// applyGridRequest takes one watch-leg change from the window, or refuses it
// and leaves the stream on the leg it had. Either way the caller pushes the
// roster afterwards: the push carries the leg and the knob values that hold
// now, so a window whose controls show a refused change is corrected by the
// answer to it.
func (a *App) applyGridRequest(r watch.GridRequest, live []watch.LiveStream, transportName string, choices map[string]watch.WatchChoice) {
	i := slices.IndexFunc(live, func(l watch.LiveStream) bool { return l.Name == r.Stream })
	if i < 0 {
		logger.Warnf("native grid asked for %q, which the relay does not report live", r.Stream)
		return
	}

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	if _, _, err := watch.WatchLeg(s, live[i], transportName, r.Choice()); err != nil {
		logger.Warnf("native grid watch leg refused: %v", err)
		return
	}
	choices[r.Stream] = r.Choice()
	logger.Infof("native grid watches %q over %s", r.Stream, r.Transport)
}

// pushGrid writes the state the window should be showing. It reports whether
// the child is still there to write to, which is what ends the poll.
func (a *App) pushGrid(proc *ffmpeg.Proc, transportName string, live []watch.LiveStream, choices map[string]watch.WatchChoice, app watch.GridApp) bool {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	// A choice can outlive what made it possible: a stream that comes back in
	// another format leaves one its transport no longer carries. Dropping it
	// puts that stream back on the window's leg and keeps the push, which the
	// whole roster would otherwise be refused over.
	for _, err := range watch.PruneWatchChoices(s, live, transportName, choices) {
		logger.Warnf("native grid watch leg dropped: %v", err)
	}

	cfg, err := watch.BuildGridConfig(s, live, transportName, choices, app)
	if err != nil {
		logger.Warnf("native grid roster push: %v", err)
		return true
	}
	if _, err := fmt.Fprintln(proc.Stdin, cfg); err != nil {
		return false
	}
	return true
}

// StopNativeGrid closes the native grid window.
func (a *App) StopNativeGrid() {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	if a.nativeGrid != nil {
		a.nativeGrid.Stop()
		a.nativeGrid = nil
		a.setNativeGridWatchingLocked(nil)
		logger.Infof("stopped the native grid")
	}
}

// NativeGridRunning reports whether the native grid window is open. The
// frontend polls it, which is also how a window closed via its own close
// button is noticed.
func (a *App) NativeGridRunning() bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	return a.nativeGrid != nil && a.nativeGrid.Running()
}

// NativeGridWatching lists the streams the native grid window has a tile open on,
// as the window last reported them.
// It is empty while no window is open, and while one is opening and has yet to report.
// The frontend polls it beside NativeGridRunning,
// and the "nativegrid:watching" event carries the same list as it changes.
func (a *App) NativeGridWatching() []string {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	return gridWatching(a.nativeGridWatching)
}
