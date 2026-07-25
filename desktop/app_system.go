package main

import (
	"fmt"
	"os/exec"
	goruntime "runtime"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/display"
	"bjoernblessin.de/screenshare/encoders"
	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/netspeed"
	"bjoernblessin.de/screenshare/platform"
)

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
// disable capture backends that cannot run on this machine.
func (a *App) Platform() platform.Info {
	return platform.Detect()
}

// Capabilities returns the codec capability table, the single source both the
// encoder and the UI derive their codec/chroma/transport rules from.
func (a *App) Capabilities() []capabilities.Codec {
	return capabilities.Codecs
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
