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

// encodeTimeout bounds one test encode.
// Two frames at 320x240 return in well under a second on every element here,
// so what the bound catches is an element that takes the frames and emits nothing:
// svtav1enc does that in the low-delay structure, and a stall is a failure rather than a reason to
// wait.
const encodeTimeout = 20 * time.Second

// baseStream is the default draft with both ladder steps left unnamed.
//
// A step is one encoder's own identifier and these tests reach every codec off one draft,
// so the default codec's step would reach encoders that never heard of it,
// which the builder refuses as it refuses the default quantizer on a codec whose scale stops below
// it.
// An unnamed step resolves to the codec's own (capabilities.Ladder.Resolve),
// so each codec here encodes at the step its row names for the mode under test.
func baseStream() settings.Settings {
	s := settings.Defaults()
	// The default relay is the one on the internet, so its SRT leg is refused without the passphrase
	// that encrypts it, and a refused publish builds no pipeline for these tests to read.
	s.Relay.SrtPassphrase = "a-passphrase-long-enough"
	s.Publish.Effort, s.Publish.Tune = "", ""
	return s
}

// The mappings are a wire format shared with GStreamer: an element name has to exist,
// a property has to be spelled the way that element spells it, and a value has to sit in its range.
// No compiler holds any of that, and a wrong property is a pipeline that fails once a user hits
// Publish.
// So one real gst-launch runs per codec and mode, on videotestsrc rather than the portal node.
//
// The capability table drives the loop, which is what finds a codec added there without a mapping,
// or a mode declared reachable that the element has no property for.
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

		// The last format this engine reaches is the narrowest, 10-bit where the element takes it and
		// 4:2:0 where it does not, so it is the one most likely to fail negotiation.
		// A format gapped here is left out: pinning it would assert only that the element rejects what
		// capabilities.Validate already refuses.
		engineChromas := cap.EngineChromas("gstreamer")

		for _, mode := range []string{"cbr", "vbr", "abr", "crf", "lossless"} {
			if _, gap := cap.OptionGap("gstreamer", capabilities.OptionMode, mode); gap {
				continue
			}
			t.Run(name+"/"+mode, func(t *testing.T) {
				s := baseStream()
				s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = name, mode, engineChromas[len(engineChromas)-1]
				// The quantizer rides each encoder's own scale, and the defaults carry one off another
				// codec's.
				s.Publish.Cq = cap.CqMaxOn(EngineGst) / 2

				// System memory: what is under test is the properties an element takes,
				// and videotestsrc puts no device in the chain.
				// The device elements and the layouts they negotiate are covered where the conversion into
				// them is (TestPublishedColorimetryReachesTheDecoder).
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
				// gst-launch reports a bad property or an unset caps field on stderr and still exits zero for
				// some of them, so the output is read as well.
				if err != nil || strings.Contains(string(out), "no property") {
					t.Errorf("gst-launch %s: %v\n%s", strings.Join(args, " "), err, out)
				}
			})
		}
	}
}

// A codec the capability table declares implemented has to build on the engine that will be asked
// for it, or the portal capture backend fails at launch on a combination the form offered.
// A codec gapped off this engine is never asked for, Validate refusing it here: the AMF rows,
// whose plugin is Windows-only, and the Vulkan ones, whose encoder takes Vulkan device memory.
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

// The reverse: a mapping for a codec this engine is told it cannot run is code no pipeline reaches,
// and the gap's reason is then a lie.
func TestNoGstMappingForAGappedCodec(t *testing.T) {
	for name := range gstCodecs {
		c, ok := capabilities.Get(name)
		if !ok {
			continue // the missing row is TestGstEncodersAgainstGstLaunch's report
		}
		if gap, ok := c.EngineGap(EngineGst); ok {
			t.Errorf("codec %s has a GStreamer mapping and a gap saying it has none: %s", name, gap.Reason)
		}
	}
}

// The quantizer reaches every element on that element's own scale, so a codec counting to 255 takes
// no value clamped to 51, and the reverse.
// The scale is this engine's: qsvvp9enc passes VP9's own quantizer index through where the ffmpeg
// wrapper of the same silicon states a CQP on the H.26x scale.
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

// The va elements state a VBR target as a percentage of the ceiling and take 50 at the lowest,
// so a target under half its ceiling is a pair they have no form for.
// Coding it at the floor would run 100 Mbit/s where 20 was asked for,
// and the ffmpeg engine hands the same settings to the same hardware as -b:v 20M -maxrate 200M.
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

	// Half the ceiling is the lowest ratio the property expresses, so it builds,
	// and what it builds targets the rate the settings state.
	s.Publish.BitrateM = 100
	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(encoder, " "); !strings.Contains(line, "target-percentage=50") {
		t.Errorf("h264_vaapi vbr at half its ceiling: %s, want target-percentage=50", line)
	}
}

// Every bitrate mode can drive the property past the range it accepts: cbr and vbr with the figure
// the settings carry, abr with the ceiling it derives at twice the target.
// Each is refused, the alternative being a stream running at the bound instead of at the rate the
// form shows.
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

	// The bound is itself a rate the property accepts.
	s := baseStream()
	s.Publish.Codec, s.Publish.Chroma, s.Publish.Mode = "h264_vaapi", "yuv420p", "cbr"
	s.Publish.BitrateM = vaMaxBitrateKbps / 1000
	if _, _, err := gstEncoder(s, 60, gpupath.MemorySystem); err != nil {
		t.Errorf("h264_vaapi cbr at the property's highest rate: %v", err)
	}
}

// The AV1 and VP9 qsv elements hold their bitrate in an unsigned 16-bit property where the H.264
// and H.265 ones take any rate, so the bound sits on the row and not on the family.
// Each bitrate mode drives it past that range: cbr and vbr with the figure the settings carry,
// abr with the ceiling it derives at twice the target.
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

			// The same rate is inside an H.26x element's property range, so the bound must not reach it.
			s.Publish.Codec = "h264_qsv"
			if _, _, err := gstEncoder(s, 60, gpupath.MemorySystem); err != nil {
				t.Errorf("h264_qsv %s at %d Mbit/s: %v", tc.mode, tc.bitrateM, err)
			}
		})
	}

	// The bound is itself a rate the property accepts, and crf drives no rate at all.
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

// Two bindings of one SVT-AV1 library, so a stream's look must not follow from the capture backend
// that produced it.
// The preset decides that look and both builders take it off the codec's row,
// so what is held here is that neither spells a value of its own into the command.
//
// Both sides are read off what was built rather than off the row, since a test asking the row would
// agree with itself whatever the builders spend.
func TestSvtAv1PresetAgreesAcrossEngines(t *testing.T) {
	s := baseStream()
	s.Publish.Codec = "libsvtav1"
	s.Publish.Chroma = "yuv420p"
	s.Publish.Transport = "rtsp"
	s.Publish.Mode = "crf"
	s.Publish.Capture = "x11grab"
	// The codec's own steps, which is what a draft naming it holds after the migration or the repair.
	// The defaults carry another encoder's, and both builders refuse a step off the ladder rather than
	// encoding at one the row never named.
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

// The GStreamer half of the ffmpeg package's test of the same name.
// abr aims at an average and vbr bounds the burst above it, so an element taking no ceiling
// implements one of the two and the table declares the other a mode gap (gstNoRateCeiling).
//
// x264enc, x265enc, vp8enc, vp9enc and av1enc each built one command for both modes,
// so a VBR publish ran as an uncapped average while the command and the estimate called it VBR.
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
			// A rate every element's property takes, so what is compared is the two modes and not one
			// codec's rate bound: the defaults sit above SVT-AV1's ceiling and above what the qsv
			// elements accept once abr doubles it.
			//
			// The ceiling is not twice the target, which is the value abr derives for the families that
			// code against a maximum either way: the two modes agree there for a reason,
			// and a fixture sitting on it would read that agreement as a collapse.
			s.Publish.BitrateM, s.Publish.MaxrateM = 10, 15
			if capabilities.Validate(EngineGst, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM, s.Publish.Gop, capabilities.Device{}) != nil {
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

// A software preset ships a lookahead, and a lookahead is frames the viewer waits for: a leg
// draining slower than the capture paces drains them at its own rate, so the pin is what keeps a
// shortfall costing frames instead of seconds (gstLiveDelay).
// Read off the built encoder rather than off the table, a pin the mapping never appends being the
// same picture as a table with no row.
func TestTheLookaheadPinReachesEveryElementThatHoldsFrames(t *testing.T) {
	pinned := map[string][]string{
		"libx264":    {"sliced-threads=true", "option-string=rc-lookahead=0"},
		"libvpx":     {"lag-in-frames=0"},
		"libvpx-vp9": {"lag-in-frames=0"},
		"librav1e":   {"low-latency=true"},
		// x265 carries its own in the option-string every mode composes, which is why the key is read
		// and not the whole property.
		"libx265": {"rc-lookahead=0"},
	}
	for codec, want := range pinned {
		c, ok := capabilities.Get(codec)
		if !ok {
			t.Errorf("%s has a lookahead pin and no capability row", codec)
			continue
		}
		for _, mode := range capabilities.Modes {
			if !capabilities.Reaches(codec, EngineGst, capabilities.OptionMode, mode) {
				continue
			}
			s := baseStream()
			s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = codec, mode, c.EngineChromas(EngineGst)[0]
			s.Publish.Cq = c.CqMaxOn(EngineGst) / 2
			encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
			if err != nil {
				t.Fatalf("%s %s: %v", codec, mode, err)
			}
			line := strings.Join(encoder, " ")
			for _, pin := range want {
				if !strings.Contains(line, pin) {
					t.Errorf("%s %s: %s, want %s on it", codec, mode, line, pin)
				}
			}
		}
	}
}

// One property carries every libx265 knob, so the mode's own keys and the pins share it or one of
// the two is dropped: a second option-string on the same element is the first one overwritten.
func TestTheX265OptionStringCarriesTheModeKeysBesideThePins(t *testing.T) {
	s := baseStream()
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx265", "cbr", "yuv420p"
	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}

	var options []string
	for _, arg := range encoder {
		if strings.HasPrefix(arg, "option-string=") {
			options = append(options, arg)
		}
	}
	if len(options) != 1 {
		t.Fatalf("x265 cbr states %d option-strings (%v), want the one the element reads", len(options), options)
	}
	for _, key := range []string{"rc-lookahead=0", "vbv-maxrate=", "vbv-bufsize="} {
		if !strings.Contains(options[0], key) {
			t.Errorf("x265 cbr option-string %q carries no %s", options[0], key)
		}
	}
}

// A constant-quality encode spends what the picture costs, and the ceiling is what holds that inside
// a link's budget.
// Read off the built element rather than off the mapping: x264enc takes the ceiling on the same
// property a bitrate mode targets, so a branch that forgot it looks like every other branch.
func TestAStatedCeilingReachesAConstantQualityEncode(t *testing.T) {
	for _, codec := range []string{"libx264", "libx265"} {
		s := baseStream()
		s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = codec, "crf", "yuv420p"
		s.Publish.MaxrateM, s.Publish.VbvMs = 12, 0
		encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}

		line := strings.Join(encoder, " ")
		if !strings.Contains(line, "12000") {
			t.Errorf("%s crf under a 12 Mbit/s ceiling: %s, want the ceiling on it", codec, line)
		}
	}
}

// An encode with no ceiling has to state that it has none.
// x264enc reads its ceiling off the bitrate property, and both that and the buffer holding it carry
// a default, so an unbounded quality target is one that takes the buffer away: without it the encode
// is capped at 2 Mbit/s whatever quantizer it asked for.
func TestAnUnboundedConstantQualityEncodeTakesTheRateBufferAway(t *testing.T) {
	s := baseStream()
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx264", "crf", "yuv420p"
	s.Publish.MaxrateM = 0
	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(encoder, " ")
	if !strings.Contains(line, "vbv-buf-capacity=0") {
		t.Errorf("libx264 crf with no ceiling: %s, want the rate buffer off", line)
	}
	if strings.Contains(line, "bitrate=") {
		t.Errorf("libx264 crf with no ceiling: %s, want no rate on it", line)
	}
}

// x265's qp property is a fixed quantizer, which takes no VBV: a ceiling stated beside it would
// reach the element and bound nothing, and the same settings would code at one quality here and
// another on the ffmpeg engine.
func TestX265CodesConstantQualityAsARateFactor(t *testing.T) {
	s := baseStream()
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx265", "crf", "yuv420p"
	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(encoder, " ")
	if !strings.Contains(line, "crf="+strconv.Itoa(s.Publish.Cq)) {
		t.Errorf("libx265 crf: %s, want the rate factor on it", line)
	}
	if strings.Contains(line, "qp=") {
		t.Errorf("libx265 crf: %s, want no fixed quantizer on it", line)
	}
}

// libx265 takes a ceiling above its target, which is what constrained VBR is.
// The gap the other software rows carry states the opposite, so a mapping that stopped building this
// mode would leave the capability table promising one nothing implements.
func TestX265BuildsConstrainedVbrWithACeilingAboveTheTarget(t *testing.T) {
	s := baseStream()
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx265", "vbr", "yuv420p"
	s.Publish.BitrateM, s.Publish.MaxrateM, s.Publish.VbvMs = 10, 15, 0
	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(encoder, " ")
	for _, want := range []string{"bitrate=10000", "vbv-maxrate=15000"} {
		if !strings.Contains(line, want) {
			t.Errorf("libx265 vbr: %s, want %s on it", line, want)
		}
	}
}
