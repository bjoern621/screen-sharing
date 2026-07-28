package publish

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/ffmpeg"
	"bjoernblessin.de/screenshare/settings"
)

// encodeTimeout bounds one test encode. Two frames at 320x240 return in well under a
// second on every element here, so the only thing this catches is an element that
// takes the frames and emits nothing: svtav1enc does exactly that in the low-delay
// structure, and a stall is a failure like any other, not a reason to wait.
const encodeTimeout = 20 * time.Second

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
	if _, err := exec.LookPath(gstExe); err != nil {
		t.Skipf("%s not installed", gstExe)
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
			if _, gap := cap.ModeGap("gstreamer", mode); gap {
				continue
			}
			t.Run(name+"/"+mode, func(t *testing.T) {
				s := settings.Defaults()
				s.Codec, s.Mode, s.Chroma = name, mode, engineChromas[len(engineChromas)-1]
				// The quantizer target rides each encoder's own scale, and the default
				// settings carry one from another codec's.
				s.Cq = cap.CqMax / 2

				format, err := gstChromaFormat(name, s.Chroma)
				if err != nil {
					t.Fatal(err)
				}
				encoder, link, err := gstEncoder(s, 60)
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
				out, err := exec.CommandContext(ctx, gstExe, args...).CombinedOutput()
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
func TestGstEncoderQuantizerFollowsTheCodecScale(t *testing.T) {
	for _, name := range []string{"libx264", "libvpx-vp9", "librav1e"} {
		cap, _ := capabilities.Get(name)
		s := settings.Defaults()
		s.Codec, s.Mode, s.Chroma, s.Cq = name, "crf", cap.EngineChromas("gstreamer")[0], cap.CqMax
		encoder, _, err := gstEncoder(s, 60)
		if err != nil {
			t.Fatal(err)
		}
		want := strconv.Itoa(cap.CqMax)
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
	s := settings.Defaults()
	s.Codec, s.Chroma, s.Mode = "h264_vaapi", "yuv420p", "vbr"
	s.BitrateM, s.MaxrateM = 20, 200
	_, _, err := gstEncoder(s, 60)
	if err == nil {
		t.Fatalf("h264_vaapi vbr at %d/%d Mbit/s must be refused, not encoded at another rate", s.BitrateM, s.MaxrateM)
	}
	for _, want := range []string{"20 Mbit/s", "200 Mbit/s"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q names neither number, want %s in it", err, want)
		}
	}

	// Half the ceiling is the lowest ratio the property expresses, so it builds, and
	// what it builds targets what the settings state.
	s.BitrateM = 100
	encoder, _, err := gstEncoder(s, 60)
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
			s := settings.Defaults()
			s.Codec, s.Chroma, s.Mode = "h264_vaapi", "yuv420p", tc.mode
			s.BitrateM, s.MaxrateM = tc.bitrateM, tc.maxrateM
			_, _, err := gstEncoder(s, 60)
			if err == nil {
				t.Fatalf("h264_vaapi %s at %d Mbit/s must be refused, not clamped", tc.mode, tc.bitrateM)
			}
			if !strings.Contains(err.Error(), strconv.Itoa(vaMaxBitrateKbps)) {
				t.Errorf("the refusal %q does not name the %d kbit/s limit", err, vaMaxBitrateKbps)
			}
		})
	}

	// The bound itself is a rate the property takes.
	s := settings.Defaults()
	s.Codec, s.Chroma, s.Mode = "h264_vaapi", "yuv420p", "cbr"
	s.BitrateM = vaMaxBitrateKbps / 1000
	if _, _, err := gstEncoder(s, 60); err != nil {
		t.Errorf("h264_vaapi cbr at the property's highest rate: %v", err)
	}
}

// The two engines drive one SVT-AV1 library through different bindings, so a
// stream's look must not depend on which capture backend produced it. The preset
// is the knob that decides that look, and each engine states it in its own file,
// which a comment asks to keep equal and nothing enforced.
//
// The ffmpeg side is read out of the built command rather than from its constant,
// since the constant is unexported to this package and what matters is the value
// that reaches the encoder either way.
func TestSvtAv1PresetAgreesAcrossEngines(t *testing.T) {
	s := settings.Defaults()
	s.Codec = "libsvtav1"
	s.Chroma = "yuv420p"
	s.Transport = "rtsp"
	s.Mode = "crf"
	s.Capture = "x11grab"

	args, err := ffmpeg.BuildPublishArgs(s)
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
	if ffmpegPreset != svtav1Preset {
		t.Errorf("ffmpeg encodes SVT-AV1 at preset %s, this engine at %s: one library, two looks",
			ffmpegPreset, svtav1Preset)
	}
}
