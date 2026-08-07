package app

import (
	"context"
	"fmt"
	"os/exec"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/encoders"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/netspeed"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The three long-running calls in this file each come in two forms, and the split is
// the same one every time: the exported method is what the Wails binding can carry,
// and the unexported one beside it takes the context.
//
// The binding generator has no model for a context.Context parameter, so a bound
// method cannot be handed one and reaches for the window's instead. A control shell
// has a real one - its request's - and cancelling a probe when the shell that asked
// for it hung up is the whole point of having it. Neither caller is wrong, so the
// work sits in the form that takes a context and the bound method passes the window's.

// MeasureUplink probes the machine's real upload throughput (Mbit/s) so the UI
// can replace the guessed uplink figure with a measured one.
func (a *App) MeasureUplink() (float64, error) {
	return a.measureUplink(a.ctx)
}

// measureUplink probes the upload throughput under ctx.
func (a *App) measureUplink(ctx context.Context) (float64, error) {
	return netspeed.MeasureUplink(ctx)
}

// MeasureEncodeRate times the configured encoder on generated frames of the
// captured monitor's size, so the UI can warn when the target frame rate is above
// what this machine encodes at these settings. The uplink probe's counterpart: one
// bounds what the line carries away, this one what the encoder produces.
func (a *App) MeasureEncodeRate(s settings.Stream) (encoderate.Rate, error) {
	return a.measureEncodeRate(a.ctx, s)
}

// measureEncodeRate times the encoder s configures, under ctx.
//
// The picture size comes from the selected monitor rather than from the caller,
// since it is the same enumeration the capture backend crops to and the bitrate
// estimate is priced from.
func (a *App) measureEncodeRate(ctx context.Context, s settings.Stream) (encoderate.Rate, error) {
	width, height := 0, 0
	for _, m := range display.List() {
		if m.Index == s.Monitor {
			width, height = m.Width, m.Height
			break
		}
	}
	// A scaled run encodes the scaled picture, so that is the size the probe has to time:
	// measuring the captured one would report the capacity of a stream this configuration
	// never publishes, and report it low.
	if size, scaled, err := s.OutputSize(); err != nil {
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

// Encoders reports which video codecs this machine can actually run, per publish
// engine, so the UI can grey out NVENC options on a machine without an NVIDIA GPU and
// a codec whose GStreamer plugin is missing on the portal capture backend alone. The
// probe runs once per codec and engine and is cached for the process lifetime.
func (a *App) Encoders() encoders.Availability {
	return a.probeEncoders(a.ctx)
}

// probeEncoders runs the probe under ctx, or waits out the one already running, and
// answers with what it found. It is the call that costs seconds on a fresh process
// and nothing after that.
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

// showSettings brings this window to the front on the settings form, for a
// request from outside it: the native grid's sidebar reaches the form this way
// rather than leaving the window to be found behind whatever covers it.
//
// Showing uncovers a hidden window and unminimising presents it, which is what
// raises and focuses one that is merely behind another. The "app:show-settings"
// event is the other half: a frontend showing the web grid over the form takes
// it off, so the raised window shows the settings and not what was on top of
// them.
//
// The window is this process's and is raised here; the event asks whichever surface
// receives it to bring its own configuration screen forward, which is why it goes to
// the control shells too rather than to the frontend alone.
func (a *App) showSettings() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	a.emit(wire.ShowSettingsEvent())
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
