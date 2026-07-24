// Package ffmpeg builds the ffmpeg/ffplay command lines and supervises the
// child processes that publish a captured screen to the relay and play a
// stream back.
//
// The argument builders mirror the prototype scripts in scripts/publish.ps1 and
// scripts/watch.ps1; the values there were tuned against a real relay and are
// the reference for what these functions must produce. The destination URL and
// muxer come from the transport registry, so the encoder args stay independent
// of how bytes leave the machine.
package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// BuildPublishArgs returns the ffmpeg arguments (without the executable) that
// capture the selected monitor and push the encoded stream to the relay.
//
// The order matches scripts/publish.ps1: capture input, encoder, pixel format
// and color range, GOP, then the transport's muxer and destination URL.
func BuildPublishArgs(s settings.Stream) ([]string, error) {
	t, ok := transport.Get(s.Transport)
	if !ok {
		return nil, fmt.Errorf("unknown transport %q", s.Transport)
	}

	if err := validateCodec(s); err != nil {
		return nil, err
	}

	capture, err := captureArgs(s)
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
	args = append(args, enc...)
	args = append(args, "-pix_fmt", s.Chroma)
	// color_range only applies to YUV formats; gbrp is inherently full range.
	if s.Chroma != "gbrp" {
		args = append(args, "-color_range", s.ColorRange)
	}
	args = append(args, "-g", strconv.Itoa(gop))
	args = append(args, t.PublishArgs(s)...)

	return args, nil
}

// BuildWatchArgs returns the ffplay arguments (without the executable) that open
// streamName from the relay in a low-latency viewer window.
//
// -loglevel fatal -nostats is deliberate: ffplay's status line and per-frame
// decode messages otherwise go to a console whose blocking write stalls the
// decode loop, expiring SRT packets and freezing the picture.
func BuildWatchArgs(s settings.Stream, streamName string) ([]string, error) {
	t, ok := transport.Get(s.Transport)
	if !ok {
		return nil, fmt.Errorf("unknown transport %q", s.Transport)
	}

	return []string{
		"-hide_banner", "-loglevel", "fatal", "-nostats",
		"-fflags", "nobuffer", "-flags", "low_delay", "-framedrop",
		"-window_title", WatchWindowTitle(streamName),
		t.WatchURL(s, streamName),
	}, nil
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
		display := os.Getenv("DISPLAY")
		if display == "" {
			display = ":0.0"
		}
		return []string{"-f", "x11grab", "-framerate", fps, "-i", display}, nil
	case "kmsgrab":
		// Direct KMS capture for a bare display server. Needs CAP_SYS_ADMIN
		// (run the binary with the capability set or via sudo). hwdownload moves
		// the captured frame off the GPU so a software or nvenc encoder can read it.
		return []string{
			"-device", drmCaptureDevice(), "-f", "kmsgrab", "-framerate", fps, "-i", "-",
			"-vf", "hwdownload,format=bgr0",
		}, nil
	default:
		return nil, fmt.Errorf("unknown capture backend %q", s.Capture)
	}
}

// drmCaptureDevice returns the /dev/dri card node kmsgrab should capture from.
// card0 is not a safe default: on machines with a discrete GPU the boot
// framebuffer often holds card0 under the simple-framebuffer driver while the
// real display controller lands on card1 or higher. The first card whose kernel
// driver is not simple-framebuffer wins; /dev/dri/card0 is the fallback when the
// sysfs probe finds nothing (no DRI nodes, or none with a readable driver).
func drmCaptureDevice() string {
	const fallback = "/dev/dri/card0"

	// filepath.Glob returns matches in lexical order, so the lowest-numbered
	// real GPU is selected.
	cards, err := filepath.Glob("/dev/dri/card[0-9]*")
	if err != nil {
		return fallback
	}
	for _, dev := range cards {
		// The device/driver symlink resolves to the bound kernel module; its
		// base name is the driver, e.g. i915, amdgpu, nvidia, simple-framebuffer.
		driver, err := os.Readlink(filepath.Join("/sys/class/drm", filepath.Base(dev), "device", "driver"))
		if err != nil {
			continue // no driver bound (or not a real DRM card): skip it
		}
		if filepath.Base(driver) == "simple-framebuffer" {
			continue
		}
		return dev
	}
	return fallback
}

// encoderArgs returns the encoder arguments for the configured codec and mode.
func encoderArgs(s settings.Stream) ([]string, error) {
	bitrate := fmt.Sprintf("%dM", s.BitrateM)
	cq := strconv.Itoa(s.Cq)
	bframes := strconv.Itoa(s.Bframes)

	switch {
	case s.Codec == "libx264":
		if s.Mode == "quality" {
			return []string{"-c:v", "libx264", "-preset", "slow", "-crf", cq}, nil
		}
		return []string{"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency", "-b:v", bitrate}, nil

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
		case "quality":
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
		default: // latency
			if preset == "" {
				preset = "p5"
			}
			return []string{"-c:v", s.Codec, "-preset", preset, "-tune", "ll", "-rc", "cbr", "-b:v", bitrate, "-bf", "0"}, nil
		}

	default:
		// amf / qsv and anything else: a generic bitrate-targeted path.
		return []string{"-c:v", s.Codec, "-b:v", bitrate}, nil
	}
}

// validateCodec rejects a codec/chroma/transport combination the capability
// table forbids, so a settings object the frontend never normalized cannot
// produce a broken ffmpeg command.
func validateCodec(s settings.Stream) error {
	c, ok := capabilities.Get(s.Codec)
	if !ok {
		return fmt.Errorf("unknown codec %q", s.Codec)
	}
	if !capabilities.SupportsChroma(c.Name, s.Chroma) {
		return fmt.Errorf("codec %s cannot encode pixel format %s", c.Name, s.Chroma)
	}
	if !capabilities.CarriedBy(c.Name, s.Transport) {
		return fmt.Errorf("transport %s cannot carry codec %s", s.Transport, c.Name)
	}
	return nil
}

// WatchWindowTitle returns the title ffplay sets on the viewer window for
// streamName. WindowExists polls for exactly this string.
func WatchWindowTitle(streamName string) string {
	return "watch: " + streamName
}

// FindExe locates an ffmpeg-family executable (name is "ffmpeg" or "ffplay").
// A copy shipped next to the app binary wins over one on PATH, so a bundled
// build is self-contained.
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
