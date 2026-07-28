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

// requestBuffer holds the watch-leg changes the window sent while pushRoster
// was mid-poll. A burst is a person turning a knob, so the buffer only has to
// outlast one poll.
const requestBuffer = 8

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
func (a *App) StartNativeGrid(transportName string) error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	status := a.relay.Fetch(s.RelayHost, s.ApiPort)
	if !status.Reachable {
		return fmt.Errorf("relay not reachable: %s", status.Error)
	}
	live := liveStreams(status)

	cfg, err := watch.BuildGridConfig(s, live, transportName, nil)
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
	}

	// requests carries the window's watch-leg changes to the one goroutine that
	// owns them. done releases the reader once that goroutine is gone, so a
	// window still asking after the poll ended cannot wedge the pipe it reads.
	requests := make(chan watch.GridRequest, requestBuffer)
	done := make(chan struct{})

	// hideWindow must be false: SW_HIDE would hide the grid window too.
	proc, err := ffmpeg.Start(exe, []string{"-config", cfg}, false, true, "nativegrid", nil, nil,
		func(line string) {
			r, err := watch.ParseGridRequest(line)
			if err != nil {
				logger.Warnf("native grid: %v", err)
				return
			}
			select {
			case requests <- r:
			case <-done:
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
			runtime.EventsEmit(a.ctx, "nativegrid:exit", exitEvent{Message: message, LogPath: logPath})
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("native grid opened with %v over %s", streamNames(live), transportName)
	a.nativeGrid = proc
	go a.pushRoster(proc, transportName, live, requests, done)
	return nil
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

// pushRoster keeps one grid window's roster current: it polls the relay and
// pushes the full roster whenever the set of live streams changes, and it
// answers the watch-leg changes the window asks for. Both write one JSON config
// per stdin line, so the window has a single way in.
//
// The loop ends with the child: a dead process stops the poll, a failed write
// means the child is gone, and either releases the reader behind requests. An
// unreachable relay keeps the last pushed roster, so the window's rows stay
// explainable while the relay is down.
//
// The per-stream choices live here and nowhere else. This goroutine is the only
// one that touches them, so they need no lock, and they leave with the window
// that made them: they are deviations from the settings for one run, not
// settings of their own.
func (a *App) pushRoster(proc *ffmpeg.Proc, transportName string, live []watch.LiveStream, requests <-chan watch.GridRequest, done chan<- struct{}) {
	defer close(done)

	ticker := time.NewTicker(rosterPollInterval)
	defer ticker.Stop()

	choices := map[string]watch.WatchChoice{}
	for {
		select {
		case <-ticker.C:
			if !proc.Running() {
				return
			}
			a.settingsMu.Lock()
			s := a.settings
			a.settingsMu.Unlock()

			status := a.relay.Fetch(s.RelayHost, s.ApiPort)
			if !status.Reachable {
				continue
			}
			next := liveStreams(status)
			if slices.Equal(next, live) {
				continue
			}
			live = next
			if !a.pushGrid(proc, transportName, live, choices) {
				return
			}
			logger.Infof("native grid roster now %v", streamNames(live))

		case r := <-requests:
			a.applyGridRequest(r, live, transportName, choices)
			if !a.pushGrid(proc, transportName, live, choices) {
				return
			}
		}
	}
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

// pushGrid writes the roster the window should be showing. It reports whether
// the child is still there to write to, which is what ends the poll.
func (a *App) pushGrid(proc *ffmpeg.Proc, transportName string, live []watch.LiveStream, choices map[string]watch.WatchChoice) bool {
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

	cfg, err := watch.BuildGridConfig(s, live, transportName, choices)
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
