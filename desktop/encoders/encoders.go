// Package encoders reports which video encoders this machine can actually run, so
// the UI can grey out codecs that would only fail at launch.
//
// Two kinds of encoder cannot be assumed present. A hardware codec needs the GPU,
// its driver and the matching build: NVENC an NVIDIA card, VAAPI a render node
// whose driver exposes that encode entrypoint, QSV an Intel GPU whose oneVPL runtime the
// dispatcher finds, Vulkan Video a driver implementing the encode extension for that
// format.
// That is where the families diverge per generation, an AMD card carrying no VP8 or VP9
// encoder and a pre-Arc Intel one no AV1.
// The AV1 and VPx software encoders each need their library compiled in, which a bundled
// or distro build may well lack.
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
	"fmt"
	"os/exec"
	"slices"
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
// available reports whether the engine can be asked at all, codecs lists what is
// worth testing there, and usable runs the test.
type engineProbe struct {
	available func() error
	codecs    func() []string
	usable    func(ctx context.Context, codec string) bool
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
	publish.EngineFfmpeg: {available: ffmpegAvailable, codecs: ffmpegProbed, usable: ffmpegUsable},
	publish.EngineGst:    {available: gstAvailable, codecs: gstProbed, usable: gstUsable},
}

// ffmpegAvailable reports whether the ffmpeg engine can be probed at all. Without
// the executable every test encode fails, and reporting that as a per-codec verdict
// would answer a question about ffmpeg with a sentence about the machine's encoder
// hardware.
func ffmpegAvailable() error {
	_, err := ffmpeg.FindExe("ffmpeg")
	return err
}

// gstAvailable reports whether the GStreamer registry can be queried. gst-inspect
// ships with the same package as the pipeline launcher, so its absence means this
// install cannot run the engine at all rather than that one plugin is missing.
func gstAvailable() error {
	if _, err := exec.LookPath(gstInspectExe); err != nil {
		return fmt.Errorf("%s not found: this install carries no GStreamer command-line tools, so no encoder element can be located", gstInspectExe)
	}
	return nil
}

// ffmpegAssumed are the encoders no probe is spent on: both are in every ffmpeg build
// worth shipping and neither fails to initialize, so a test encode would spend a
// process on a foregone answer.
var ffmpegAssumed = []string{"libx264", "libx265"}

// ffmpegProbed lists the ffmpeg encoders worth testing: every implemented codec this
// engine has an encoder for, minus the ones that need no asking. It is derived from
// the capability table rather than listed, so a codec added there is probed and greyed
// where it cannot run instead of reaching a launch that fails.
func ffmpegProbed() []string {
	var out []string
	for _, c := range capabilities.Codecs {
		_, gap := c.EngineGap(publish.EngineFfmpeg)
		if c.Implemented && !gap && !slices.Contains(ffmpegAssumed, c.Name) {
			out = append(out, c.Name)
		}
	}
	return out
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
//
// Unprobed names the engines whose own tooling is missing, with the reason. No codec
// on such an engine was tested and none of them can run there, the ones no probe is
// spent on included, so the reason covers the whole engine rather than a codec at a
// time. Without it a missing ffmpeg reads as a machine with no encoder hardware, and
// the two encoders in ffmpegAssumed stay selectable while being the only ones certain
// to fail at launch.
type Availability struct {
	Usable   map[string]map[string]bool `json:"usable"`
	Unprobed map[string]string          `json:"unprobed"`
}

// Detect probes every codec on every publish engine, concurrently, and returns what
// this machine can run. An engine that cannot be asked contributes its reason and no
// verdicts.
func Detect(ctx context.Context) Availability {
	usable := make(map[string]map[string]bool, len(engineProbes))
	unprobed := make(map[string]string, len(engineProbes))

	var mu sync.Mutex
	var wg sync.WaitGroup
	for engine, probe := range engineProbes {
		if err := probe.available(); err != nil {
			unprobed[engine] = err.Error()
			continue
		}
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

	return Availability{Usable: usable, Unprobed: unprobed}
}

// probeSize is the frame every test encode runs on. It clears each probed encoder's
// minimum: SVT-AV1 refuses anything under 64 square, and a fixed-function encoder
// refuses more than that, with a floor that varies by driver and codec (an AMD VCN
// wants 130 pixels of width for HEVC and 128 for H.264).
const probeSize = "256x256"

// ffmpegUsable encodes a single frame with codec and reports whether ffmpeg exited
// cleanly. Success confirms the whole chain: the build has the encoder and, for a
// hardware codec, the driver is loaded and a GPU accepted the session. Detect has
// already located the executable (ffmpegAvailable), so a failure here is the
// encoder's and not the engine's.
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

	device, surface, err := ffmpeg.HwSurfaceDevice(codec)
	if err != nil {
		// The family has no device on this machine: no render node for VAAPI, which is
		// the verdict the UI wants anyway.
		return false
	}

	args := []string{"-hide_banner", "-loglevel", "error"}
	args = append(args, device...)
	args = append(args, "-f", "lavfi", "-i", "nullsrc=s="+probeSize, "-frames:v", "1")
	// A VAAPI, QSV or Vulkan encoder reads GPU surfaces, so the probe uploads the frame
	// exactly as the publish command does.
	if surface {
		upload, err := ffmpeg.HwSurfaceFilters(codec, "yuv420p")
		if err != nil {
			return false
		}
		args = append(args, "-vf", strings.Join(upload, ","))
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
