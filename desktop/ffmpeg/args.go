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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

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

	if err := validateCodec(s); err != nil {
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

// captureArgs returns the input arguments for the configured capture backend.
// ddagrab and gdigrab are Windows-only; x11grab and kmsgrab are Linux-only.
func captureArgs(s settings.Stream) ([]string, error) {
	fps := strconv.Itoa(s.Fps)

	switch s.Capture {
	case "ddagrab":
		// GPU capture as a filter source; hwdownload hands frames back to system
		// memory so any encoder can consume them.
		return []string{
			"-filter_complex",
			fmt.Sprintf("ddagrab=output_idx=%d:framerate=%s,hwdownload,format=bgra", s.Monitor, fps),
		}, nil
	case "gdigrab":
		return []string{"-f", "gdigrab", "-framerate", fps, "-i", "desktop"}, nil
	case "x11grab":
		disp := os.Getenv("DISPLAY")
		if disp == "" {
			disp = ":0.0"
		}
		args := []string{"-f", "x11grab", "-framerate", fps}
		// Crop to the selected monitor when its geometry is known; otherwise
		// capture the whole X screen, as enumeration failing leaves no offset.
		if m, ok := monitorByIndex(s.Monitor); ok && m.Width > 0 && m.Height > 0 {
			args = append(args,
				"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height),
				"-i", fmt.Sprintf("%s+%d,%d", disp, m.OffsetX, m.OffsetY),
			)
		} else {
			args = append(args, "-i", disp)
		}
		return args, nil
	case "kmsgrab":
		return kmsgrabCaptureArgs(s, fps), nil
	default:
		return nil, fmt.Errorf("unknown capture backend %q", s.Capture)
	}
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

// encoderArgs returns the encoder arguments for the configured codec and rate
// control mode. The five modes are the rate-control methods themselves: cbr and
// vbr and abr all target a bitrate and differ in the ceiling, crf targets a
// quality, lossless is bit-exact. The GStreamer publish path expresses the same
// five in gstEncoder.
func encoderArgs(s settings.Stream) ([]string, error) {
	bitrate := fmt.Sprintf("%dM", s.BitrateM)
	maxrate := fmt.Sprintf("%dM", s.MaxrateM)
	cq := strconv.Itoa(s.Cq)
	bframes := strconv.Itoa(s.Bframes)

	switch {
	case s.Codec == "libx264":
		// x264 reaches bit-exact at -qp 0.
		return softwareArgs("libx264", []string{"-qp", "0"}, s, bitrate, maxrate, cq), nil

	case s.Codec == "libx265":
		// x265 has no bit-exact qp; lossless is its own param.
		return softwareArgs("libx265", []string{"-x265-params", "lossless=1"}, s, bitrate, maxrate, cq), nil

	case capabilities.IsNvenc(s.Codec):
		preset := s.EncPreset
		switch s.Mode {
		case "lossless":
			if preset == "" {
				preset = "p7"
			}
			// True nvenc lossless: no rate control, the frame costs whatever
			// exactness costs and can burst well past 1 Gbps.
			return []string{"-c:v", s.Codec, "-preset", preset, "-tune", "lossless", "-bf", bframes}, nil
		case "crf":
			if preset == "" {
				preset = "p7"
			}
			// VBR targeting a constant quantizer: cq drives the look, the bitrate
			// only caps bursts. multipass fullres spends the most effort per bit.
			return []string{
				"-c:v", s.Codec, "-preset", preset, "-tune", "hq", "-multipass", "fullres",
				"-rc", "vbr", "-cq", cq, "-b:v", "0", "-maxrate", bitrate, "-bufsize", bitrate,
				"-bf", bframes,
			}, nil
		case "abr":
			if preset == "" {
				preset = "p7"
			}
			// VBR toward an average with no ceiling.
			return []string{"-c:v", s.Codec, "-preset", preset, "-tune", "hq", "-rc", "vbr", "-b:v", bitrate, "-bf", bframes}, nil
		case "vbr":
			if preset == "" {
				preset = "p7"
			}
			return []string{
				"-c:v", s.Codec, "-preset", preset, "-tune", "hq", "-rc", "vbr",
				"-b:v", bitrate, "-maxrate", maxrate, "-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs),
				"-bf", bframes,
			}, nil
		default: // cbr
			if preset == "" {
				preset = "p5"
			}
			args := []string{"-c:v", s.Codec, "-preset", preset, "-tune", "ll", "-rc", "cbr", "-b:v", bitrate, "-bf", "0"}
			if s.VbvMs > 0 {
				args = append(args, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
			}
			return args, nil
		}

	default:
		// Generic bitrate-targeted path. validateCodec rejects every codec whose
		// capability entry has Implemented:false, so the not-yet-wired hardware
		// families (vaapi, qsv, amf, v4l2, rkmpp, vulkan) never reach here. When
		// one is implemented, give it its own case: VAAPI needs a -vaapi_device
		// and a format=nv12,hwupload filter chain, QSV its own device and load
		// path, so none of them fit this bare -b:v fallback.
		return []string{"-c:v", s.Codec, "-b:v", bitrate}, nil
	}
}

// softwareArgs is the ffmpeg rate-control mapping shared by the CPU H.26x
// encoders libx264 and libx265; the five modes match gstEncoder's software path.
// Only the encoder name and the lossless knob differ between the two, so both
// are parameters: x264 reaches bit-exact at -qp 0, x265 at -x265-params
// lossless=1.
func softwareArgs(codec string, lossless []string, s settings.Stream, bitrate, maxrate, cq string) []string {
	switch s.Mode {
	case "crf":
		return []string{"-c:v", codec, "-preset", "slow", "-crf", cq}
	case "lossless":
		// No rate control, bursts to hundreds of Mbit/s. zerolatency keeps live
		// delay by dropping the B-frames and lookahead lossless gains little from.
		return append([]string{"-c:v", codec, "-preset", "veryfast", "-tune", "zerolatency"}, lossless...)
	case "abr":
		// One-pass average bitrate, no VBV cap: quality holds and bitrate bursts
		// freely toward the target average.
		return []string{"-c:v", codec, "-preset", "medium", "-b:v", bitrate}
	case "vbr":
		// Constrained VBR: targets the bitrate but bursts up to the maxrate
		// ceiling on motion. bufsize sizes the ceiling's VBV window.
		return []string{
			"-c:v", codec, "-preset", "medium",
			"-b:v", bitrate, "-maxrate", maxrate, "-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs),
		}
	default: // cbr
		// maxrate = bitrate with a bounded bufsize is true CBR; without them
		// -b:v alone is one-pass ABR and bursts past a capped link.
		return []string{
			"-c:v", codec, "-preset", "veryfast", "-tune", "zerolatency",
			"-b:v", bitrate, "-maxrate", bitrate, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs),
		}
	}
}

// bufsizeArg returns the ffmpeg -bufsize value in kbit for a rate (Mbit/s) held
// over a VBV window. A zero window defaults to one second, the conventional CBR
// buffer. rateM Mbit/s over ms milliseconds is rateM*ms kbit.
func bufsizeArg(rateM, vbvMs int) string {
	ms := vbvMs
	if ms <= 0 {
		ms = 1000
	}
	return strconv.Itoa(rateM*ms) + "k"
}

// validateCodec rejects a codec/chroma/transport combination the capability
// table forbids, so a settings object the frontend never normalized cannot
// produce a broken ffmpeg command.
func validateCodec(s settings.Stream) error {
	c, ok := capabilities.Get(s.Codec)
	if !ok {
		return fmt.Errorf("unknown codec %q", s.Codec)
	}
	if !c.Implemented {
		return fmt.Errorf("codec %s is listed but not implemented yet", c.Name)
	}
	if !capabilities.SupportsChroma(c.Name, s.Chroma) {
		return fmt.Errorf("codec %s cannot encode pixel format %s", c.Name, s.Chroma)
	}
	if !capabilities.CarriedBy(c.Name, s.Transport) {
		return fmt.Errorf("transport %s cannot carry codec %s", s.Transport, c.Name)
	}
	return nil
}

// FindExe locates a media executable (ffmpeg, ffplay, mpv). A copy shipped
// next to the app binary wins over one on PATH, so a bundled build is
// self-contained.
func FindExe(name string) (string, error) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	if self, err := os.Executable(); err == nil {
		bundled := filepath.Join(filepath.Dir(self), name)
		if info, statErr := os.Stat(bundled); statErr == nil && !info.IsDir() {
			return bundled, nil
		}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found: install ffmpeg or place %s next to the app", name, name)
	}
	return path, nil
}

// EnvKmsgrabFFmpeg names the executable kmsgrab capture runs, overriding the
// backend's default resolution. kmsgrab reads the raw KMS scanout, which the
// kernel gates behind CAP_SYS_ADMIN, so it needs a privileged ffmpeg that the
// other backends must not share. A packaging layer sets this to the capability
// wrapper (nix/screen-share.nix points it at security.wrappers' ffmpeg-kmsgrab).
const EnvKmsgrabFFmpeg = "SCREENSHARE_FFMPEG_KMSGRAB"

// FindCaptureExe locates the ffmpeg build to run for a given capture backend.
//
// Only kmsgrab needs a different binary from the rest: its CAP_SYS_ADMIN
// requirement (see EnvKmsgrabFFmpeg) means the plain ffmpeg from FindExe cannot
// open the input. Its resolution order is the EnvKmsgrabFFmpeg override, then a
// wrapper named ffmpeg-kmsgrab on PATH, then the plain ffmpeg as a last resort
// (which fails on the capability, no worse than before). Every other backend
// uses the plain ffmpeg directly, keeping the privileged binary off the
// unprivileged capture paths.
func FindCaptureExe(capture string) (string, error) {
	if capture == "kmsgrab" {
		if override := os.Getenv(EnvKmsgrabFFmpeg); override != "" {
			return override, nil
		}
		if wrapper, err := exec.LookPath("ffmpeg-kmsgrab"); err == nil {
			return wrapper, nil
		}
	}
	return FindExe("ffmpeg")
}

// LogDir returns the directory that holds per-run ffmpeg logs, creating it if
// needed. It sits beside the settings file under the user config directory.
func LogDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}

	dir := filepath.Join(base, "screenshare", "logs")
	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return "", fmt.Errorf("cannot create log directory %s: %w", dir, err)
	}
	return dir, nil
}
