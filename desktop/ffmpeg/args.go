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
	"bjoernblessin.de/screenshare/gpupath"
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

	if err := capabilities.Validate(capabilities.EngineFfmpeg, s.Codec, s.CapabilityOptions(), s.Cq, s.BitrateM); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublish(s.Transport, capabilities.EngineFfmpeg, s.Codec); err != nil {
		return nil, err
	}
	if err := capabilities.ValidateAudio(capabilities.EngineFfmpeg, s.AudioTrack()); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublishAudio(s.Transport, capabilities.EngineFfmpeg, s.AudioTrack()); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublishSettings(s); err != nil {
		return nil, err
	}
	// Every grabber takes the rate as an input option and the keyframe interval
	// follows from it (gopFor), so a non-positive rate would reach the command as
	// "-framerate 0" and "-g 0". The GStreamer engine refuses the same value.
	if s.Fps <= 0 {
		return nil, fmt.Errorf("the ffmpeg publish engine needs a positive fps, got %d", s.Fps)
	}

	memory, err := frameMemory(s)
	if err != nil {
		return nil, err
	}

	src, err := captureArgs(s, memory)
	if err != nil {
		return nil, err
	}

	audioIn, err := audioInputArgs(s)
	if err != nil {
		return nil, err
	}

	enc, err := encoderArgs(s, gopFor(s))
	if err != nil {
		return nil, err
	}
	assert.Assert(len(src.args) > 0 || len(src.filters) > 0, "a validated stream yields a capture source", s.Capture)
	assert.Assert(len(enc) > 0, "a validated stream yields an encoder", s.Codec)

	// On the GPU path the frames never become software ones, so the map and the
	// device-side conversion replace the colour tag, the upload and the device option
	// in one chain: the conversion states the colour it wrote, and the encoder takes
	// its device from the frames it is handed (gpu.go).
	var device []string
	surface := memory == gpupath.MemoryGpu
	if surface {
		gpu, err := GpuFilters(s.Codec, s.Chroma, s.ColorRange)
		if err != nil {
			return nil, err
		}
		src.filters = append(src.filters, gpu...)
	} else {
		// The colour description rides on the frames, so it is tagged while they are
		// still software ones, ahead of the upload a surface encode ends its chain in.
		if colour := colourFilter(s); colour != "" {
			src.filters = append(src.filters, colour)
		}

		// A VAAPI, QSV or Vulkan encoder reads GPU surfaces, so its device opens ahead
		// of the input and the grabber's chain ends in a conversion and an upload
		// (hwsurface.go).
		var uploads bool
		device, uploads, err = HwSurfaceDevice(s.Codec)
		if err != nil {
			return nil, err
		}
		if uploads {
			upload, err := HwSurfaceFilters(s.Codec, s.Chroma)
			if err != nil {
				return nil, err
			}
			src.filters = append(src.filters, upload...)
		}
		surface = uploads
	}

	args := []string{"-hide_banner"}
	args = append(args, device...)
	args = append(args, src.args...)
	args = append(args, audioIn...)
	if len(src.filters) > 0 {
		args = append(args, src.filterFlag, strings.Join(src.filters, ","))
	}
	args = append(args, enc...)
	// The upload filter has already pinned a surface encode's layout, and the encoder's
	// own pixel format is the opaque hardware one, so -pix_fmt would ask ffmpeg to
	// convert GPU surfaces it cannot read.
	if !surface {
		args = append(args, "-pix_fmt", s.Chroma)
	}
	// color_range only applies to YUV formats; gbrp is inherently full range.
	if s.Chroma != "gbrp" {
		args = append(args, "-color_range", s.ColorRange)
	}
	args = append(args, "-g", strconv.Itoa(gopFor(s)))
	if len(audioIn) > 0 {
		enc, err := audioEncodeArgs(s)
		if err != nil {
			return nil, err
		}
		args = append(args, enc...)
	}

	pub, ok := transport.PublishArgs(s)
	if !ok {
		return nil, fmt.Errorf("transport %q has no ffmpeg publish form", s.Transport)
	}
	args = append(args, pub...)

	return args, nil
}

// colourDescription is the colour space this engine encodes against, spelled as
// setparams and ffprobe name it. It is the ffmpeg side of what the GStreamer engine
// pins as gstBt709: BT.709 is the colour space of every HD and larger picture, which
// is every screen this app captures, so a stream's colour does not follow from which
// engine published it.
const colourDescription = "bt709"

// colourFilter tags the captured frames with the colour space they are encoded
// against, empty for a chroma with none to state.
//
// The tag is what puts the colour description in the bitstream, and the bitstream is
// the only place a viewer reads it from, since RTP and MPEG-TS carry no colour of
// their own. A component left unsignalled is one the viewer picks off the picture
// size, and it picks limited-range BT.709, so an unsignalled full-range stream is
// expanded as if it were limited: crushed blacks, clipped whites.
//
// The output options reach only part of it. -colorspace lands in the bitstream where
// -color_primaries and -color_trc do not, and a partial description is no
// description, a GStreamer viewer reporting no colorimetry at all for one.
//
// The range is deliberately absent from the tag. Left unspecified on the frames, the
// conversion to the encoder's pixel format takes its target range from -color_range.
// Stated here as well, that conversion writes limited range whatever -color_range
// says, and full-range white reaches the encoder as Y=235 under a bitstream claiming
// 255.
//
// Planar RGB is skipped for the reason it carries no -color_range: it has no matrix
// and is full range by construction.
func colourFilter(s settings.Stream) string {
	if s.Chroma == "gbrp" {
		return ""
	}
	return fmt.Sprintf("setparams=colorspace=%s:color_primaries=%s:color_trc=%s",
		colourDescription, colourDescription, colourDescription)
}

// gopFor returns the keyframe interval in frames. A settings value of zero is the
// form's automatic setting, a keyframe every two seconds. The encoder builder reads
// it as well, since one encoder aligns its parameter-set repeat to the GOP, so both
// halves of the command have to agree on the interval.
func gopFor(s settings.Stream) int {
	if s.Gop > 0 {
		return s.Gop
	}
	return s.Fps * 2
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
// ddagrab and gdigrab are Windows-only, x11grab and kmsgrab Linux-only, and
// avfoundation macOS-only, which the capture list the UI offers already reflects
// (see publish.Captures). fps arrives rendered because every grabber takes it as a
// string.
//
// A backend returns an error where the settings name something it cannot capture: a
// monitor this machine does not have, a DRM download strategy no table row names.
// The alternative is a command that captures something else, which is the one
// outcome the form has no way to show.
var captureBackends = map[string]func(s settings.Stream, fps, memory string) (captureSource, error){
	"ddagrab":      ddagrabArgs,
	"gdigrab":      gdigrabArgs,
	"x11grab":      x11grabArgs,
	"kmsgrab":      kmsgrabCaptureArgs,
	"avfoundation": avfoundationArgs,
}

// captureArgs returns the input arguments and filter chain for the configured
// capture backend, in the memory this run's frames reach the encoder in. A backend
// with no GPU path is handed the resolved value all the same and ignores it, so the
// one place the two shapes are chosen between stays the pair table.
func captureArgs(s settings.Stream, memory string) (captureSource, error) {
	build, ok := captureBackends[s.Capture]
	if !ok {
		return captureSource{}, fmt.Errorf("unknown capture backend %q", s.Capture)
	}
	src, err := build(s, strconv.Itoa(s.Fps), memory)
	if err != nil {
		return captureSource{}, err
	}
	if src.filterFlag == "" {
		src.filterFlag = "-vf"
	}
	return src, nil
}

// frameMemory resolves where this run's frames reach the encoder, against the pair
// table both publish engines read.
//
// No device check follows it here. Both of this engine's GPU paths map the captured
// frames onto a device derived from the frames themselves, so the encoder runs on the
// GPU the capture came off whatever else the machine carries.
func frameMemory(s settings.Stream) (string, error) {
	c, ok := capabilities.Get(s.Codec)
	if !ok {
		return "", fmt.Errorf("unknown codec %q", s.Codec)
	}
	return gpupath.Resolve(capabilities.EngineFfmpeg, s.Capture, c.Family, s.CaptureMemory)
}

// ddagrabArgs captures on the GPU as a filter source.
//
// On the system-memory path hwdownload hands the frames back so any encoder can read
// them, and format=bgra pins the layout swscale converts from. On the GPU path the
// texture stays where Desktop Duplication put it and the map that follows derives the
// encoder's device from it, so the chain here ends at the grabber.
func ddagrabArgs(s settings.Stream, fps, memory string) (captureSource, error) {
	filters := []string{fmt.Sprintf("ddagrab=output_idx=%d:framerate=%s", s.Monitor, fps)}
	if memory != gpupath.MemoryGpu {
		filters = append(filters, "hwdownload", "format=bgra")
	}
	return captureSource{filterFlag: "-filter_complex", filters: filters}, nil
}

func gdigrabArgs(_ settings.Stream, fps, _ string) (captureSource, error) {
	return captureSource{args: []string{"-f", "gdigrab", "-framerate", fps, "-i", "desktop"}}, nil
}

// x11grabArgs crops the X screen to the selected monitor's geometry.
//
// A monitor index no enumerated output carries is refused: it names a screen this
// machine does not have, and capturing the whole desktop instead would publish
// something other than what the form shows selected. The one index with no geometry
// is display.List's placeholder, which stands for enumeration being unavailable
// here, and the whole X screen is what the single entry it offers means.
//
// DISPLAY is likewise refused when unset rather than guessed at: x11grab reads an X
// screen, and no environment naming one is no X session to capture.
func x11grabArgs(s settings.Stream, fps, _ string) (captureSource, error) {
	disp := os.Getenv("DISPLAY")
	if disp == "" {
		return captureSource{}, fmt.Errorf("x11grab captures an X screen and DISPLAY names none")
	}
	args := []string{"-f", "x11grab", "-framerate", fps}
	m, ok := monitorByIndex(s.Monitor)
	if !ok {
		return captureSource{}, fmt.Errorf("monitor %d is not one of this machine's outputs", s.Monitor)
	}
	if m.Width <= 0 || m.Height <= 0 {
		return captureSource{args: append(args, "-i", disp)}, nil
	}
	return captureSource{args: append(args,
		"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height),
		"-i", fmt.Sprintf("%s+%d,%d", disp, m.OffsetX, m.OffsetY),
	)}, nil
}

// avfoundationScreenDevice is the name AVFoundation lists a screen under, with the
// monitor index appended. ffmpeg takes one video and one audio device in a single
// -i, separated by a colon, so the audio half is part of this string rather than an
// input of its own.
const avfoundationScreenDevice = "Capture screen "

// avfNoAudioDevice is the audio half of the device string. It stays unset because
// the two halves travel together: an audio device named here would become this
// input's second stream whatever the audio setting says, and desktop audio is
// refused on this backend for a reason of its own (audioInputArgs).
const avfNoAudioDevice = ":none"

// avfoundationArgs captures a macOS screen through AVFoundation.
//
// The monitor index goes into the device name without a lookup, the way ddagrab's
// output_idx does. What it indexes is AVFoundation's own list of screen devices,
// and display.List has no macOS enumerator to hold it against: the placeholder it
// answers with there would refuse every index but zero.
//
// The device naming is ffmpeg's documented avfoundation input rather than a reading
// off a machine, since macOS is not available here.
func avfoundationArgs(s settings.Stream, fps, _ string) (captureSource, error) {
	return captureSource{args: []string{
		"-f", "avfoundation",
		"-capture_cursor", "1",
		"-framerate", fps,
		"-i", avfoundationScreenDevice + strconv.Itoa(s.Monitor) + avfNoAudioDevice,
	}}, nil
}

// pulseMonitorDevice is the libpulse magic name for the monitor of the default
// sink: the mixed desktop audio. PipeWire's pulse server implements it as well,
// so the same device string works on both sound servers.
const pulseMonitorDevice = "@DEFAULT_MONITOR@"

// audioInputArgs returns the audio capture input for the selected audio source.
// Desktop audio comes from the PulseAudio/PipeWire monitor source, which only a
// Linux session serves. A backend on another platform rejects the option instead
// of publishing a silent track, and each rejection names why that platform has no
// mixed output to read: the two are different missing pieces, and a user who reads
// one reason cannot act on the other.
func audioInputArgs(s settings.Stream) ([]string, error) {
	switch s.Audio {
	case "", "none":
		return nil, nil
	case "desktop":
		switch s.Capture {
		case "ddagrab", "gdigrab":
			return nil, fmt.Errorf("desktop audio capture needs PulseAudio/PipeWire, which the %s (Windows) backend cannot reach", s.Capture)
		case "avfoundation":
			return nil, fmt.Errorf("desktop audio capture needs PulseAudio/PipeWire, and the %s (macOS) backend reaches no equivalent: AVFoundation enumerates input devices only, so what the machine plays is not a source ffmpeg can open", s.Capture)
		}
		return []string{"-f", "pulse", "-i", pulseMonitorDevice}, nil
	default:
		return nil, fmt.Errorf("unknown audio source %q", s.Audio)
	}
}

// audioEncodeArgs encodes the captured audio as a stereo track in the configured
// codec. The encoder name, the sample rate and the bitrate all come from the
// audio table, so this engine and the GStreamer one code the same track from one
// declaration rather than from two hardcoded element lists.
//
// The stream reaching here is validated, so an unknown codec or one this engine
// cannot code is a caller that skipped the validator rather than a user's value.
func audioEncodeArgs(s settings.Stream) ([]string, error) {
	a, ok := capabilities.GetAudio(s.AudioTrack())
	if !ok {
		return nil, fmt.Errorf("unknown audio codec %q", s.AudioTrack())
	}
	enc, ok := a.EncoderOn(capabilities.EngineFfmpeg)
	if !ok {
		return nil, fmt.Errorf("audio codec %s has no ffmpeg encoder", a.Name)
	}
	return []string{
		"-c:a", enc.Element,
		"-b:a", fmt.Sprintf("%dk", a.BitrateK),
		"-ar", strconv.Itoa(a.Rate),
		"-ac", "2",
	}, nil
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
