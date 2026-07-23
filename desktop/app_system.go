package main

import (
	"fmt"
	"os/exec"
	goruntime "runtime"

	"bjoernblessin.de/screenshare/display"
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
