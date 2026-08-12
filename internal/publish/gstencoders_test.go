package publish

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// encodeTimeout bounds one test encode. Two frames at 320x240 return in well under a
// second on every element here, so the only thing this catches is an element that
// takes the frames and emits nothing: svtav1enc does exactly that in the low-delay
// structure, and a stall is a failure like any other, not a reason to wait.
const encodeTimeout = 20 * time.Second

// baseStream is the default draft with the two ladder steps left unnamed.
//
// The tests in this package reach every codec off one draft, and a step is one encoder's
// own identifier, so a draft carrying the default codec's step would hand most of them a
// step from an encoder that never heard of it - which the builder refuses, the way it
// refuses the default quantizer on a codec whose scale stops below it. An unnamed step is
// the codec's declared one (capabilities.Ladder.Resolve), so each codec here encodes at
// the step its own row names for the mode under test.
func baseStream() settings.Settings {
	s := settings.Defaults()
	s.Publish.Effort, s.Publish.Tune = "", ""
	return s
}

// The encoder mappings are a wire format shared with GStreamer: every element name
// has to exist, every property has to be spelled the way that element spells it, and
// every value has to sit in its range. None of that holds against a compiler, and a
// wrong property is a pipeline that only fails once a user hits Publish. So this runs
// a real gst-launch per codec and mode, on videotestsrc rather than the portal node.
//
// The capability table drives the loop, which is what makes the test find a codec
// added there without a mapping, or a mode declared reachable that the element has no
// property for.
func TestGstEncodersAgainstGstLaunch(t *testing.T) {
	if _, err := exec.LookPath(GstExe); err != nil {
		t.Skipf("%s not installed", GstExe)
	}

	for name := range gstCodecs {
		cap, ok := capabilities.Get(name)
		if !ok {
			t.Errorf("%s has a GStreamer mapping but no capability row", name)
			continue
		}
		elem, _ := GstEncoderElement(name)
		if err := exec.Command("gst-inspect-1.0", "--exists", elem).Run(); err != nil {
			t.Logf("skipping %s: %s plugin not installed", name, elem)
			continue
		}

		// The chroma is the last format this engine reaches, which is the narrowest
		// (10-bit where the element takes it, 4:2:0 where it does not) and therefore
		// the one most likely to fail negotiation. A format the table declares gapped
		// here is left out: refusing it is the point, and pinning it would only assert
		// that the element rejects what capabilities.Validate already refuses.
		engineChromas := cap.EngineChromas("gstreamer")

		for _, mode := range []string{"cbr", "vbr", "abr", "crf", "lossless"} {
			if _, gap := cap.OptionGap("gstreamer", capabilities.OptionMode, mode); gap {
				continue
			}
			t.Run(name+"/"+mode, func(t *testing.T) {
				s := baseStream()
				s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = name, mode, engineChromas[len(engineChromas)-1]
				// The quantizer target rides each encoder's own scale, and the default
				// settings carry one from another codec's.
				s.Publish.Cq = cap.CqMaxOn(EngineGst) / 2

				// System memory: what this covers is the properties an element takes,
				// and the frames come off videotestsrc with no device in the chain. The
				// device elements and the layouts they negotiate are measured where the
				// conversion into them is (TestPublishedColorimetryReachesTheDecoder).
				format, err := gstChromaFormat(name, s.Publish.Chroma, gpupath.MemorySystem)
				if err != nil {
					t.Fatal(err)
				}
				encoder, link, err := gstEncoder(s, 60, gpupath.MemorySystem)
				if err != nil {
					t.Fatal(err)
				}

				args := []string{"-q", "videotestsrc", "num-buffers=2",
					"!", "video/x-raw,format=" + format + ",width=320,height=240,framerate=30/1",
					"!"}
				args = append(args, encoder...)
				if len(link) > 0 {
					args = append(args, "!")
					args = append(args, link...)
				}
				args = append(args, "!", "fakesink")

				ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
				defer cancel()
				out, err := exec.CommandContext(ctx, GstExe, args...).CombinedOutput()
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					err = errors.New("the pipeline stalled: two frames in, nothing out")
				}
				// gst-launch reports a bad property or an unset caps field on stderr and
				// still exits zero for some of them, so the output is checked as well.
				if err != nil || strings.Contains(string(out), "no property") {
					t.Errorf("gst-launch %s: %v\n%s", strings.Join(args, " "), err, out)
				}
			})
		}
	}
}

// Every codec the capability table declares implemented has to be buildable on the
// engine that will be asked for it, or the portal capture backend fails at launch on a
// combination the UI offered. A codec the table gaps off this engine is not asked for:
// Validate refuses it here, which is the AMF rows' case, their plugin being
// Windows-only, and the Vulkan ones', whose encoder takes Vulkan device memory.
func TestEveryImplementedCodecHasAGstMapping(t *testing.T) {
	for _, c := range capabilities.Codecs {
		_, gap := c.EngineGap(EngineGst)
		if !c.Implemented || gap {
			continue
		}
		if _, ok := gstCodecs[c.Name]; !ok {
			t.Errorf("codec %s is implemented but has no GStreamer encoder mapping", c.Name)
		}
	}
}

// The reverse holds too: a mapping for a codec this engine is told it cannot run is
// dead code the pipeline never reaches, and the gap's reason would be a lie.
func TestNoGstMappingForAGappedCodec(t *testing.T) {
	for name := range gstCodecs {
		c, ok := capabilities.Get(name)
		if !ok {
			continue // TestGstEncodersAgainstGstLaunch reports the missing row
		}
		if gap, ok := c.EngineGap(EngineGst); ok {
			t.Errorf("codec %s has a GStreamer mapping and a gap saying it has none: %s", name, gap.Reason)
		}
	}
}

// The quantizer target reaches every element on that element's own scale, so a codec
// counting to 255 must not be handed a value clamped to 51, and the reverse.
// The scale read here is this engine's: qsvvp9enc passes VP9's own quantizer index through
// where the ffmpeg wrapper of the same silicon states a CQP on the H.26x scale.
func TestGstEncoderQuantizerFollowsTheCodecScale(t *testing.T) {
	for _, name := range []string{"libx264", "libvpx-vp9", "librav1e", "vp9_qsv"} {
		cap, _ := capabilities.Get(name)
		cqMax := cap.CqMaxOn(EngineGst)
		s := baseStream()
		s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma, s.Publish.Cq = name, "crf", cap.EngineChromas(EngineGst)[0], cqMax
		encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
		if err != nil {
			t.Fatal(err)
		}
		want := strconv.Itoa(cqMax)
		if line := strings.Join(encoder, " "); !strings.Contains(line, "="+want) {
			t.Errorf("%s crf at its maximum quantizer: %s, want a property set to %s", name, line, want)
		}
	}
}

// The va elements state a VBR target as a percentage of the ceiling and take 50 at the
// lowest, so a target under half its ceiling is a pair they have no form for. Coding it
// at the floor would run 100 Mbit/s where 20 was asked for, and the ffmpeg engine hands
// the same settings to the same hardware as -b:v 20M -maxrate 200M.
func TestGstVaVbrRefusesATargetUnderHalfTheCeiling(t *testing.T) {
	s := baseStream()
	s.Publish.Codec, s.Publish.Chroma, s.Publish.Mode = "h264_vaapi", "yuv420p", "vbr"
	s.Publish.BitrateM, s.Publish.MaxrateM = 20, 200
	_, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err == nil {
		t.Fatalf("h264_vaapi vbr at %d/%d Mbit/s must be refused, not encoded at another rate", s.Publish.BitrateM, s.Publish.MaxrateM)
	}
	for _, want := range []string{"20 Mbit/s", "200 Mbit/s"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q names neither number, want %s in it", err, want)
		}
	}

	// Half the ceiling is the lowest ratio the property expresses, so it builds, and
	// what it builds targets what the settings state.
	s.Publish.BitrateM = 100
	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(encoder, " "); !strings.Contains(line, "target-percentage=50") {
		t.Errorf("h264_vaapi vbr at half its ceiling: %s, want target-percentage=50", line)
	}
}

// Every bitrate mode can drive the bitrate property past the range it accepts: cbr and
// vbr with the figure the settings carry, abr with the ceiling it derives at twice the
// target. Each is refused, since the alternative is a stream running at the bound
// instead of at the rate the form shows.
func TestGstVaRefusesARateAboveTheBitrateBound(t *testing.T) {
	aboveBoundM := vaMaxBitrateKbps/1000 + 1
	for _, tc := range []struct {
		mode               string
		bitrateM, maxrateM int
	}{
		{"cbr", aboveBoundM, aboveBoundM},
		{"vbr", aboveBoundM, aboveBoundM},
		{"abr", aboveBoundM/vaAbrPeak + 1, 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			s := baseStream()
			s.Publish.Codec, s.Publish.Chroma, s.Publish.Mode = "h264_vaapi", "yuv420p", tc.mode
			s.Publish.BitrateM, s.Publish.MaxrateM = tc.bitrateM, tc.maxrateM
			_, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
			if err == nil {
				t.Fatalf("h264_vaapi %s at %d Mbit/s must be refused, not clamped", tc.mode, tc.bitrateM)
			}
			if !strings.Contains(err.Error(), strconv.Itoa(vaMaxBitrateKbps)) {
				t.Errorf("the refusal %q does not name the %d kbit/s limit", err, vaMaxBitrateKbps)
			}
		})
	}

	// The bound itself is a rate the property takes.
	s := baseStream()
	s.Publish.Codec, s.Publish.Chroma, s.Publish.Mode = "h264_vaapi", "yuv420p", "cbr"
	s.Publish.BitrateM = vaMaxBitrateKbps / 1000
	if _, _, err := gstEncoder(s, 60, gpupath.MemorySystem); err != nil {
		t.Errorf("h264_vaapi cbr at the property's highest rate: %v", err)
	}
}

// The AV1 and VP9 qsv elements hold their bitrate in an unsigned 16-bit property where
// the H.264 and H.265 ones take any rate, so the bound belongs to the row and not to the
// family. Each bitrate mode can drive it past the range: cbr and vbr with the figure the
// settings carry, abr with the ceiling it derives at twice the target.
func TestGstQsvRefusesARateAboveTheShortBitrateBound(t *testing.T) {
	aboveBoundM := qsvShortBitrateKbps/1000 + 1
	for _, tc := range []struct {
		mode               string
		bitrateM, maxrateM int
	}{
		{"cbr", aboveBoundM, aboveBoundM},
		{"vbr", aboveBoundM, aboveBoundM},
		{"abr", aboveBoundM/qsvAbrPeak + 1, 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			s := baseStream()
			s.Publish.Codec, s.Publish.Chroma, s.Publish.Mode = "av1_qsv", "yuv420p", tc.mode
			s.Publish.BitrateM, s.Publish.MaxrateM = tc.bitrateM, tc.maxrateM
			_, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
			if err == nil {
				t.Fatalf("av1_qsv %s at %d Mbit/s must be refused, not clamped", tc.mode, tc.bitrateM)
			}
			if !strings.Contains(err.Error(), strconv.Itoa(qsvShortBitrateKbps)) {
				t.Errorf("the refusal %q does not name the %d kbit/s limit", err, qsvShortBitrateKbps)
			}

			// The same rate on an H.26x element is inside its property's range, so the
			// bound must not reach it.
			s.Publish.Codec = "h264_qsv"
			if _, _, err := gstEncoder(s, 60, gpupath.MemorySystem); err != nil {
				t.Errorf("h264_qsv %s at %d Mbit/s: %v", tc.mode, tc.bitrateM, err)
			}
		})
	}

	// The bound itself is a rate the property takes, and crf drives no rate at all.
	s := baseStream()
	s.Publish.Codec, s.Publish.Chroma, s.Publish.Mode = "av1_qsv", "yuv420p", "cbr"
	s.Publish.BitrateM = qsvShortBitrateKbps / 1000
	if _, _, err := gstEncoder(s, 60, gpupath.MemorySystem); err != nil {
		t.Errorf("av1_qsv cbr at the property's highest rate: %v", err)
	}
	s.Publish.Mode, s.Publish.BitrateM = "crf", aboveBoundM
	if _, _, err := gstEncoder(s, 60, gpupath.MemorySystem); err != nil {
		t.Errorf("av1_qsv crf sends no bitrate, so the bound must not bind: %v", err)
	}
}

// The two engines drive one SVT-AV1 library through different bindings, so a
// stream's look must not depend on which capture backend produced it. The preset
// is the knob that decides that look, and both builders now take it off the codec's
// row, so what this holds is that neither of them spells a value of its own into the
// command.
//
// Both sides are read out of what was built rather than from a table, since a test that
// asked the row would agree with itself whatever the builders spend.
func TestSvtAv1PresetAgreesAcrossEngines(t *testing.T) {
	s := baseStream()
	s.Publish.Codec = "libsvtav1"
	s.Publish.Chroma = "yuv420p"
	s.Publish.Transport = "rtsp"
	s.Publish.Mode = "crf"
	s.Publish.Capture = "x11grab"
	// The steps this codec declares, which is what a draft naming it holds after the
	// migration or the repair. The defaults carry another encoder's, and both builders
	// refuse a step off the ladder rather than encoding at one the row never named.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)

	args, err := ffmpeg.BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	ffmpegPreset := ""
	for i, a := range args {
		if a == "-preset" && i+1 < len(args) {
			ffmpegPreset = args[i+1]
		}
	}
	if ffmpegPreset == "" {
		t.Fatalf("the ffmpeg libsvtav1 command carries no preset: %v", args)
	}

	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}
	gstPreset := ""
	for _, p := range encoder {
		if value, ok := strings.CutPrefix(p, "preset="); ok {
			gstPreset = value
		}
	}
	if gstPreset == "" {
		t.Fatalf("the svtav1enc element carries no preset: %v", encoder)
	}
	if ffmpegPreset != gstPreset {
		t.Errorf("ffmpeg encodes SVT-AV1 at preset %s, this engine at %s: one library, two looks",
			ffmpegPreset, gstPreset)
	}
}

// The GStreamer half of TestAbrAndVbrDifferWhereBothAreAllowed. abr aims at an
// average and vbr bounds the burst above it, so an element that takes no ceiling
// implements one of the two, and the table declares the other as a mode gap
// (gstNoRateCeiling).
//
// Every software element here was the failure this guards: x264enc, x265enc, vp8enc,
// vp9enc and av1enc all built one command for both modes, so a VBR publish ran as an
// uncapped average while the command and the estimate kept calling it VBR.
func TestAbrAndVbrDifferWhereBothAreAllowed(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		chromas := c.EngineChromas(EngineGst)
		if len(chromas) == 0 {
			continue
		}
		built := map[string]string{}
		for _, mode := range []string{capabilities.ModeAbr, capabilities.ModeVbr} {
			s := baseStream()
			s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = c.Name, mode, chromas[0]
			// A rate every element's property takes, so what this compares is the two
			// modes and not one codec's rate bound. The defaults sit above SVT-AV1's
			// ceiling and above what the qsv elements accept once abr doubles it.
			//
			// The ceiling is deliberately not twice the target. That is the value abr
			// derives for the families that coded against a maximum either way, so the
			// two modes agree there for a reason, and a fixture sitting on it would
			// report the agreement as a collapse.
			s.Publish.BitrateM, s.Publish.MaxrateM = 10, 15
			if capabilities.Validate(EngineGst, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM) != nil {
				continue
			}
			enc, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
			if err != nil {
				t.Fatalf("%s %s: %v", c.Name, mode, err)
			}
			built[mode] = strings.Join(enc, " ")
		}
		if len(built) == 2 && built[capabilities.ModeAbr] == built[capabilities.ModeVbr] {
			t.Errorf("%s builds one element for abr and vbr (%q), so one of the two modes is a name for the other",
				c.Name, built[capabilities.ModeAbr])
		}
	}
}
