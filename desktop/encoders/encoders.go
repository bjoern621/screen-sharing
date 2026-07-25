// Package encoders reports which video encoders this machine can actually run, so
// the UI can grey out codecs that would only fail at launch.
//
// Two kinds of encoder cannot be assumed present. A hardware codec needs the GPU,
// its driver and the matching ffmpeg build: NVENC an NVIDIA card, VAAPI a render node
// whose driver exposes that encode entrypoint, which is where the families diverge
// per generation, an AMD card carrying no VP8 or VP9 encoder and a pre-Arc Intel one
// no AV1. The AV1 and VPx software encoders each need their library compiled in,
// which a bundled or distro ffmpeg may well lack. In both cases the encoder appearing
// in "ffmpeg -encoders" only proves the build lists it, not that it initializes, so
// the reliable test is to run a one-frame encode and see if it exits cleanly, which
// is what Detect does.
//
// The verdict is ffmpeg's. The portal capture backend encodes through GStreamer
// elements instead, whose availability is a separate question this probe cannot
// answer, so a codec reported unusable is greyed on both publish paths.
package encoders

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/ffmpeg"
)

// probed lists the encoders worth testing. libx264 and libx265 are absent: both are
// in every ffmpeg build worth shipping and neither fails to initialize.
var probed = []string{
	"hevc_nvenc", "h264_nvenc", "av1_nvenc",
	"libvpx", "libvpx-vp9", "libaom-av1", "libsvtav1", "librav1e",
	"h264_vaapi", "hevc_vaapi", "av1_vaapi", "vp9_vaapi", "vp8_vaapi",
}

// probeTimeout bounds a single test encode. A working encoder returns in well
// under a second; the timeout only guards against a hung ffmpeg.
const probeTimeout = 10 * time.Second

// Availability maps each probed codec to whether a test encode succeeded on this
// machine. Codecs absent from probed do not appear here and the UI treats them as
// always available.
type Availability struct {
	Usable map[string]bool `json:"usable"`
}

// Detect probes every codec concurrently and returns which ones this machine can
// run. If ffmpeg cannot be located, every probed codec is reported unusable,
// leaving the x264 and x265 encoders as the only choice.
func Detect(ctx context.Context) Availability {
	usable := make(map[string]bool, len(probed))

	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		for _, codec := range probed {
			usable[codec] = false
		}
		return Availability{Usable: usable}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, codec := range probed {
		wg.Add(1)
		go func(codec string) {
			defer wg.Done()
			ok := probe(ctx, exe, codec)
			mu.Lock()
			usable[codec] = ok
			mu.Unlock()
		}(codec)
	}
	wg.Wait()

	return Availability{Usable: usable}
}

// probeSize is the frame every test encode runs on. It clears each probed encoder's
// minimum: SVT-AV1 refuses anything under 64 square, and a fixed-function encoder
// refuses more than that, with a floor that varies by driver and codec (an AMD VCN
// wants 130 pixels of width for HEVC and 128 for H.264).
const probeSize = "256x256"

// probe encodes a single frame with codec and reports whether ffmpeg exited cleanly.
// Success confirms the whole chain: the build has the encoder and, for a hardware
// codec, the driver is loaded and a GPU accepted the session.
//
// The frame is 8-bit 4:2:0, so the verdict is per codec and not per chroma: an
// encoder that opens here but implements no 10-bit profile fails at launch on p010le
// instead. capabilities.Codecs declares only the bit depths a family's drivers
// implement broadly, which is what keeps that gap narrow.
func probe(ctx context.Context, exe, codec string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	args := []string{"-hide_banner", "-loglevel", "error"}
	// A VAAPI encoder reads GPU surfaces, so the probe opens the device and uploads
	// the frame exactly as the publish command does. A machine with no render node
	// fails every VAAPI codec here, which is the verdict the UI wants anyway.
	if capabilities.IsVaapi(codec) {
		device, err := ffmpeg.VaapiDevice()
		if err != nil {
			return false
		}
		upload, err := ffmpeg.VaapiFilters("yuv420p")
		if err != nil {
			return false
		}
		args = append(args, device...)
		args = append(args, "-f", "lavfi", "-i", "nullsrc=s="+probeSize, "-frames:v", "1",
			"-vf", strings.Join(upload, ","))
	} else {
		args = append(args, "-f", "lavfi", "-i", "nullsrc=s="+probeSize, "-frames:v", "1")
	}
	args = append(args, "-c:v", codec, "-f", "null", "-")

	return exec.CommandContext(ctx, exe, args...).Run() == nil
}
