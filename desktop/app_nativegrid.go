package main

import (
	"fmt"
	"os"
	"slices"
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

// StartNativeGrid opens the native grid window: a separate GTK4 binary,
// separate because the webview process is GTK3 and the two toolkits cannot
// share a process. The window's sidebar offers every stream the relay reports
// live, received over the named transport, the watch leg for every tile in that
// window and unrelated to how each stream was published; picking streams happens
// in the window, not here. The -config argument carries the roster known at spawn,
// which can be empty, and pushRoster keeps it current over the child's stdin.
// A running window is replaced.
func (a *App) StartNativeGrid(transportName string) error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	status := a.relay.Fetch(s.RelayHost, s.ApiPort)
	if !status.Reachable {
		return fmt.Errorf("relay not reachable: %s", status.Error)
	}
	names := liveNames(status)

	cfg, err := watch.BuildGridConfig(s, names, transportName)
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

	// hideWindow must be false: SW_HIDE would hide the grid window too.
	proc, err := ffmpeg.Start(exe, []string{"-config", cfg}, false, true, "nativegrid", nil, nil,
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
	logger.Infof("native grid opened with %v over %s", names, transportName)
	a.nativeGrid = proc
	go a.pushRoster(proc, transportName, names)
	return nil
}

// liveNames returns the ready path names, sorted so two rosters compare by
// value.
func liveNames(status relay.Status) []string {
	var names []string
	for _, path := range status.Paths {
		if path.Ready {
			names = append(names, path.Name)
		}
	}
	slices.Sort(names)
	return names
}

// pushRoster polls the relay while the grid window runs and, whenever the set
// of live streams changes, pushes the full roster to the child as one JSON
// config per stdin line. The loop ends with the child: a dead process stops
// the poll, a failed write means the child is gone. An unreachable relay
// keeps the last pushed roster, so the window's rows stay explainable while
// the relay is down.
func (a *App) pushRoster(proc *ffmpeg.Proc, transportName string, last []string) {
	ticker := time.NewTicker(rosterPollInterval)
	defer ticker.Stop()

	for range ticker.C {
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
		names := liveNames(status)
		if slices.Equal(names, last) {
			continue
		}

		cfg, err := watch.BuildGridConfig(s, names, transportName)
		if err != nil {
			logger.Errorf("native grid roster push: %v", err)
			continue
		}
		_, err = fmt.Fprintln(proc.Stdin, cfg)
		if err != nil {
			return
		}
		last = names
		logger.Infof("native grid roster now %v", names)
	}
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
