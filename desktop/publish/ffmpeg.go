package publish

import (
	"strings"

	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// ffmpegEngine runs a screen grabber whose frames feed one ffmpeg process that
// captures, encodes, muxes and ships. It covers the ddagrab, gdigrab, x11grab
// and kmsgrab backends, which differ only in ffmpeg input arguments.
type ffmpegEngine struct{}

func (ffmpegEngine) Command(s settings.Stream) (string, error) {
	args, err := ffmpeg.BuildPublishArgs(s)
	if err != nil {
		return "", err
	}
	// Name the binary the run will actually spawn: kmsgrab uses the capability
	// wrapper (see ffmpeg.FindCaptureExe), everything else plain ffmpeg.
	exe := "ffmpeg"
	if s.Capture == "kmsgrab" {
		exe = "ffmpeg-kmsgrab"
	}
	return exe + " " + strings.Join(args, " "), nil
}

func (ffmpegEngine) Engine() string {
	return EngineFfmpeg
}

// Carries reports whether the transport can terminate an ffmpeg command.
func (ffmpegEngine) Carries(transportName string) bool {
	return transport.CanFFmpegPublish(transportName)
}

func (ffmpegEngine) Start(s settings.Stream, tag string, cb Callbacks) (Handle, error) {
	args, err := ffmpeg.BuildPublishArgs(s)
	if err != nil {
		return nil, err
	}
	exe, err := ffmpeg.FindCaptureExe(s.Capture)
	if err != nil {
		return nil, err
	}
	proc, err := ffmpeg.Start(exe, args, true, false, tag, nil, cb.OnStats, cb.OnExit)
	if err != nil {
		return nil, err
	}
	return proc, nil
}
