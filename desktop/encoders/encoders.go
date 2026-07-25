// Package encoders reports which video encoders this machine can actually run, so
// the UI can grey out codecs that would only fail at launch.
//
// Two kinds of encoder cannot be assumed present. A hardware codec needs the GPU,
// its driver and the matching build: NVENC an NVIDIA card, VAAPI a render node
// whose driver exposes that encode entrypoint, which is where the families diverge
// per generation, an AMD card carrying no VP8 or VP9 encoder and a pre-Arc Intel one
// no AV1. The AV1 and VPx software encoders each need their library compiled in,
// which a bundled or distro build may well lack.
//
// The answer is per publish engine, because the two wrap different encoder
// implementations: an ffmpeg build with librav1e compiled in says nothing about
// whether this GStreamer install carries the rav1enc element, and either one can be
// the missing half. Each engine is probed the way its own failure shows up, and a
// codec that runs on one engine only is reported usable there and unusable on the
// other, so the settings form can grey it for the capture backends that cannot run it
// instead of for all of them.
package encoders

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/publish"
)

// gstInspectExe is the GStreamer registry query. It ships with the same package as
// the pipeline launcher, so an install that can publish can answer this.
const gstInspectExe = "gst-inspect-1.0"

// engineProbe is one publish engine's answer to "can this machine run this codec".
// codecs lists what is worth testing there; usable runs the test.
type engineProbe struct {
	codecs func() []string
	usable func(ctx context.Context, codec string) bool
}

// engineProbes holds the probe per publish engine. publish.Engines is the list this
// has to cover, which TestEveryEngineIsProbed checks.
//
// The two probes differ because the two engines fail differently. An ffmpeg encoder
// appearing in "ffmpeg -encoders" only proves the build lists it, not that it
// initializes, so the reliable test is a one-frame encode. A GStreamer element is
// absent from the registry altogether when its plugin is not installed, and the
// hardware plugins register their elements per detected device, so asking the registry
// answers both halves without spawning an encode per codec.
var engineProbes = map[string]engineProbe{
	publish.EngineFfmpeg: {codecs: ffmpegProbed, usable: ffmpegUsable},
	publish.EngineGst:    {codecs: gstProbed, usable: gstUsable},
}

// ffmpegProbed lists the ffmpeg encoders worth testing. libx264 and libx265 are
// absent: both are in every ffmpeg build worth shipping and neither fails to
// initialize.
func ffmpegProbed() []string {
	return []string{
		"hevc_nvenc", "h264_nvenc", "av1_nvenc",
		"libvpx", "libvpx-vp9", "libaom-av1", "libsvtav1", "librav1e",
		"h264_vaapi", "hevc_vaapi", "av1_vaapi", "vp9_vaapi", "vp8_vaapi",
		"h264_amf", "hevc_amf", "av1_amf",
	}
}

// gstProbed lists every implemented codec this engine has an encoder for, unlike the
// ffmpeg half: each element comes from its own plugin package (x264enc from
// gst-plugins-ugly, rav1enc from gst-plugins-rs, the va and nvcodec elements from
// device-conditional plugins), so none of them is safe to assume.
//
// A codec gapped off this engine altogether is left out rather than probed to false.
// The gap already states why it is unavailable here, and a probe verdict would
// replace that with the machine's answer to a question this engine cannot ask: there
// is no element name to look for.
func gstProbed() []string {
	var out []string
	for _, c := range capabilities.Codecs {
		if _, gap := c.EngineGap(publish.EngineGst); c.Implemented && !gap {
			out = append(out, c.Name)
		}
	}
	return out
}

// probeTimeout bounds a single test encode. A working encoder returns in well
// under a second; the timeout only guards against a hung ffmpeg.
const probeTimeout = 10 * time.Second

// Availability maps each publish engine to the codecs probed on it and whether each
// one ran. A codec absent from an engine's map was not probed there and the UI treats
// it as available; an engine absent from the outer map imposes no restriction at all.
type Availability struct {
	Usable map[string]map[string]bool `json:"usable"`
}

// Detect probes every codec on every publish engine, concurrently, and returns what
// this machine can run.
func Detect(ctx context.Context) Availability {
	usable := make(map[string]map[string]bool, len(engineProbes))

	var mu sync.Mutex
	var wg sync.WaitGroup
	for engine, probe := range engineProbes {
		perCodec := make(map[string]bool)
		usable[engine] = perCodec
		for _, codec := range probe.codecs() {
			wg.Add(1)
			go func(codec string, test func(context.Context, string) bool) {
				defer wg.Done()
				ok := test(ctx, codec)
				mu.Lock()
				perCodec[codec] = ok
				mu.Unlock()
			}(codec, probe.usable)
		}
	}
	wg.Wait()

	return Availability{Usable: usable}
}

// probeSize is the frame every test encode runs on. It clears each probed encoder's
// minimum: SVT-AV1 refuses anything under 64 square, and a fixed-function encoder
// refuses more than that, with a floor that varies by driver and codec (an AMD VCN
// wants 130 pixels of width for HEVC and 128 for H.264).
const probeSize = "256x256"

// ffmpegUsable encodes a single frame with codec and reports whether ffmpeg exited
// cleanly. Success confirms the whole chain: the build has the encoder and, for a
// hardware codec, the driver is loaded and a GPU accepted the session. An ffmpeg that
// cannot be located fails every codec, leaving the x264 and x265 encoders, which are
// not probed, as the only choice.
//
// The frame is 8-bit 4:2:0, so the verdict is per codec and not per chroma: an
// encoder that opens here but implements no 10-bit profile fails at launch on p010le
// instead. capabilities.Codecs declares only the bit depths a family's drivers
// implement broadly, which is what keeps that gap narrow.
func ffmpegUsable(ctx context.Context, codec string) bool {
	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		return false
	}

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

// gstUsable reports whether this GStreamer install registers the element that encodes
// codec. Registration is the test because it already carries both conditions: a plugin
// that is not installed contributes no element, and the va and nvcodec plugins
// enumerate devices at load and register an element per encode entrypoint the hardware
// exposes, so a missing card leaves the name unresolved exactly as a missing plugin
// does.
func gstUsable(ctx context.Context, codec string) bool {
	elem, ok := publish.GstEncoderElement(codec)
	if !ok {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return exec.CommandContext(ctx, gstInspectExe, "--exists", elem).Run() == nil
}
