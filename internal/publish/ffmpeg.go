package publish

import (
	"fmt"
	"strings"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// ffmpegEngine runs a screen grabber whose frames feed one ffmpeg process that
// captures, encodes, muxes and ships. It covers the ddagrab, gdigrab, x11grab
// and kmsgrab backends, which differ only in ffmpeg input arguments.
type ffmpegEngine struct{}

// buildArgs renders the command this engine runs, refusing first what the ffmpeg
// argument builder cannot refuse for itself.
//
// The builder is pure over the settings: it names no operating system, which is what
// lets a Windows pipeline be rendered and tested on a Linux machine. The second-track
// source is the one field whose validity depends on which platform the capture backend
// runs on, and that column is captureNeeds', in this package. So the check is made
// where both facts are reachable and the builder below stays a builder - the same split
// capabilities.Validate and transport.ValidatePublish already sit on.
//
// Both entry points render through here so the displayed command and the started run
// are refused alike, which is what publish.Command promises. preview is the one thing
// they differ by, for the reason the GStreamer engine's meter branch is: it carries a
// port the kernel handed out for this run, so a rendered command that showed it would
// be a different string every time it was rendered - and whether two settings build one
// pipeline is decided by comparing exactly that string (SamePipeline).
func buildArgs(s settings.Settings, preview PreviewLeg) ([]string, error) {
	if available, _ := AudioAvailable(s.Publish.Capture, s.Publish.Audio); !available {
		return nil, fmt.Errorf("the %s backend cannot record %s audio", s.Publish.Capture, s.Publish.Audio)
	}
	if !preview.Wanted() {
		return ffmpeg.BuildPublishArgs(s, nil)
	}
	// A format with no local carriage publishes without a preview rather than failing to
	// publish. The backend has already read the same table to decide whether to bring a
	// receiver up at all, so this branch is the one that survives the two disagreeing.
	tap, ok := ffmpegPreviewTap(s.Publish.Codec, preview)
	if !ok {
		return ffmpeg.BuildPublishArgs(s, nil)
	}
	return ffmpeg.BuildPublishArgs(s, &tap)
}

func (ffmpegEngine) Command(s settings.Settings) (string, error) {
	args, err := buildArgs(s, PreviewLeg{})
	if err != nil {
		return "", err
	}
	// Name the binary the run will actually spawn: kmsgrab uses the capability
	// wrapper (see ffmpeg.FindCaptureExe), everything else plain ffmpeg.
	exe := "ffmpeg"
	if s.Publish.Capture == "kmsgrab" {
		exe = "ffmpeg-kmsgrab"
	}
	return exe + " " + strings.Join(args, " "), nil
}

func (ffmpegEngine) Engine() string {
	return EngineFfmpeg
}

// Carries reports whether the transport can terminate an ffmpeg command.
func (ffmpegEngine) Carries(transportName string) bool {
	return transport.CanPublish(transportName, EngineFfmpeg)
}

func (ffmpegEngine) Start(s settings.Settings, tag string, preview PreviewLeg, cb Callbacks) (Handle, error) {
	args, err := buildArgs(s, preview)
	if err != nil {
		return nil, err
	}
	exe, err := ffmpeg.FindCaptureExe(s.Publish.Capture)
	if err != nil {
		return nil, err
	}
	proc, err := ffmpeg.Start(exe, args, true, false, tag, nil, cb.OnStats, nil, cb.OnExit)
	if err != nil {
		return nil, err
	}
	return proc, nil
}
