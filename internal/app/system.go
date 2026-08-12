package app

import (
	"context"
	"fmt"
	"os/exec"
	goruntime "runtime"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/audiodev"
	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/netspeed"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The three long-running calls in this file take a context, and it is the caller's
// own: a shell's request context, so cancelling a probe when the shell that asked for
// it hung up is what a hang-up does.
//
// Each used to have a second, context-free form beside it, because the Wails binding
// generator had no model for a context.Context parameter and a bound method reached
// for the window's instead. There is no binding and no window here now, so the form
// that takes a context is the only one.

// measureUplink probes the machine's real upload throughput (Mbit/s) under ctx, so a
// shell can replace the guessed uplink figure with a measured one.
func (a *App) measureUplink(ctx context.Context) (float64, error) {
	return netspeed.MeasureUplink(ctx)
}

// measureEncodeRate times the configured encoder on generated frames of the captured
// monitor's size, so a shell can warn when the target frame rate is above what this
// machine encodes at these settings. The uplink probe's counterpart: one bounds what
// the line carries away, this one what the encoder produces.
//
// The picture size comes from the selected monitor rather than from the caller,
// since it is the same enumeration the capture backend crops to and the bitrate
// estimate is priced from.
func (a *App) measureEncodeRate(ctx context.Context, s settings.Settings) (encoderate.Rate, error) {
	width, height := 0, 0
	for _, m := range display.List() {
		if m.Index == s.Publish.Monitor {
			width, height = m.Width, m.Height
			break
		}
	}
	// A scaled run encodes the scaled picture, so that is the size the probe has to time:
	// measuring the captured one would report the capacity of a stream this configuration
	// never publishes, and report it low.
	if size, scaled, err := s.Publish.OutputSize(); err != nil {
		return encoderate.Rate{}, err
	} else if scaled {
		width, height = size.Width, size.Height
	}
	return encoderate.Measure(ctx, s, width, height)
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

// probeEncoders reports which video codecs this machine can actually run, per publish
// engine, so a form can grey out NVENC options on a machine without an NVIDIA GPU and
// a codec whose GStreamer plugin is missing on the portal capture backend alone.
//
// It runs the probe under ctx, or waits out the one already running, and answers with
// what it found. It is the call that costs seconds on a fresh process and nothing
// after that.
func (a *App) probeEncoders(ctx context.Context) encoders.Availability {
	a.encodersOnce.Do(func() {
		probed := encoders.Detect(ctx)
		a.encoders.Store(&probed)
	})

	available := a.encoders.Load()
	assert.IsNotNil(available, "a finished probe leaves an availability to read")
	return *available
}

// cachedEncoders is the probe result if one has been taken, and the zero value if
// none has.
//
// It never waits, and that is what it is for. A form is resolved on every keystroke
// and a probe takes seconds, so a resolve reads what is known now: an engine nothing
// has probed is an engine nothing is greyed on, which is a form that has not been
// told rather than a form claiming there is nothing usable (docs/ipc-api.md,
// "ResolveForm").
func (a *App) cachedEncoders() encoders.Availability {
	if available := a.encoders.Load(); available != nil {
		return *available
	}
	return encoders.Availability{}
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

// audioDevices is what this machine offers inside each audio kind.
//
// It is the enumeration's cached answer, taken on the first call and read from memory after
// that (internal/audiodev). Unlike the encoder probe there is no waiting form of it: the
// enumeration is one short subprocess rather than a run of every encoder, so the first
// resolve pays for it and every one after it does not.
func (a *App) audioDevices() []platform.AudioDevice {
	return audiodev.Cached(context.Background())
}
