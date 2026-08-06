package app

import (
	"fmt"
	"os/exec"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/netspeed"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// MeasureUplink probes the machine's real upload throughput (Mbit/s) so the UI
// can replace the guessed uplink figure with a measured one.
func (a *App) MeasureUplink() (float64, error) {
	return netspeed.MeasureUplink(a.ctx)
}

// MeasureEncodeRate times the configured encoder on generated frames of the
// captured monitor's size, so the UI can warn when the target frame rate is above
// what this machine encodes at these settings. The uplink probe's counterpart: one
// bounds what the line carries away, this one what the encoder produces.
//
// The picture size comes from the selected monitor rather than from the caller,
// since it is the same enumeration the capture backend crops to and the bitrate
// estimate is priced from.
func (a *App) MeasureEncodeRate(s settings.Stream) (encoderate.Rate, error) {
	width, height := 0, 0
	for _, m := range display.List() {
		if m.Index == s.Monitor {
			width, height = m.Width, m.Height
			break
		}
	}
	return encoderate.Measure(a.ctx, s, width, height)
}

// Monitors lists the display monitors (index, resolution, primary flag) so the
// capture-source UI can offer one entry per output and estimate the bitrate from
// the selected monitor's size.
func (a *App) Monitors() []display.Monitor {
	return display.List()
}

// Platform reports the OS and (on Linux) the display server, so the UI can
// disable capture backends that cannot run on this machine.
func (a *App) Platform() platform.Info {
	return platform.Detect()
}

// Capabilities returns the codec capability table, the single source both the
// encoder and the UI derive their codec/chroma/transport rules from.
func (a *App) Capabilities() []capabilities.Codec {
	return capabilities.Codecs
}

// Decoders returns the decode capability table, so the form can say what a publish
// choice costs the viewer: which GPUs decode the stream, and which decode the format but
// not the pixel format. It is not a fact about this machine, unlike Encoders: the viewer
// is someone else's hardware, and a stream is published once and watched on all of it.
func (a *App) Decoders() []capabilities.Decoder {
	return capabilities.Decoders
}

// Encoders reports which video codecs this machine can actually run, per publish
// engine, so the UI can grey out NVENC options on a machine without an NVIDIA GPU and
// a codec whose GStreamer plugin is missing on the portal capture backend alone. The
// probe runs once per codec and engine and is cached for the process lifetime.
func (a *App) Encoders() encoders.Availability {
	a.encodersOnce.Do(func() {
		a.encoders = encoders.Detect(a.ctx)
	})
	return a.encoders
}

// showSettings brings this window to the front on the settings form, for a
// request from outside it: the native grid's sidebar reaches the form this way
// rather than leaving the window to be found behind whatever covers it.
//
// Showing uncovers a hidden window and unminimising presents it, which is what
// raises and focuses one that is merely behind another. The "app:show-settings"
// event is the other half: a frontend showing the web grid over the form takes
// it off, so the raised window shows the settings and not what was on top of
// them.
func (a *App) showSettings() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.EventsEmit(a.ctx, "app:show-settings")
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
