// Package encoders reports which video encoders this machine runs,
// so a codec that would only fail at launch is greyed out instead of offered.
//
// Two kinds of encoder cannot be assumed present.
// A hardware codec needs the GPU, its driver and the matching build:
// NVENC an NVIDIA card, VAAPI a render node whose driver exposes that encode entrypoint,
// QSV an Intel GPU whose oneVPL runtime the dispatcher finds,
// Vulkan Video a driver implementing the encode extension for that format.
// A family diverges per generation on top of that, an AMD card carrying no VP8 or VP9 encoder
// and a pre-Arc Intel one no AV1.
// The AV1 and VPx software encoders each need their library compiled in,
// which a bundled or distro build may lack.
//
// The verdict is per publish engine, the two wrapping different encoder implementations.
// An ffmpeg build with librav1e compiled in says nothing about whether this GStreamer install
// carries the rav1enc element, and either one can be the missing half.
// Each engine is probed the way its own failure shows up,
// so a codec running on one engine alone is greyed for the capture backends that cannot run it
// rather than for all of them.
//
// Every verdict is a reading of the machine, so nothing here asserts on one:
// a missing encoder is an Umgebungsfehler and the whole point of asking.
package encoders

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/transport"
)

// gstInspectExe queries the GStreamer registry.
// Ships in the same package as the pipeline launcher, so an install that can publish carries it.
const gstInspectExe = "gst-inspect-1.0"

// engineProbe is one publish engine's answer to "does this machine run this codec".
// available answers for the engine as a whole, codecs lists what is worth testing there,
// and usable runs the test.
type engineProbe struct {
	available func() error
	codecs    func() []string
	usable    func(ctx context.Context, codec string) bool
}

// engineProbes holds the probe per publish engine, covering publish.Engines.
//
// The two probes differ because the two engines fail differently.
// An ffmpeg encoder listed by "ffmpeg -encoders" proves the build carries it,
// not that it initializes, so the test that answers is a one-frame encode.
// A GStreamer element is absent from the registry when its plugin is not installed,
// and the hardware plugins register elements per detected device,
// so one registry query answers both halves and costs no encode per codec.
var engineProbes = map[string]engineProbe{
	publish.EngineFfmpeg: {available: ffmpegAvailable, codecs: ffmpegProbed, usable: ffmpegUsable},
	publish.EngineGst:    {available: gstAvailable, codecs: gstProbed, usable: gstUsable},
}

// ffmpegAvailable reports whether the ffmpeg engine can be probed at all.
// Without the executable every test encode fails,
// and a per-codec verdict would then answer a question about ffmpeg
// with a sentence about the machine's encoder hardware.
func ffmpegAvailable() error {
	_, err := ffmpeg.FindExe("ffmpeg")
	return err
}

// gstAvailable reports whether the GStreamer registry can be queried.
// Missing tools mean the engine does not run here at all rather than one plugin being absent.
func gstAvailable() error {
	if _, err := exec.LookPath(gstInspectExe); err != nil {
		return fmt.Errorf("%s not found: this install carries no GStreamer command-line tools, so no encoder element can be located", gstInspectExe)
	}
	return nil
}

// ffmpegAssumed are the encoders no probe is spent on:
// every ffmpeg build worth shipping carries them and none of them fails to initialize,
// so a test encode would spend a process on a foregone answer.
var ffmpegAssumed = []string{"libx264", "libx265"}

// ffmpegProbed lists the ffmpeg encoders worth testing:
// every implemented codec this engine has an encoder for, minus the ones that need no asking.
// Derived from the capability table rather than listed, so a codec added there is probed and greyed
// where it cannot run instead of reaching a launch that fails.
func ffmpegProbed() []string {
	var out []string
	for _, c := range capabilities.Codecs {
		if c.Implemented && capabilities.HasEncoderOn(c.Name, publish.EngineFfmpeg) &&
			!slices.Contains(ffmpegAssumed, c.Name) {
			out = append(out, c.Name)
		}
	}
	return out
}

// gstProbed lists every implemented codec this engine has an encoder for,
// nothing assumed present the way ffmpegAssumed is.
// Each element ships in a plugin package of its own:
// x264enc in gst-plugins-ugly, rav1enc in gst-plugins-rs,
// the va and nvcodec elements in device-conditional plugins.
//
// A codec gapped off this engine is left out rather than probed to false.
// The gap already states why it is unavailable here,
// and a verdict would replace that with the machine's answer
// to a question carrying no element name to look for.
func gstProbed() []string {
	var out []string
	for _, c := range capabilities.Codecs {
		if c.Implemented && capabilities.HasEncoderOn(c.Name, publish.EngineGst) {
			out = append(out, c.Name)
		}
	}
	return out
}

// probeTimeout bounds a single test encode.
// A working encoder returns in well under a second, so the bound only catches a hung ffmpeg.
const probeTimeout = 10 * time.Second

// Availability maps each publish engine to the codecs probed on it and whether each one ran.
// A codec absent from an engine's map was not probed there and counts as available.
// An engine absent from the outer map restricts nothing.
//
// Unprobed names the engines whose own tooling is missing, with the reason.
// Nothing on such an engine was tested and nothing can run there,
// the codecs no probe is spent on included,
// so the reason covers the engine rather than a codec at a time.
// Without it a missing ffmpeg reads as a machine with no encoder hardware,
// and the encoders in ffmpegAssumed stay selectable while being the ones certain to fail at launch.
type Availability struct {
	Usable   map[string]map[string]bool `json:"usable"`
	Unprobed map[string]string          `json:"unprobed"`
	// Legs maps a publish leg to whether this GStreamer install registers every element its sink is made of.
	// A leg absent from the map was not probed and counts as available, as an unprobed codec does.
	//
	// The encoder half of a pipeline is not the whole of it: an install carrying the older WHIP
	// element and not the one this app builds passes every encoder probe there is, and the leg then
	// starts, dies at launch and spends its retry budget on an element the registry never held.
	// The GStreamer engine alone, the ffmpeg engine's legs being muxers its own build carries or does
	// not, which its executable check already answers for.
	Legs map[string]bool `json:"legs"`
}

// Detect probes every codec on every publish engine, concurrently, and answers what this machine
// runs.
// An engine that cannot be asked contributes its reason and no verdicts.
//
// A call is one child process per probed codec, all of them at once, so the sweep costs the slowest
// of them rather than their sum.
// Nothing here is cached: a call re-reads the machine, and holding an answer for the process
// lifetime is the caller's (internal/app).
func Detect(ctx context.Context) Availability {
	assert.IsNotNil(ctx, "a probe sweep runs under a context")

	usable := make(map[string]map[string]bool, len(engineProbes))
	unprobed := make(map[string]string, len(engineProbes))
	legs := make(map[string]bool)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for engine, probe := range engineProbes {
		assert.IsNotNil(probe.available, "every engine states whether it can be asked", engine)
		assert.IsNotNil(probe.codecs, "every engine states what is worth probing on it", engine)
		assert.IsNotNil(probe.usable, "every engine states how a codec is tested on it", engine)

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

	// The legs of the GStreamer engine, beside its codecs and under the same excuse: an install
	// with no command-line tools answers for no element, sink or encoder.
	if _, excused := unprobed[capabilities.EngineGst]; !excused {
		for _, leg := range transport.Names() {
			elements := transport.GstSinkElements(leg)
			if len(elements) == 0 {
				continue
			}
			wg.Add(1)
			go func(leg string, elements []string) {
				defer wg.Done()
				ok := gstElementsExist(ctx, elements)
				mu.Lock()
				legs[leg] = ok
				mu.Unlock()
			}(leg, elements)
		}
	}
	wg.Wait()

	// An engine is asked or excused, exactly one of the two.
	// A name in both maps would let a surface read a verdict off an engine it was told carries none.
	for engine := range engineProbes {
		_, asked := usable[engine]
		_, excused := unprobed[engine]
		assert.Assert(asked != excused, "an engine is either probed or excused", engine, asked, excused)
	}
	return Availability{Usable: usable, Unprobed: unprobed, Legs: legs}
}

// gstElementsExist reports whether this GStreamer install registers every one of them.
// All of them: a sink is the elements it is made of, and one missing is a pipeline that does not
// build.
func gstElementsExist(ctx context.Context, elements []string) bool {
	assert.IsNotNil(ctx, "an element query runs under a context")
	assert.Assert(len(elements) > 0, "an element query names what it looks up")

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	for _, elem := range elements {
		if exec.CommandContext(ctx, gstInspectExe, "--exists", elem).Run() != nil {
			return false
		}
	}
	return true
}

// probeSize is the frame every test encode runs on.
// It clears every probed encoder's minimum: SVT-AV1 refuses anything under 64 square, and
// a fixed-function encoder refuses more than that, on a floor that varies by driver and codec (an
// AMD VCN wants 130 pixels of width for HEVC and 128 for H.264).
const probeSize = "256x256"

// ffmpegUsable encodes a single frame with codec and reports whether ffmpeg exited cleanly.
// A clean exit confirms the whole chain: the build carries the encoder and, for a hardware codec,
// the driver is loaded and a GPU accepted the session.
// Detect located the executable first (ffmpegAvailable), so a failure here is the encoder's and not
// the engine's.
//
// The frame is 8-bit 4:2:0, so the verdict is per codec and not per chroma: an encoder that opens
// here and implements no 10-bit profile fails at launch on p010le instead.
// capabilities.Codecs declares only the bit depths a family's drivers implement broadly, keeping
// that gap narrow.
func ffmpegUsable(ctx context.Context, codec string) bool {
	assert.IsNotNil(ctx, "a test encode runs under a context")
	assert.Assert(codec != "", "a test encode names the codec it tests")

	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	device, surface, err := ffmpeg.HwSurfaceDevice(codec)
	if err != nil {
		// The family has no device here, no render node for a VAAPI codec among them,
		// which is the unusable verdict anyway.
		return false
	}

	args := []string{"-hide_banner", "-loglevel", "error"}
	args = append(args, device...)
	args = append(args, "-f", "lavfi", "-i", "nullsrc=s="+probeSize, "-frames:v", "1")
	// A VAAPI, QSV or Vulkan encoder reads GPU surfaces, so the probe uploads the frame exactly
	// as the publish command does.
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

// gstUsable reports whether this GStreamer install registers the element that encodes codec.
// Registration carries both conditions: an uninstalled plugin contributes no element, and the va
// and nvcodec plugins enumerate devices at load and register an element per encode entrypoint
// the hardware exposes, so a missing card leaves the name unresolved exactly as a missing plugin
// does.
func gstUsable(ctx context.Context, codec string) bool {
	assert.IsNotNil(ctx, "an element query runs under a context")
	assert.Assert(codec != "", "an element query names the codec it looks up")

	elem, ok := publish.GstEncoderElement(codec)
	if !ok {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return exec.CommandContext(ctx, gstInspectExe, "--exists", elem).Run() == nil
}
