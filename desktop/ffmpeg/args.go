// Package ffmpeg builds the ffmpeg publish command line, locates the media
// executables, and supervises the child processes.
//
// The argument builder mirrors the prototype script in scripts/publish.ps1;
// the values there were tuned against a real relay and are the reference for
// what it must produce. The destination URL and muxer come from the transport
// registry, so the encoder args stay independent of how bytes leave the
// machine. The viewer command lines live in the watch package.
package ffmpeg

import (
	"fmt"
	"os"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/display"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// BuildPublishArgs returns the ffmpeg arguments (without the executable) that
// capture the selected monitor and push the encoded stream to the relay.
//
// The order matches scripts/publish.ps1: capture input, encoder, pixel format
// and color range, GOP, then the transport's muxer and destination URL.
func BuildPublishArgs(s settings.Stream) ([]string, error) {
	if _, ok := transport.Get(s.Transport); !ok {
		return nil, fmt.Errorf("unknown transport %q", s.Transport)
	}

	if err := capabilities.Validate(s.Codec, s.Chroma, s.Transport, s.Mode, s.Cq); err != nil {
		return nil, err
	}

	capture, err := captureArgs(s)
	if err != nil {
		return nil, err
	}

	audioIn, err := audioInputArgs(s)
	if err != nil {
		return nil, err
	}

	enc, err := encoderArgs(s)
	if err != nil {
		return nil, err
	}
	assert.Assert(len(capture) > 0 && len(enc) > 0, "a validated stream yields a capture input and an encoder", s.Capture, s.Codec)

	gop := s.Gop
	if gop <= 0 {
		gop = s.Fps * 2 // auto: a keyframe every two seconds
	}

	args := []string{"-hide_banner"}
	args = append(args, capture...)
	args = append(args, audioIn...)
	args = append(args, enc...)
	args = append(args, "-pix_fmt", s.Chroma)
	// color_range only applies to YUV formats; gbrp is inherently full range.
	if s.Chroma != "gbrp" {
		args = append(args, "-color_range", s.ColorRange)
	}
	args = append(args, "-g", strconv.Itoa(gop))
	if len(audioIn) > 0 {
		args = append(args, audioEncodeArgs()...)
	}

	pub, ok := transport.PublishArgs(s)
	if !ok {
		return nil, fmt.Errorf("transport %q cannot publish through ffmpeg", s.Transport)
	}
	args = append(args, pub...)

	return args, nil
}

// captureBackends is the input side of the command, one entry per screen grabber.
// ddagrab and gdigrab are Windows-only; x11grab and kmsgrab are Linux-only, which
// the capture list the UI offers already reflects (see publish.Captures). fps
// arrives rendered because every grabber takes it as a string.
var captureBackends = map[string]func(s settings.Stream, fps string) []string{
	"ddagrab": ddagrabArgs,
	"gdigrab": gdigrabArgs,
	"x11grab": x11grabArgs,
	"kmsgrab": kmsgrabCaptureArgs,
}

// captureArgs returns the input arguments for the configured capture backend.
func captureArgs(s settings.Stream) ([]string, error) {
	build, ok := captureBackends[s.Capture]
	if !ok {
		return nil, fmt.Errorf("unknown capture backend %q", s.Capture)
	}
	return build(s, strconv.Itoa(s.Fps)), nil
}

// ddagrabArgs captures on the GPU as a filter source; hwdownload hands frames back
// to system memory so any encoder can consume them.
func ddagrabArgs(s settings.Stream, fps string) []string {
	return []string{
		"-filter_complex",
		fmt.Sprintf("ddagrab=output_idx=%d:framerate=%s,hwdownload,format=bgra", s.Monitor, fps),
	}
}

func gdigrabArgs(_ settings.Stream, fps string) []string {
	return []string{"-f", "gdigrab", "-framerate", fps, "-i", "desktop"}
}

// x11grabArgs crops to the selected monitor when its geometry is known; otherwise
// it captures the whole X screen, as enumeration failing leaves no offset.
func x11grabArgs(s settings.Stream, fps string) []string {
	disp := os.Getenv("DISPLAY")
	if disp == "" {
		disp = ":0.0"
	}
	args := []string{"-f", "x11grab", "-framerate", fps}
	m, ok := monitorByIndex(s.Monitor)
	if !ok || m.Width <= 0 || m.Height <= 0 {
		return append(args, "-i", disp)
	}
	return append(args,
		"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height),
		"-i", fmt.Sprintf("%s+%d,%d", disp, m.OffsetX, m.OffsetY),
	)
}

// pulseMonitorDevice is the libpulse magic name for the monitor of the default
// sink: the mixed desktop audio. PipeWire's pulse server implements it as well,
// so the same device string works on both sound servers.
const pulseMonitorDevice = "@DEFAULT_MONITOR@"

// audioInputArgs returns the audio capture input for the selected audio source.
// Desktop audio comes from the PulseAudio/PipeWire monitor source, which the
// Windows capture backends have no counterpart for: ffmpeg lacks WASAPI
// loopback capture, so those backends reject the option instead of publishing
// a silent track.
func audioInputArgs(s settings.Stream) ([]string, error) {
	switch s.Audio {
	case "", "none":
		return nil, nil
	case "desktop":
		switch s.Capture {
		case "ddagrab", "gdigrab":
			return nil, fmt.Errorf("desktop audio capture needs PulseAudio/PipeWire, which the %s (Windows) backend cannot reach", s.Capture)
		}
		return []string{"-f", "pulse", "-i", pulseMonitorDevice}, nil
	default:
		return nil, fmt.Errorf("unknown audio source %q", s.Audio)
	}
}

// audioEncodeArgs encodes the captured audio as 128 kbit/s stereo Opus. Opus is
// the one codec every hop already handles: ffmpeg muxes it into MPEG-TS,
// MediaMTX forwards it over SRT and WebRTC, and ffplay/mpv/browsers decode it.
func audioEncodeArgs() []string {
	return []string{"-c:a", "libopus", "-b:a", "128k", "-ac", "2"}
}

// monitorByIndex looks up an enumerated monitor by its capture index. The second
// return is false when no monitor carries that index, which happens when
// enumeration is unavailable on this platform or the saved index is stale.
func monitorByIndex(idx int) (display.Monitor, bool) {
	for _, m := range display.List() {
		if m.Index == idx {
			return m, true
		}
	}
	return display.Monitor{}, false
}
