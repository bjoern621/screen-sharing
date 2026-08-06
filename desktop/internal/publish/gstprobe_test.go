package publish

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// probeSettings is a stream the probe builder can be exercised on: the software
// encoder every install carries, at a size small enough that a real run of it costs
// nothing.
func probeSettings() settings.Stream {
	s := settings.Defaults()
	s.Capture = "portal"
	s.Codec = "libx264"
	s.Mode = "crf"
	s.Chroma = "yuv420p"
	s.Fps = 30
	return s
}

// The probe pipeline is a wire format like the encoder mappings themselves: an
// element name or a property this builder gets wrong is a measurement that fails
// where the publish it predicts would have run. So this launches the real thing on
// both ends of the content range.
func TestGstEncodeProbeAgainstGstLaunch(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	for _, tc := range []struct {
		name  string
		heavy bool
	}{
		{"heavy", true},
		{"light", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, err := GstEncodeProbe(probeSettings(), 320, 240, 2, tc.heavy)
			if err != nil {
				t.Fatalf("building the probe: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
			defer cancel()
			out, err := exec.CommandContext(ctx, GstExe, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s %s\n%v\n%s", GstExe, strings.Join(args, " "), err, out)
			}
		})
	}
}

// The ceiling pipeline is what says whether a measurement found the encoder or the
// generator, so it has to run on the same frames the encode does and stop at the
// sink.
func TestGstProbeCeilingAgainstGstLaunch(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	args := GstProbeCeiling(probeSettings(), 320, 240, 2, true)
	ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, GstExe, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %s\n%v\n%s", GstExe, strings.Join(args, " "), err, out)
	}
}

// The probe exists to time the encoder, so the generator has to be on a thread of its
// own. Serialised behind it, the reading is the sum of the two and the encoder is
// priced with its instrument.
func TestGstEncodeProbeDecouplesTheGenerator(t *testing.T) {
	args, err := GstEncodeProbe(probeSettings(), 320, 240, 2, true)
	if err != nil {
		t.Fatalf("building the probe: %v", err)
	}
	pipeline := strings.Join(args, " ")
	queue := strings.Join(gstProbeQueue, " ")
	if !strings.Contains(pipeline, queue) {
		t.Errorf("the probe runs the generator on the encoder's thread\ngot: %s", pipeline)
	}
	if strings.Index(pipeline, queue) > strings.Index(pipeline, "x264enc") {
		t.Errorf("the queue sits after the encoder, so it decouples nothing\ngot: %s", pipeline)
	}
}

// The probe measures the encoder the settings name, so what it builds around that
// encoder has to be what a publish builds: the same rate-control properties on the
// same element, reached through the same encoder-input caps.
func TestGstEncodeProbeCarriesTheRunsEncoder(t *testing.T) {
	s := probeSettings()
	probe, err := GstEncodeProbe(s, 320, 240, 2, true)
	if err != nil {
		t.Fatalf("building the probe: %v", err)
	}
	mem, err := gstMemory(s)
	if err != nil {
		t.Fatalf("resolving the frame memory: %v", err)
	}
	encoder, _, err := gstEncoder(s, gstGop(s), mem.memory)
	if err != nil {
		t.Fatalf("building the encoder: %v", err)
	}
	if !strings.Contains(strings.Join(probe, " "), strings.Join(encoder, " ")) {
		t.Errorf("the probe encodes with something other than the run's encoder\nprobe:   %s\nencoder: %s",
			strings.Join(probe, " "), strings.Join(encoder, " "))
	}

	caps, err := gstEncoderCaps(s, mem)
	if err != nil {
		t.Fatalf("building the encoder caps: %v", err)
	}
	if !strings.Contains(strings.Join(probe, " "), caps) {
		t.Errorf("the probe feeds the encoder caps a run does not\nprobe: %s\ncaps:  %s",
			strings.Join(probe, " "), caps)
	}
}

// A probe of a device path has to reach that path's encoder, which is a different element
// reading a different memory. The generated frames start in system memory where a captured
// one is already on the device, so the family's upload has to sit between the generator and
// the conversion: without it the probe builds a pipeline whose first link fails, and the
// measurement is missing exactly where the publish it predicts runs fastest.
func TestGstEncodeProbeReachesTheDeviceEncoder(t *testing.T) {
	s := gstD3d11Stream()
	s.Fps = 30
	probe, err := GstEncodeProbe(s, 320, 240, 2, true)
	if err != nil {
		t.Fatalf("building the probe: %v", err)
	}
	mem, err := gstMemory(s)
	if err != nil {
		t.Fatal(err)
	}
	if mem.upload == "" {
		t.Fatalf("the %s path names no upload, so nothing puts generated frames where %s reads them",
			mem.memory, mem.convert)
	}
	line := strings.Join(probe, " ")
	upload, convert := strings.Index(line, mem.upload), strings.Index(line, mem.convert)
	if upload < 0 || convert < 0 || upload > convert {
		t.Errorf("the probe must upload before it converts: %s", line)
	}
	elem, named := GstEncoderElementOn(s.Codec, mem.memory)
	if !named {
		t.Fatalf("%s names no encoder element in %s memory", s.Codec, mem.memory)
	}
	if !strings.Contains(line, elem) {
		t.Errorf("the probe measures something other than the element a run on this path launches (%s): %s", elem, line)
	}
}

// The transport is no part of what an encoder costs, so a leg that cannot carry the
// codec has to leave the measurement alone. Refusing here would answer a question
// about this CPU with a fact about a protocol, and the settings form would lose the
// probe exactly where a user is picking their way out of the refusal.
func TestGstEncodeProbeIgnoresTheTransport(t *testing.T) {
	s := probeSettings()
	s.Transport = "hls"
	if _, err := GstEncodeProbe(s, 320, 240, 2, true); err != nil {
		t.Errorf("the probe refused a measurement over a transport it does not use: %v", err)
	}
}

// A settings combination the engine cannot encode is refused, since there is no
// encoder to time. That check is the capability table's, so the probe has to run it
// rather than build a pipeline the element rejects at negotiation.
func TestGstEncodeProbeRefusesAGappedCombination(t *testing.T) {
	s := probeSettings()
	s.Chroma = "gbrp" // gstNoPlanarRGB: no encoder element here takes planar RGB
	if _, err := GstEncodeProbe(s, 320, 240, 2, true); err == nil {
		t.Error("the probe built a pipeline for a chroma this engine encodes nothing in")
	}
}

// The keyframe interval is part of what an encode costs, and both halves of this
// engine resolve it from the same rule, so a probe and the run it predicts cannot
// code at different intervals.
func TestGstGopMatchesTheAutomaticSetting(t *testing.T) {
	s := probeSettings()
	s.Gop = 0
	if got, want := gstGop(s), s.Fps*2; got != want {
		t.Errorf("automatic GOP = %d, want %d", got, want)
	}
	s.Gop = 45
	if got, want := gstGop(s), 45; got != want {
		t.Errorf("explicit GOP = %d, want %d", got, want)
	}
}

// Both ends of the range have to be patterns videotestsrc actually has, and they have
// to differ: one pattern measured twice is a range with nothing in it.
func TestGstProbePatternsDiffer(t *testing.T) {
	if gstProbeHeavy == gstProbeLight {
		t.Fatal("both ends of the content range generate the same frames")
	}
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}
	for _, pattern := range []string{gstProbeHeavy, gstProbeLight} {
		ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
		args := []string{
			"videotestsrc", "num-buffers=1", "pattern=" + pattern,
			"!", "video/x-raw,format=" + gstProbeSource + ",width=64,height=64",
			"!", "fakesink",
		}
		out, err := exec.CommandContext(ctx, GstExe, args...).CombinedOutput()
		cancel()
		if err != nil {
			t.Errorf("videotestsrc has no %q pattern: %v\n%s", pattern, err, out)
		}
	}
}

// Whatever this engine can publish, it has to be able to measure. A combination the
// form lets a user start and the probe refuses is one whose cost stays invisible
// until the frames are already being discarded, which is the case the probe exists
// for.
//
// The run is what drives the loop rather than a list of its own: buildPipeline
// deciding a combination is publishable is the whole precondition, so a codec, chroma
// or mode added anywhere below it is covered here without being named.
func TestEveryPublishableCombinationCanBeProbed(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if _, gap := c.EngineGap(EngineGst); !c.Implemented || gap {
			continue
		}
		for _, chroma := range c.EngineChromas(EngineGst) {
			for _, mode := range capabilities.Modes {
				s := probeSettings()
				s.Codec, s.Chroma, s.Mode = c.Name, chroma, mode
				if _, err := buildPipeline(s, []string{"videotestsrc"}, ""); err != nil {
					continue
				}
				if _, err := GstEncodeProbe(s, 320, 240, 2, true); err != nil {
					t.Errorf("%s at %s/%s publishes but cannot be probed: %v",
						c.Name, chroma, mode, err)
				}
			}
		}
	}
}

// A run of no frames or of a picture with no size is a caller's mistake, not a
// user's: the resolution is refused where it is read off the machine, so a probe
// reached with one is a bug above it.
func TestGstEncodeProbeAssertsItsBounds(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		width, height, frames int
	}{
		{"no width", 0, 240, 2},
		{"no height", 320, 0, 2},
		{"no frames", 320, 240, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("the probe accepted a bound it cannot encode")
				}
			}()
			_, _ = GstEncodeProbe(probeSettings(), tc.width, tc.height, tc.frames, true)
		})
	}
}
