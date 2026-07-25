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
	"strings"

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

	if err := capabilities.Validate("ffmpeg", s.Codec, s.Chroma, s.Transport, s.Mode, s.Cq, s.BitrateM); err != nil {
		return nil, err
	}

	src, err := captureArgs(s)
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
	assert.Assert(len(src.args) > 0 || len(src.filters) > 0, "a validated stream yields a capture source", s.Capture)
	assert.Assert(len(enc) > 0, "a validated stream yields an encoder", s.Codec)

	// A VAAPI encoder reads GPU surfaces, so its device opens ahead of the input and
	// the grabber's chain ends in a conversion and an upload (vaapi.go).
	vaapi := capabilities.IsVaapi(s.Codec)
	var device []string
	if vaapi {
		device, err = VaapiDevice()
		if err != nil {
			return nil, err
		}
		upload, err := VaapiFilters(s.Chroma)
		if err != nil {
			return nil, err
		}
		src.filters = append(src.filters, upload...)
	}

	gop := s.Gop
	if gop <= 0 {
		gop = s.Fps * 2 // auto: a keyframe every two seconds
	}

	args := []string{"-hide_banner"}
	args = append(args, device...)
	args = append(args, src.args...)
	args = append(args, audioIn...)
	if len(src.filters) > 0 {
		args = append(args, src.filterFlag, strings.Join(src.filters, ","))
	}
	args = append(args, enc...)
	// The upload filter has already pinned a VAAPI encode's layout, and the encoder's
	// own pixel format is the opaque hardware one, so -pix_fmt would ask ffmpeg to
	// convert GPU surfaces it cannot read.
	if !vaapi {
		args = append(args, "-pix_fmt", s.Chroma)
	}
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
		return nil, fmt.Errorf("transport %q has no ffmpeg publish form", s.Transport)
	}
	args = append(args, pub...)

	return args, nil
}

// captureSource is one screen grabber's contribution to the command: the input
// arguments, and the filter chain the frames reach the encoder through. The chain
// is kept apart from the arguments because the encoder may extend it (a VAAPI
// encode appends a conversion and an upload) and a chain can only be passed once.
type captureSource struct {
	// args are the input options and the -i itself, empty for a filter source.
	args []string
	// filters is the chain, one link per element, joined with commas when emitted.
	filters []string
	// filterFlag is the option the chain travels in, -vf by default and
	// -filter_complex for ddagrab, which is a source filter with no input to attach
	// a per-stream chain to.
	filterFlag string
}

// captureBackends is the input side of the command, one entry per screen grabber.
// ddagrab and gdigrab are Windows-only; x11grab and kmsgrab are Linux-only, which
// the capture list the UI offers already reflects (see publish.Captures). fps
// arrives rendered because every grabber takes it as a string.
var captureBackends = map[string]func(s settings.Stream, fps string) captureSource{
	"ddagrab": ddagrabArgs,
	"gdigrab": gdigrabArgs,
	"x11grab": x11grabArgs,
	"kmsgrab": kmsgrabCaptureArgs,
}

// captureArgs returns the input arguments and filter chain for the configured
// capture backend.
func captureArgs(s settings.Stream) (captureSource, error) {
	build, ok := captureBackends[s.Capture]
	if !ok {
		return captureSource{}, fmt.Errorf("unknown capture backend %q", s.Capture)
	}
	src := build(s, strconv.Itoa(s.Fps))
	if src.filterFlag == "" {
		src.filterFlag = "-vf"
	}
	return src, nil
}

// ddagrabArgs captures on the GPU as a filter source; hwdownload hands frames back
// to system memory so any encoder can consume them.
func ddagrabArgs(s settings.Stream, fps string) captureSource {
	return captureSource{
		filterFlag: "-filter_complex",
		filters: []string{
			fmt.Sprintf("ddagrab=output_idx=%d:framerate=%s", s.Monitor, fps),
			"hwdownload",
			"format=bgra",
		},
	}
}

func gdigrabArgs(_ settings.Stream, fps string) captureSource {
	return captureSource{args: []string{"-f", "gdigrab", "-framerate", fps, "-i", "desktop"}}
}

// x11grabArgs crops to the selected monitor when its geometry is known; otherwise
// it captures the whole X screen, as enumeration failing leaves no offset.
func x11grabArgs(s settings.Stream, fps string) captureSource {
	disp := os.Getenv("DISPLAY")
	if disp == "" {
		disp = ":0.0"
	}
	args := []string{"-f", "x11grab", "-framerate", fps}
	m, ok := monitorByIndex(s.Monitor)
	if !ok || m.Width <= 0 || m.Height <= 0 {
		return captureSource{args: append(args, "-i", disp)}
	}
	return captureSource{args: append(args,
		"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height),
		"-i", fmt.Sprintf("%s+%d,%d", disp, m.OffsetX, m.OffsetY),
	)}
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
