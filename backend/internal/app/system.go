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
	"bjoernblessin.de/screenshare/internal/gpu"
	"bjoernblessin.de/screenshare/internal/netspeed"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/reach"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The long-running calls here take the caller's own context, a shell's request context,
// so a hang-up cancels the work that shell asked for.

// measureUplink probes this machine's real upload throughput in Mbit/s,
// so a shell can replace the guessed uplink figure with a measured one.
func (a *App) measureUplink(ctx context.Context) (float64, error) {
	return netspeed.MeasureUplink(ctx)
}

// checkRelay dials every leg of the relay the given settings name and answers what each listener said,
// so a shell can say which of them this machine reaches.
//
// Nothing of this app's is read: the addresses are the draft's and the answers the relay's,
// so a check of the settings on screen needs nothing that has been saved.
func (a *App) checkRelay(ctx context.Context, s settings.Settings) []reach.Result {
	return reach.Check(ctx, s)
}

// measureEncodeRate times the configured encoder on generated frames,
// so a shell can warn where the target frame rate is above what this machine encodes at these
// settings.
// The uplink probe's counterpart: one bounds what the line carries away, this one what the encoder
// produces.
//
// The picture size comes from the selected monitor rather than from the caller,
// the same enumeration the capture backend crops to and the bitrate estimate is priced from.
func (a *App) measureEncodeRate(ctx context.Context, s settings.Settings) (encoderate.Rate, error) {
	width, height := 0, 0
	for _, m := range display.List() {
		if m.Index == s.Publish.Monitor {
			width, height = m.Width, m.Height
			break
		}
	}
	// A scaled run encodes the scaled picture, so that is the size to time:
	// measuring the captured one would report the capacity of a stream this configuration never
	// publishes, and report it low.
	if size, scaled, err := s.Publish.OutputSize(); err != nil {
		return encoderate.Rate{}, err
	} else if scaled {
		width, height = size.Width, size.Height
	}
	return encoderate.Measure(ctx, s, width, height)
}

// Monitors is the outputs a shell offers as capture sources.
// The bounded read rather than the live one: both callers are shell reads,
// and the form behind them resolves on every keystroke (display.Recent).
func (a *App) Monitors() []display.Monitor {
	return display.Recent()
}

// Platform is what a capture backend's availability is decided against:
// the OS, and on Linux the display server.
func (a *App) Platform() platform.Info {
	return platform.Detect()
}

// Device is the video driver an encode here runs through,
// which the codec table's driver defects are matched against.
func (a *App) Device() capabilities.Device {
	return gpu.Device()
}

// Capabilities is the codec table both the encoder and the form derive their codec,
// chroma and transport rules from (docs/domain-model.md).
func (a *App) Capabilities() []capabilities.Codec {
	assert.Assert(len(capabilities.Codecs) > 0, "the codec table carries the rows every surface derives from")

	return capabilities.Codecs
}

// Decoders is the decode table, what a publish choice costs the viewer:
// which GPUs decode the stream, and which decode the format but not the pixel format.
// Unlike Encoders it states nothing about this machine:
// the viewer is someone else's hardware, and a stream is published once and watched on all of it.
func (a *App) Decoders() []capabilities.Decoder {
	assert.Assert(len(capabilities.Decoders) > 0, "the decode table carries the rows a publish choice is priced against")

	return capabilities.Decoders
}

// probeEncoders is which video codecs this machine can run, per publish engine:
// NVENC greys on a machine with no NVIDIA GPU,
// and a codec whose GStreamer plugin is missing greys on the portal capture backend alone.
//
// It runs the probe under ctx, or waits out the one already running.
// Seconds on a fresh process and nothing after that.
func (a *App) probeEncoders(ctx context.Context) encoders.Availability {
	a.encodersMu.Lock()
	defer a.encodersMu.Unlock()

	if available := a.encoders.Load(); available != nil {
		return *available
	}

	probed := encoders.Detect(ctx)

	// A cancelled sweep is thrown away rather than kept.
	// Every probe is a child process run under this context, so cancelling it fails all of them at once,
	// and a failed probe is recorded as a codec that does not run here,
	// rather than as one nothing asked about.
	// Stored, that verdict would be the answer for the life of the process:
	// the shell greys every codec on every engine with "probe failed",
	// and asking again, the documented recovery, would find the answer there and re-publish it.
	// The engine an install cannot reach is a different fact and travels in Unprobed,
	// which keeps this from throwing away a real answer about a machine.
	if ctx.Err() != nil {
		return probed
	}

	a.encoders.Store(&probed)
	return probed
}

// cachedEncoders is the probe result where one has been taken, and the zero value where none has.
//
// It never waits, and that is what it is for.
// A form is resolved on every keystroke and a probe takes seconds, so a resolve reads what is known:
// an engine nothing has probed is an engine nothing is greyed on,
// a form that has not been told rather than one claiming there is nothing usable
// (docs/ipc-api.md, "ResolveForm").
func (a *App) cachedEncoders() encoders.Availability {
	if available := a.encoders.Load(); available != nil {
		return *available
	}
	return encoders.Availability{}
}

// OpenLog hands one run log to the platform's default application.
//
// Not idempotent, the departure being the desktop's:
// nothing reports whether a window onto this path is open, so a second call opens a second one
// (docs/development-principles.md, "Effects across a process boundary").
func (a *App) OpenLog(path string) error {
	if path == "" {
		return fmt.Errorf("no log file for this run")
	}
	return openInShell(path)
}

// OpenLogsFolder hands the run-log directory to the file browser, on OpenLog's terms.
func (a *App) OpenLogsFolder() error {
	dir, err := ffmpeg.LogDir()
	if err != nil {
		return err
	}
	return openInShell(dir)
}

// openInShell opens a file or folder with the platform's default handler.
//
// The path is asserted rather than checked: both callers refuse an empty one above,
// so nothing reaching here is a caller that forgot,
// and handing a default handler no argument would open whatever it opens for one.
func openInShell(path string) error {
	assert.Assert(path != "", "a default handler is pointed at something to open")

	switch goruntime.GOOS {
	case "windows":
		// The empty argument is start's window title, which the path would otherwise be taken for.
		return exec.Command("cmd", "/c", "start", "", path).Run()
	case "darwin":
		return exec.Command("open", path).Run()
	default:
		return exec.Command("xdg-open", path).Run()
	}
}

// audioDevices is what this machine offers inside each audio kind.
//
// The enumeration's cached answer (internal/audiodev),
// taken on the first call and read from memory after that.
// Unlike the encoder probe there is no waiting form of it:
// the enumeration is one short subprocess rather than a run of every encoder,
// so the first resolve pays for it and none after it does.
func (a *App) audioDevices() []platform.AudioDevice {
	return audiodev.Devices(context.Background())
}

// portalCapabilities is what the desktop portal here serves: which pointer modes and source kinds
// its ScreenCast interface answers for.
//
// The compositor's answer rather than the interface's, so a mode one desktop serves and another
// refuses is greyed where it is refused instead of failing the publish (internal/portal).
// One D-Bus round trip on the first call, on audioDevices' terms.
func (a *App) portalCapabilities() portal.Capabilities {
	return portal.Cached(context.Background())
}
