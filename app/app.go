package main

import (
	"context"
	"fmt"
	"os/exec"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/display"
	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/netspeed"
	"bjoernblessin.de/screenshare/platform"
	"bjoernblessin.de/screenshare/relay"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// exitEvent is the payload of the "publish:exit" event: the (possibly empty)
// error message and the path of the full run log.
type exitEvent struct {
	Message string `json:"message"`
	LogPath string `json:"logPath"`
}

// watchExitEvent is the payload of the "watch:exit" event. Name lets the UI
// clear the right stream's connecting state.
type watchExitEvent struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	LogPath string `json:"logPath"`
}

// App is the Wails-bound backend. All exported methods are callable from the
// frontend; events flow the other way via runtime.EventsEmit.
type App struct {
	ctx      context.Context
	mu       sync.Mutex
	settings settings.Stream
	relay    *relay.Client
	pub      *ffmpeg.Proc
	watchers map[string]*ffmpeg.Proc
}

func NewApp() *App {
	return &App{
		settings: settings.Load(),
		relay:    relay.New(),
		watchers: map[string]*ffmpeg.Proc{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown kills every child so no orphan ffmpeg keeps encoding after quit.
func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pub != nil {
		a.pub.Stop()
	}
	for _, watcher := range a.watchers {
		watcher.Stop()
	}
}

// --- settings ---

func (a *App) GetSettings() settings.Stream {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *App) SaveSettings(s settings.Stream) error {
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()

	return settings.Save(s)
}

// Transports lists the registered transports for the UI dropdown.
func (a *App) Transports() []string {
	return transport.Names()
}

// MeasureUplink probes the machine's real upload throughput (Mbit/s) so the UI
// can replace the guessed uplink figure with a measured one.
func (a *App) MeasureUplink() (float64, error) {
	return netspeed.MeasureUplink(a.ctx)
}

// Monitors lists the display monitors (index, resolution, primary flag) so the
// capture-source UI can offer one entry per output and estimate the bitrate from
// the selected monitor's size.
func (a *App) Monitors() []display.Monitor {
	return display.List()
}

// Platform reports the OS and (on Linux) the display server, so the UI can
// disable capture APIs that cannot run on this machine.
func (a *App) Platform() platform.Info {
	return platform.Detect()
}

// OpenLog opens a single run log in the OS default application.
func (a *App) OpenLog(path string) error {
	if path == "" {
		return fmt.Errorf("no log file for this run")
	}
	return openInShell(path)
}

// OpenLogsFolder opens the run-log directory in the OS file browser.
func (a *App) OpenLogsFolder() error {
	dir, err := ffmpeg.LogDir()
	if err != nil {
		return err
	}
	return openInShell(dir)
}

// openInShell opens a file or folder with the platform's default handler.
func openInShell(path string) error {
	switch goruntime.GOOS {
	case "windows":
		// The empty first argument is start's window-title parameter.
		return exec.Command("cmd", "/c", "start", "", path).Run()
	case "darwin":
		return exec.Command("open", path).Run()
	default:
		return exec.Command("xdg-open", path).Run()
	}
}

// --- publish ---

// PublishCommand returns the exact ffmpeg command line for the given settings
// without running it. Shown in the UI for transparency.
func (a *App) PublishCommand(s settings.Stream) (string, error) {
	args, err := ffmpeg.BuildPublishArgs(s)
	if err != nil {
		return "", err
	}

	return "ffmpeg " + strings.Join(args, " "), nil
}

// StartPublish validates s, persists it and starts the encoder child.
func (a *App) StartPublish(s settings.Stream) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pub != nil && a.pub.Running() {
		return fmt.Errorf("already publishing")
	}

	a.settings = s
	err := settings.Save(s)
	if err != nil {
		logger.Warnf("Cannot persist settings: %v", err)
	}

	args, err := ffmpeg.BuildPublishArgs(s)
	if err != nil {
		return err
	}

	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		return err
	}

	proc, err := ffmpeg.Start(exe, args, true, "publish", // hide ffmpeg's console window
		func(stats ffmpeg.Stats) {
			runtime.EventsEmit(a.ctx, "publish:stats", stats)
		},
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
			}
			runtime.EventsEmit(a.ctx, "publish:exit", exitEvent{Message: message, LogPath: logPath})
		})
	if err != nil {
		return err
	}

	logger.Infof("publishing '%s' via %s (%s, %s, %d fps)", s.Name, s.Transport, s.Mode, s.Chroma, s.Fps)
	a.pub = proc
	return nil
}

func (a *App) StopPublish() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pub != nil {
		a.pub.Stop()
		a.pub = nil
		logger.Infof("publishing stopped")
	}
}

func (a *App) Publishing() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pub != nil && a.pub.Running()
}

// --- discovery / watch ---

// Live returns the relay snapshot. The frontend polls this every 2 seconds;
// per-path bitrates are only meaningful with such a steady poll interval.
func (a *App) Live() relay.Status {
	a.mu.Lock()
	s := a.settings
	a.mu.Unlock()

	return a.relay.Fetch(s.RelayHost, s.ApiPort)
}

// Watching lists the streams currently viewed. Dead viewers are reaped here.
func (a *App) Watching() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	names := []string{}
	for name, watcher := range a.watchers {
		if watcher.Running() {
			names = append(names, name)
		} else {
			delete(a.watchers, name)
		}
	}
	return names
}

// StartWatch opens an ffplay window for streamName.
func (a *App) StartWatch(streamName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	watcher, present := a.watchers[streamName]
	if present && watcher.Running() {
		return fmt.Errorf("already watching %s", streamName)
	}

	args, err := ffmpeg.BuildWatchArgs(a.settings, streamName)
	if err != nil {
		return err
	}

	exe, err := ffmpeg.FindExe("ffplay")
	if err != nil {
		return err
	}

	// hideWindows must be false: SW_HIDE would hide ffplay's video window itself.
	proc, err := ffmpeg.Start(exe, args, false, "watch-"+streamName, nil,
		func(err error, stderrTail string, logPath string) {
			message := ""
			if err != nil {
				message = err.Error()
				if stderrTail != "" {
					message += "\n" + stderrTail
				}
			}
			runtime.EventsEmit(a.ctx, "watch:exit", watchExitEvent{
				Name: streamName, Message: message, LogPath: logPath,
			})
		})
	if err != nil {
		return err
	}

	assert.IsNotNil(proc, "Start returns a non-nil Proc when err is nil")
	logger.Infof("watching '%s'", streamName)
	a.watchers[streamName] = proc

	// The viewer window only appears once ffplay decoded the first frame
	// (SRT handshake + waiting for a keyframe, typically 1-3s). Poll for it
	// so the UI can show a connecting state until then.
	go a.emitWhenViewerReady(streamName, proc)

	return nil
}

// emitWhenViewerReady emits "watch:ready" once the ffplay window for
// streamName exists, or gives up when the viewer dies or 30s pass
// (watch:exit covers the death case in the UI).
func (a *App) emitWhenViewerReady(streamName string, proc *ffmpeg.Proc) {
	title := ffmpeg.WatchWindowTitle(streamName)

	for range 150 { // 150 * 200ms = 30s
		if !proc.Running() {
			return
		}
		if ffmpeg.WindowExists(title) {
			runtime.EventsEmit(a.ctx, "watch:ready", streamName)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	logger.Warnf("viewer window for '%s' did not appear within 30s", streamName)
}

func (a *App) StopWatch(streamName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	watcher, present := a.watchers[streamName]
	if present {
		watcher.Stop()
		delete(a.watchers, streamName)
		logger.Infof("stopped watching '%s'", streamName)
	}
}
