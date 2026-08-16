package publish

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// ffmpegEngine runs one ffmpeg process that captures, encodes, muxes and ships.
// Its backends differ only in ffmpeg input arguments.
type ffmpegEngine struct{}

// buildArgs renders the command this engine runs, refusing first what the ffmpeg argument builder
// cannot refuse for itself.
//
// The builder is pure over the settings and names no operating system, which is what lets a Windows
// pipeline be rendered and tested on a Linux machine.
// The second-track source is the one field whose validity depends on which platform the capture
// backend runs on, and that column is captureNeeds', in this package, so the check is made here and
// the builder stays a builder.
// The same split capabilities.Validate and transport.ValidatePublish sit on.
//
// Both entry points render through here, so the displayed command and the started run are refused
// alike, which is what publish.Command promises.
// preview is the one thing they differ by, for the reason the GStreamer engine's meter branch is:
// it carries a port the kernel handed out for this run,
// so a command rendered with it would differ on every render,
// and whether two settings build one pipeline is decided by comparing exactly that string
// (SamePipeline).
// meterPort is empty for a command nothing weighs, and carries a port the kernel handed out for this
// run otherwise, which is why it is no part of what Command renders.
func buildArgs(s settings.Settings, preview PreviewLeg, meterPort string) ([]string, error) {
	for _, a := range s.Publish.Recorded() {
		if available, _ := AudioAvailable(s.Publish.Capture, a.Source); !available {
			return nil, fmt.Errorf("the %s backend cannot record %s audio", s.Publish.Capture, a.Source)
		}
	}

	var taps []ffmpeg.Tap
	if preview.Wanted() {
		// A format with no local carriage publishes without a preview rather than failing to publish.
		// The backend read the same table to decide whether to bring a receiver up at all, so this branch
		// is what survives the two disagreeing.
		if tap, ok := ffmpegPreviewTap(s.Publish.Codec(), preview); ok {
			taps = append(taps, tap)
		}
	}
	if meterPort != "" {
		taps = append(taps, ffmpegMeterTap(meterPort))
	}
	return ffmpeg.BuildPublishArgs(s, taps)
}

func (ffmpegEngine) Command(s settings.Settings) (string, error) {
	args, err := buildArgs(s, PreviewLeg{}, "")
	if err != nil {
		return "", err
	}
	// The binary the run spawns: the capability wrapper under kmsgrab, plain ffmpeg otherwise
	// (ffmpeg.FindCaptureExe).
	exe := "ffmpeg"
	if s.Publish.Capture == "kmsgrab" {
		exe = "ffmpeg-kmsgrab"
	}
	return exe + " " + strings.Join(args, " "), nil
}

func (ffmpegEngine) Engine() string {
	return EngineFfmpeg
}

// Carries reports whether the transport terminates an ffmpeg command.
func (ffmpegEngine) Carries(transportName string) bool {
	return transport.CanPublish(transportName, EngineFfmpeg)
}

func (ffmpegEngine) Start(s settings.Settings, tag string, preview PreviewLeg, cb Callbacks) (Handle, error) {
	// A meter this side could not open is a run without a bitrate figure rather than a run refused:
	// what the reader asked for is a stream.
	meter, meterErr := newFfmpegMeter()
	meterPort := ""
	if meterErr != nil {
		logger.Warnf("publishing without a bitrate meter: %v", meterErr)
	} else {
		meterPort = meter.port()
	}

	args, err := buildArgs(s, preview, meterPort)
	if err != nil {
		if meter != nil {
			meter.close()
		}
		return nil, err
	}
	exe, err := ffmpeg.FindCaptureExe(s.Publish.Capture)
	if err != nil {
		if meter != nil {
			meter.close()
		}
		return nil, err
	}

	onStats := cb.OnStats
	if meter != nil && onStats != nil {
		onStats = func(sample ffmpeg.Stats) { cb.OnStats(meter.fill(sample)) }
	}
	onExit := cb.OnExit
	if meter != nil {
		onExit = func(err error, stderrTail, logPath string) {
			meter.close()
			if cb.OnExit != nil {
				cb.OnExit(err, stderrTail, logPath)
			}
		}
	}

	proc, err := ffmpeg.Start(exe, args, true, false, tag, nil, onStats, nil, onExit,
		ffmpeg.WithRedactor(func(text string) string { return transport.Redact(s, text) }))
	if err != nil {
		if meter != nil {
			meter.close()
		}
		return nil, err
	}
	return proc, nil
}
