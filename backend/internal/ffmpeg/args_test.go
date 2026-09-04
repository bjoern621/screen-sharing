package ffmpeg

import (
	"context"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// encodeTimeout bounds one test encode.
// A single 256x256 frame returns in well under a second on every encoder here,
// so what the bound catches is a library that takes the frame and emits nothing.
const encodeTimeout = 20 * time.Second

func baseStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			// A name rather than an address, so this publishes to an encrypted relay:
			// these tests are about what the encoder is asked for,
			// and a publish that is refused builds no arguments to read.
			Host:    "relay.example",
			SrtPort: 8890,
			// A publish lives in a group, so a keyless fixture is one every transport refuses.
			GroupKey: group.Key(make([]byte, group.KeyBytes)).String(),
		},
		Publish: settings.Publish{
			Transport:  "srt",
			Format:     "h264",
			Encoder:    "x264",
			Mode:       "crf",
			Chroma:     "yuv444p",
			ColorRange: "pc",
			Fps:        60,
			Cq:         19,
			BitrateM:   150,
			MaxrateM:   200,
			Capture:    "x11grab",
			// No ladder step: every codec below is reached off this one draft,
			// and a step is one encoder's own identifier,
			// so naming one would carry another encoder's step into most of them.
			// An unnamed step is the codec's own declared one, which the builder resolves it to.
			DrmMap:        "auto",
			CaptureMemory: gpupath.MemoryAuto,
			// Audio is off, so only the cases that turn it on read the codec.
			// It is filled the way migrateStream fills it, since the builder validates the codec
			// of every stream naming a source.
			AudioCodec: "opus",
		},
	}
}

// flagValue returns the argument after flag, "" where flag is absent.
func flagValue(args []string, flag string) string {
	i := slices.Index(args, flag)
	if i < 0 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}

func TestBuildPublishArgsUnknownTransport(t *testing.T) {
	s := baseStream()
	s.Publish.Transport = "carrier-pigeon"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestBuildPublishArgsUnknownCapture(t *testing.T) {
	s := baseStream()
	s.Publish.Capture = "telepathy"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for unknown capture backend")
	}
}

// A non-positive rate would reach ffmpeg as "-framerate 0" and "-g 0":
// every grabber takes the rate as an input option and the keyframe interval derives from it.
func TestBuildPublishArgsRefusesANonPositiveFps(t *testing.T) {
	for _, fps := range []int{0, -1} {
		s := baseStream()
		s.Publish.Fps = fps
		if _, err := BuildPublishArgs(s, nil); err == nil {
			t.Errorf("fps %d was accepted", fps)
		}
	}
}

// An index no output carries names a screen this machine does not have,
// and grabbing the whole desktop instead would publish something other than what the form shows
// selected, with nothing saying so.
func TestX11grabRefusesAMonitorIndexNoOutputCarries(t *testing.T) {
	s := baseStream()
	s.Publish.Monitor = 9999
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("a monitor index no output carries was accepted")
	}
}

// x11grab reads an X screen, and an environment naming none is no session to capture.
// A ":0.0" fallback would capture whichever display answered.
func TestX11grabRefusesAnUnsetDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	s := baseStream()
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("an unset DISPLAY was accepted")
	}
}

// A DRM download strategy overrides the driver's guess, so a name no row carries must not resolve
// back to that guess: the setting would run as its own opposite.
// The refusal is the table's rather than the machine's, so it holds with no DRM node present.
func TestDrmMapForRefusesANameNoRowCarries(t *testing.T) {
	if _, err := drmMapFor("vaapi-with-a-typo"); err == nil {
		t.Error("an unmapped DRM download strategy resolved")
	}
	for _, m := range DrmMaps {
		if _, err := drmMapFor(m.Name); err != nil {
			t.Errorf("%s: %v", m.Name, err)
		}
	}
}

// Nothing upstream bounds the effort step: it is a free-form settings string,
// and capabilities.Validate reaches the codec, the pixel format, the mode
// and the two rate figures only.
func TestAStepOutsideTheLadderIsRefused(t *testing.T) {
	// The empty string is absent from the cases: it resolves to the codec's own declared step,
	// which a draft holding none is entitled to.
	// A step off the ladder is refused, another encoder's ladder included,
	// as a draft that changed codec holds.
	for _, tc := range []struct{ codec, step string }{
		{"hevc_nvenc", "p8"},
		{"hevc_nvenc", "slow"},
		{"libx264", "p7"},
		{"libsvtav1", "14"},
	} {
		s := baseStream()
		s.Publish.UseCodec(tc.codec)
		s.Publish.Chroma, s.Publish.Effort = "yuv420p", tc.step
		if _, err := encoderArgs(s, gopFor(s)); err == nil {
			t.Errorf("%s took the step %q", tc.codec, tc.step)
		}
	}
	// A step the codec's own row declares builds a command.
	for _, codec := range []string{"hevc_nvenc", "libx264", "libsvtav1"} {
		c, ok := capabilities.Get(codec)
		if !ok {
			t.Fatalf("no capability row for %s", codec)
		}
		for _, step := range c.Effort.Steps {
			s := baseStream()
			s.Publish.UseCodec(codec)
			s.Publish.Chroma, s.Publish.Effort = "yuv420p", step
			s.Publish.Tune, _ = c.Tune.StepFor(s.Publish.Mode)
			if _, err := encoderArgs(s, gopFor(s)); err != nil {
				t.Errorf("%s step %q: %v", codec, step, err)
			}
		}
	}
}

// A mode that pins the step spends the pinned one whatever the draft holds,
// and the form greys the control and names that step.
// Both read the codec's row, so the step encoded and the step named cannot come apart.
func TestNvencCbrPinsTheDeclaredStep(t *testing.T) {
	c, ok := capabilities.Get("hevc_nvenc")
	if !ok {
		t.Fatal("no capability row for hevc_nvenc")
	}
	want, declared := c.Effort.StepFor("cbr")
	if !declared || !c.Effort.PinsIn("cbr") {
		t.Fatalf("hevc_nvenc pins no declared step in cbr")
	}

	s := baseStream()
	s.Publish.UseCodec("hevc_nvenc")
	s.Publish.Chroma, s.Publish.Mode, s.Publish.Effort = "yuv420p", "cbr", "p7"
	args, err := encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatal(err)
	}
	if got := flagValue(args, "-preset"); got != want {
		t.Errorf("-preset = %q, want the pinned %q", got, want)
	}
}

func TestBuildPublishArgsColorRange(t *testing.T) {
	s := baseStream()
	s.Publish.Chroma = "yuv444p"
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := flagValue(args, "-pix_fmt"); got != "yuv444p" {
		t.Errorf("-pix_fmt = %q, want yuv444p", got)
	}
	if got := flagValue(args, "-color_range"); got != "pc" {
		t.Errorf("-color_range = %q, want pc", got)
	}

	// gbrp is full range by construction, so no -color_range is written.
	// The codec changes with the chroma, since not every row declares gbrp.
	s.Publish.UseCodec("hevc_nvenc")
	s.Publish.Chroma = "gbrp"
	args, err = BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "-color_range") {
		t.Errorf("gbrp must not emit -color_range, got %v", args)
	}
}

func TestBuildPublishArgsGop(t *testing.T) {
	s := baseStream()

	s.Publish.Gop = 0
	s.Publish.Fps = 60
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := flagValue(args, "-g"); got != "120" {
		t.Errorf("auto -g = %q, want 120 (2*fps)", got)
	}

	s.Publish.Gop = 45
	args, err = BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := flagValue(args, "-g"); got != "45" {
		t.Errorf("explicit -g = %q, want 45", got)
	}
}

func TestEncoderArgs(t *testing.T) {
	tests := []struct {
		name   string
		codec  string
		mode   string
		want   string // substring the joined args must hold
		reject string // substring they must not, "" to skip
	}{
		{"x264 crf uses crf", "libx264", "crf", "-crf", "zerolatency"},
		{"x264 cbr tunes zerolatency", "libx264", "cbr", "zerolatency", "-crf"},
		{"x264 cbr bounds bitrate", "libx264", "cbr", "-maxrate", ""},
		{"x264 abr is uncapped", "libx264", "abr", "-b:v", "-maxrate"},
		{"x264 vbr sets a ceiling", "libx264", "vbr", "-maxrate", "zerolatency"},
		{"x265 crf uses crf", "libx265", "crf", "-crf", "zerolatency"},
		{"x265 lossless sets x265-params", "libx265", "lossless", "lossless=1", "-qp"},
		{"x265 cbr bounds bitrate", "libx265", "cbr", "-maxrate", "-crf"},
		{"x265 abr is uncapped", "libx265", "abr", "-b:v", "-maxrate"},
		{"nvenc lossless tunes lossless", "hevc_nvenc", "lossless", "lossless", ""},
		{"nvenc crf uses cq", "hevc_nvenc", "crf", "-cq", ""},
		{"nvenc cbr uses cbr", "hevc_nvenc", "cbr", "-rc cbr", ""},
		{"nvenc abr uses vbr", "hevc_nvenc", "abr", "-rc vbr", "-maxrate"},
		{"nvenc vbr sets a ceiling", "hevc_nvenc", "vbr", "-maxrate", ""},
		{"vp9 uses realtime screen tuning", "libvpx-vp9", "cbr", "-tune-content screen", ""},
		{"vp9 crf is constant quality", "libvpx-vp9", "crf", "-crf", "-minrate"},
		{"vp9 lossless sets lossless", "libvpx-vp9", "lossless", "-lossless 1", "-crf"},
		{"vp9 cbr pins the rate", "libvpx-vp9", "cbr", "-minrate", "-crf"},
		// VP8 spells its screen coding tools as a libvpx option of its own rather than as VP9's
		// tune-content, and it has neither row threading nor a lossless mode.
		{"vp8 tunes for screen content", "libvpx", "cbr", "-screen-content-mode 1", "-tune-content"},
		{"vp8 crf is constant quality", "libvpx", "crf", "-crf", "-minrate"},
		{"vp8 cbr pins the rate", "libvpx", "cbr", "-minrate", "-crf"},
		{"aom av1 encodes realtime", "libaom-av1", "cbr", "-usage realtime", ""},
		{"aom av1 crf is constant quality", "libaom-av1", "crf", "-crf", "-minrate"},
		// SVT-AV1 refuses a ceiling outside constant-quality mode, so the table gaps vbr on both engines
		// and abr sends the target alone.
		// Its CBR is selected inside -svtav1-params and nowhere else.
		{"svt av1 abr holds no ceiling", "libsvtav1", "abr", "-b:v", "-maxrate"},
		{"svt av1 cbr selects rate control 2", "libsvtav1", "cbr", "rc=2:pred-struct=1", "-maxrate"},
		{"svt av1 crf takes no bitrate", "libsvtav1", "crf", "-crf", "-b:v"},
		// rav1e takes one bitrate target and neither a ceiling nor a rate buffer,
		// so vbr is gapped on both engines and abr is the bursting mode it implements.
		{"rav1e crf uses qp", "librav1e", "crf", "-qp", "-crf"},
		{"rav1e abr holds no ceiling", "librav1e", "abr", "-b:v", "-maxrate"},
		{"rav1e cbr drops reordering", "librav1e", "cbr", "low_latency=true", "-bufsize"},
		// A VAAPI encoder takes one -rc_mode per rate-control concept,
		// and its quantizer travels in -qp on the H.26x rows and in -global_quality elsewhere.
		{"vaapi crf is CQP", "h264_vaapi", "crf", "-rc_mode CQP -qp", "-b:v"},
		{"vaapi av1 quantizer is global_quality", "av1_vaapi", "crf", "-global_quality", "-qp"},
		{"vaapi cbr pins the ceiling to the target", "h264_vaapi", "cbr", "-rc_mode CBR", "-rc_mode VBR"},
		{"vaapi abr gives no ceiling", "h264_vaapi", "abr", "-rc_mode VBR", "-maxrate"},
		{"vaapi vbr sets a ceiling", "hevc_vaapi", "vbr", "-maxrate", "-rc_mode CBR"},
		// A QSV encoder has no rate-control option: oneVPL picks the method from which rate options carry
		// a value, so a mode is a shape rather than a name.
		// A quantizer under the qscale flag selects CQP,
		// a ceiling equal to the target CBR, and one above it VBR.
		{"qsv crf carries a quantizer alone", "h264_qsv", "crf", "-q:v", "-b:v"},
		{"qsv cbr pins the ceiling to the target", "h264_qsv", "cbr", "-b:v 150M -maxrate 150M", ""},
		{"qsv abr caps its ceiling above the target", "hevc_qsv", "abr", "-b:v 150M -maxrate 300M", "-bufsize"},
		{"qsv vbr takes the configured ceiling", "hevc_qsv", "vbr", "-maxrate 200M", ""},
		// cbr trades the encoder's pipeline depth for delay, and no other mode touches it.
		{"qsv cbr shortens the pipeline", "av1_qsv", "cbr", "-async_depth 1", ""},
		{"qsv vbr keeps the default pipeline", "av1_qsv", "vbr", "-b:v", "-async_depth"},
		{"qsv cbr encodes for speed", "h264_qsv", "cbr", "-preset veryfast", "-preset medium"},
		{"qsv crf encodes for quality", "h264_qsv", "crf", "-preset medium", "-preset veryfast"},
		{"qsv pins b-pictures off", "hevc_qsv", "vbr", "-bf 0", ""},
		// An AMF encoder takes one -rc mode per rate-control concept,
		// and its bursting modes are peak-constrained VBR, so abr states a ceiling too.
		{"amf crf is cqp", "h264_amf", "crf", "-rc cqp -qp_i", "-b:v"},
		{"amf cbr pins the ceiling to the target", "h264_amf", "cbr", "-rc cbr", "vbr_peak"},
		{"amf abr caps its peak above the target", "hevc_amf", "abr", "-rc vbr_peak -b:v 150M -maxrate 300M", ""},
		{"amf vbr takes the configured ceiling", "hevc_amf", "vbr", "-maxrate 200M", "-rc cbr"},
		// AMF's low-latency usage presets drop the H.264 IDR period, so no mode selects one.
		// cbr says it is live through the quality scale instead.
		{"amf cbr keeps the transcoding usage", "h264_amf", "cbr", "-usage transcoding", "lowlatency"},
		{"amf cbr encodes for speed", "h264_amf", "cbr", "-quality speed", "-quality quality"},
		{"amf vbr encodes for quality", "h264_amf", "vbr", "-quality quality", "-quality speed"},
		// AMF's H.264 and AV1 encoders carry a B-picture pattern and its HEVC one does not,
		// so the option switching them off must not reach the HEVC row.
		{"amf h264 pins b-frames off", "h264_amf", "vbr", "-bf 0", ""},
		{"amf av1 pins b-frames off", "av1_amf", "vbr", "-bf 0", ""},
		{"amf hevc has no b-frame option", "hevc_amf", "vbr", "-rc vbr_peak", "-bf"},
		// AMF's H.264 encoder is the one that has to be told to repeat its parameter sets, once per GOP.
		// baseStream runs 60 fps with no explicit interval, so the automatic two seconds is 120 frames.
		{"amf h264 repeats its parameter sets per gop", "h264_amf", "cbr", "-header_spacing 120", ""},
		{"amf hevc needs no header spacing", "hevc_amf", "cbr", "-rc cbr", "-header_spacing"},
		{"amf av1 has no header spacing option", "av1_amf", "cbr", "-rc cbr", "-header_spacing"},
		// A Vulkan encoder takes one -rc_mode per rate-control concept
		// and codes a bursting mode against a ceiling either way, so abr states one too.
		{"vulkan crf is cqp", "h264_vulkan", "crf", "-rc_mode cqp -qp", "-b:v"},
		{"vulkan cbr pins the ceiling to the target", "h264_vulkan", "cbr", "-rc_mode cbr -b:v 150M -maxrate 150M", ""},
		{"vulkan abr caps its ceiling above the target", "hevc_vulkan", "abr", "-rc_mode vbr -b:v 150M -maxrate 300M", ""},
		{"vulkan vbr takes the configured ceiling", "hevc_vulkan", "vbr", "-maxrate 200M", "-rc_mode cbr"},
		// Every mode declares a live stream of screen content,
		// and cbr is the one trading quality to keep up with it.
		{"vulkan states its content type", "av1_vulkan", "cbr", "-usage stream -content desktop", ""},
		{"vulkan cbr tunes for latency", "h264_vulkan", "cbr", "-tune ll", "-tune hq"},
		{"vulkan vbr tunes for quality", "h264_vulkan", "vbr", "-tune hq", "-tune ll"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseStream()
			s.Publish.UseCodec(tc.codec)
			s.Publish.Mode = tc.mode
			args, err := encoderArgs(s, gopFor(s))
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, tc.want) {
				t.Errorf("encoderArgs(%s,%s) = %q, missing %q", tc.codec, tc.mode, joined, tc.want)
			}
			if tc.reject != "" && strings.Contains(joined, tc.reject) {
				t.Errorf("encoderArgs(%s,%s) = %q, must not contain %q", tc.codec, tc.mode, joined, tc.reject)
			}
		})
	}
}

// The rate buffer is SVT-AV1's CBR knob alone: buf-sz rides in -svtav1-params,
// which no other mode sends, so a bursting mode has nothing to size
// and the window the settings carry stops at the builder.
// The form greys the field there for that reason (form.availabilityEngineRules),
// and a value reaching the command in abr would make the greying a lie.
func TestSvtAv1SizesARateBufferInCbrOnly(t *testing.T) {
	s := baseStream()
	s.Publish.UseCodec("libsvtav1")
	s.Publish.VbvMs = 500

	s.Publish.Mode = "abr"
	args, err := encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); strings.Contains(joined, "buf-sz") {
		t.Errorf("libsvtav1 abr = %q, must carry no rate buffer", joined)
	}

	s.Publish.Mode = "cbr"
	args, err = encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); !strings.Contains(joined, "buf-sz=500") {
		t.Errorf("libsvtav1 cbr = %q, want buf-sz=500", joined)
	}
}

// VP9 splits its profiles by subsampling and bit depth, and libvpx refuses a pixel format
// the selected profile cannot carry, so the profile follows the chroma
// or the row's other chromas stop encoding.
func TestVp9ProfileFollowsChroma(t *testing.T) {
	want := map[string]string{
		"yuv420p": "0",
		"yuv444p": "1",
		"gbrp":    "1",
		"p010le":  "2",
	}
	for chroma, profile := range want {
		s := baseStream()
		s.Publish.UseCodec("libvpx-vp9")
		s.Publish.Chroma = chroma
		args, err := encoderArgs(s, gopFor(s))
		if err != nil {
			t.Fatal(err)
		}
		if got := flagValue(args, "-profile:v"); got != profile {
			t.Errorf("libvpx-vp9 at %s: -profile:v = %q, want %q", chroma, got, profile)
		}
	}
}

// AMF announces the Main profile whatever surface it is handed,
// so a 10-bit encode would ship a Main-profile bitstream carrying 10-bit samples.
// The profile follows the chroma there for that reason.
func TestAmfHevcProfileFollowsChroma(t *testing.T) {
	want := map[string]string{
		"yuv420p": "main",
		"p010le":  "main10",
	}
	for chroma, profile := range want {
		s := baseStream()
		s.Publish.UseCodec("hevc_amf")
		s.Publish.Chroma = chroma
		args, err := encoderArgs(s, gopFor(s))
		if err != nil {
			t.Fatal(err)
		}
		if got := flagValue(args, "-profile"); got != profile {
			t.Errorf("hevc_amf at %s: -profile = %q, want %q", chroma, got, profile)
		}
	}
}

// The encoder arguments are a wire format shared with ffmpeg and no compiler holds any of it:
// an option that does not exist, a value out of range or a rate-control combination the library
// refuses is a publish that dies on launch.
// So one frame is encoded per codec and mode, with the arguments the builder produces.
//
// The capability table drives the loop, so a codec added there with no mapping,
// or a mode declared reachable that the library rejects, fails here.
func TestEncoderArgsAgainstFfmpeg(t *testing.T) {
	exe, err := FindExe("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	for _, cap := range capabilities.Codecs {
		if !cap.Implemented {
			continue
		}
		// A surface encode's arguments run behind the device and the upload filter the publish command
		// puts in front of them, and nowhere else.
		device, surface, err := HwSurfaceDevice(cap.Name)
		if err != nil {
			t.Logf("%s: %v", cap.Name, err)
			continue
		}
		for _, mode := range []string{"cbr", "vbr", "abr", "crf", "lossless"} {
			if _, gap := cap.OptionGap("ffmpeg", capabilities.OptionMode, mode); gap {
				continue
			}
			t.Run(cap.Name+"/"+mode, func(t *testing.T) {
				s := baseStream()
				engineChromas := cap.EngineChromas("ffmpeg")
				s.Publish.UseCodec(cap.Name)
				s.Publish.Mode, s.Publish.Chroma = mode, engineChromas[len(engineChromas)-1]
				// A quantizer target rides its own encoder's scale and a bitrate target meets the row's
				// ceiling, so both are taken off the row here.
				// baseStream holds another codec's figures, the way saved settings do before normalize runs.
				s.Publish.Cq = cap.CqMaxOn(capabilities.EngineFfmpeg) / 2
				if limit := cap.BitrateLimitOn(capabilities.EngineFfmpeg); limit > 0 && s.Publish.BitrateM > limit {
					s.Publish.BitrateM = limit
				}
				enc, err := encoderArgs(s, gopFor(s))
				if err != nil {
					t.Fatal(err)
				}

				args := []string{"-hide_banner", "-loglevel", "error"}
				args = append(args, device...)
				args = append(args, "-f", "lavfi", "-i", "nullsrc=s=256x256", "-frames:v", "1")
				if surface {
					upload, err := HwSurfaceFilters(s.Publish.Codec(), s.Publish.Chroma)
					if err != nil {
						t.Fatal(err)
					}
					args = append(args, "-vf", strings.Join(upload, ","))
				}
				args = append(args, enc...)
				// A surface encode takes its layout from the upload filter
				// and its encoder reads no software pixel format, as in BuildPublishArgs.
				if !surface {
					args = append(args, "-pix_fmt", s.Publish.Chroma)
				}
				args = append(args, "-f", "null", "-")

				ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
				defer cancel()
				out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
				if err != nil {
					// An absent hardware encoder is the machine's answer rather than the builder's,
					// and what encoders.Detect greys in the UI.
					// A software encoder is the build's answer and ships in any ffmpeg worth publishing with,
					// so a failure there is the arguments.
					if cap.Family == "software" {
						t.Errorf("ffmpeg %s: %v\n%s", strings.Join(args, " "), err, out)
					}
					t.Skipf("%s does not run on this machine", cap.Name)
				}
			})
		}
	}
}

// A VAAPI publish opens its device ahead of the input, ends the capture chain in the upload
// and pins no software pixel format, the encoder reading GPU surfaces.
// The device is real hardware, so the shape is asserted only where one exists.
// The chroma mapping is HwSurfaceFilters' and is checked without a device (TestHwSurfaceFilters).
func TestBuildPublishArgsVaapi(t *testing.T) {
	if _, err := VaapiDevice(); err != nil {
		t.Skip("no VAAPI render node on this machine")
	}

	s := baseStream()
	s.Publish.UseCodec("h264_vaapi")
	s.Publish.Chroma = "yuv420p"
	s.Publish.Mode = "cbr"
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if i, j := slices.Index(args, "-vaapi_device"), slices.Index(args, "-i"); i < 0 || i > j {
		t.Errorf("-vaapi_device must open before the input, got %v", args)
	}
	if vf := flagValue(args, "-vf"); !strings.HasSuffix(vf, "format=nv12,hwupload") {
		t.Errorf("-vf = %q, want it to end in format=nv12,hwupload", vf)
	}
	if slices.Contains(args, "-pix_fmt") {
		t.Errorf("a VAAPI encode must not pin a software pixel format, got %v", args)
	}
	if got := flagValue(args, "-color_range"); got != "pc" {
		t.Errorf("-color_range = %q, want pc", got)
	}
}

// A Vulkan publish takes the same shape with a device of its own:
// created under a name ahead of the input, then handed to the filter graph the upload attaches to.
func TestBuildPublishArgsVulkan(t *testing.T) {
	s := baseStream()
	s.Publish.UseCodec("h264_vulkan")
	s.Publish.Chroma = "yuv420p"
	s.Publish.Mode = "cbr"
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if i, j := slices.Index(args, "-init_hw_device"), slices.Index(args, "-i"); i < 0 || i > j {
		t.Errorf("-init_hw_device must create the device before the input, got %v", args)
	}
	if got := flagValue(args, "-filter_hw_device"); got != flagValue(args, "-init_hw_device")[len("vulkan="):] {
		t.Errorf("-filter_hw_device = %q, want the name -init_hw_device registered", got)
	}
	if vf := flagValue(args, "-vf"); !strings.HasSuffix(vf, "format=nv12,hwupload") {
		t.Errorf("-vf = %q, want it to end in format=nv12,hwupload", vf)
	}
	if slices.Contains(args, "-pix_fmt") {
		t.Errorf("a Vulkan encode must not pin a software pixel format, got %v", args)
	}
}

// A QSV publish creates its device under a name, as Vulkan does,
// and uploads through a frame pool it sizes itself.
func TestBuildPublishArgsQsv(t *testing.T) {
	s := baseStream()
	s.Publish.UseCodec("hevc_qsv")
	s.Publish.Chroma = "p010le"
	s.Publish.Mode = "cbr"
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if i, j := slices.Index(args, "-init_hw_device"), slices.Index(args, "-i"); i < 0 || i > j {
		t.Errorf("-init_hw_device must create the device before the input, got %v", args)
	}
	if got := flagValue(args, "-filter_hw_device"); got != qsvDeviceAlias {
		t.Errorf("-filter_hw_device = %q, want the name -init_hw_device registered", got)
	}
	if vf := flagValue(args, "-vf"); !strings.HasSuffix(vf, "format=p010,hwupload=extra_hw_frames="+qsvExtraFrames) {
		t.Errorf("-vf = %q, want it to end in the p010 conversion and the QSV upload", vf)
	}
	if slices.Contains(args, "-pix_fmt") {
		t.Errorf("a QSV encode must not pin a software pixel format, got %v", args)
	}
}

func TestHwSurfaceFilters(t *testing.T) {
	cases := map[string]string{
		"yuv420p": "format=nv12",
		"p010le":  "format=p010",
	}
	for chroma, want := range cases {
		got, err := HwSurfaceFilters("h264_vaapi", chroma)
		if err != nil {
			t.Fatalf("HwSurfaceFilters(%q): %v", chroma, err)
		}
		if len(got) != 2 || got[0] != want || got[1] != "hwupload" {
			t.Errorf("HwSurfaceFilters(%q) = %v, want [%s hwupload]", chroma, got, want)
		}
	}
	// A chroma no surface family stores is a builder error rather than a silent conversion.
	if _, err := HwSurfaceFilters("h264_vaapi", "yuv444p"); err == nil {
		t.Error("HwSurfaceFilters must reject a chroma with no hardware surface layout")
	}
}

// The upload element follows the family rather than the chroma:
// a QSV encoder holds several surfaces out of the pool the upload allocated,
// so that pool is sized past what the filter graph needs.
func TestHwSurfaceUploadFollowsTheFamily(t *testing.T) {
	cases := map[string]string{
		"h264_vaapi":  "hwupload",
		"h264_vulkan": "hwupload",
		"h264_qsv":    "hwupload=extra_hw_frames=" + qsvExtraFrames,
	}
	for codec, want := range cases {
		got, err := HwSurfaceFilters(codec, "yuv420p")
		if err != nil {
			t.Fatalf("HwSurfaceFilters(%q): %v", codec, err)
		}
		if got[len(got)-1] != want {
			t.Errorf("HwSurfaceFilters(%q) uploads with %q, want %q", codec, got[len(got)-1], want)
		}
	}
}

// A family whose encoder reads GPU surfaces opens a device and one that reads system memory must
// not: the device option builds a filter graph a software encoder cannot take frames from.
func TestHwSurfaceDeviceFollowsTheFamily(t *testing.T) {
	cases := map[string]bool{
		"h264_vaapi":  true,
		"h264_vulkan": true,
		"av1_vulkan":  true,
		"h264_qsv":    true,
		"vp9_qsv":     true,
		"libx264":     false,
		"hevc_nvenc":  false,
		"h264_amf":    false,
	}
	for codec, want := range cases {
		// The device is real hardware, so only the verdict is asserted:
		// an error says the family is absent from this machine rather than from the table.
		_, got, _ := HwSurfaceDevice(codec)
		if got != want {
			t.Errorf("HwSurfaceDevice(%q) surface = %v, want %v", codec, got, want)
		}
	}
	if _, _, err := HwSurfaceDevice("nope"); err == nil {
		t.Error("HwSurfaceDevice must reject a codec the table does not carry")
	}
}

// The audio track is coded by the element capabilities.AudioCodecs states for this engine,
// so the command is held against that row rather than against a name spelled here.
// The two engines code one codec with different libraries, and a name written into the test
// would assert one engine's answer for both.
func TestBuildPublishArgsAudio(t *testing.T) {
	// Desktop audio on a Linux backend: the pulse monitor input,
	// and the encode the codec's row declares.
	// SRT carries the audio codecs the table holds, so the transport narrows this loop by nothing.
	for _, a := range capabilities.AudioCodecs {
		enc, ok := a.EncoderOn(capabilities.EngineFfmpeg)
		if !ok {
			continue
		}
		s := baseStream()
		s.Publish.AudioCodec = a.Name
		s.Publish.AudioSources = settings.Recording("desktop")
		args, err := BuildPublishArgs(s, nil)
		if err != nil {
			t.Fatalf("%s: %v", a.Name, err)
		}
		// The first -i is the video capture, so membership is what this can ask.
		if !slices.Contains(args, "pulse") || !slices.Contains(args, platform.AudioMonitorDevice) {
			t.Errorf("missing pulse monitor input, got %v", args)
		}
		if got := flagValue(args, "-c:a"); got != enc.Element {
			t.Errorf("%s: -c:a = %q, want the element the table states, %q", a.Name, got, enc.Element)
		}
		// The rate and the bitrate follow the codec too: an encoder that codes at one rate alone
		// is handed another rate's samples otherwise.
		if got := flagValue(args, "-ar"); got != strconv.Itoa(a.Rate) {
			t.Errorf("%s: -ar = %q, want %d", a.Name, got, a.Rate)
		}
		if got := flagValue(args, "-b:a"); got != strconv.Itoa(a.BitrateK)+"k" {
			t.Errorf("%s: -b:a = %q, want %dk", a.Name, got, a.BitrateK)
		}
	}

	// Audio off, and a settings file that predates the option: no audio arguments,
	// whatever codec the settings carry.
	for _, audio := range []string{"none", ""} {
		s := baseStream()
		s.Publish.AudioSources = settings.Recording(audio)
		args, err := BuildPublishArgs(s, nil)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(args, "pulse") || slices.Contains(args, "-c:a") {
			t.Errorf("audio %q must not emit audio args, got %v", audio, args)
		}
	}

	// A backend whose platform serves no monitor source refuses desktop audio rather than publishing
	// a silent track, and that refusal is publish's rather than this builder's:
	// the source table answers it and publish holds the backend's operating system.
	// It is asserted there, against the engine entry points a run
	// and the displayed command both go
	// through (TestABackendWhosePlatformServesNoMonitorSourceRefusesDesktopAudio).

	s := baseStream()
	s.Publish.AudioSources = settings.Recording("microphone")
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for an unknown audio source")
	}

	// An audio codec no row carries is refused before a command is built:
	// the encoder name would be read off a row that is not there.
	s = baseStream()
	s.Publish.AudioCodec = "mp3"
	s.Publish.AudioSources = settings.Recording("desktop")
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for an audio codec the table does not carry")
	}

	// A codec the publish leg cannot carry is the transport's refusal and this engine has to make it:
	// WebRTC negotiates Opus and no AAC.
	s = baseStream()
	s.Publish.UseCodec("libx264")
	s.Publish.Transport, s.Publish.Chroma = "webrtc", "yuv420p"
	s.Publish.AudioCodec = "aac"
	s.Publish.AudioSources = settings.Recording("desktop")
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for an AAC track over webrtc")
	}
}

func TestBuildPublishArgsIncompatibleCodec(t *testing.T) {
	// libx264 encodes no gbrp, and the capability check is what refuses it.
	s := baseStream()
	s.Publish.UseCodec("libx264")
	s.Publish.Chroma = "gbrp"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for libx264 + gbrp")
	}

	// SRT carries no AV1.
	s = baseStream()
	s.Publish.UseCodec("av1_nvenc")
	s.Publish.Chroma = "yuv420p"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for av1_nvenc over srt")
	}

	// MPEG-TS has no VP9 mapping, so SRT carries none either.
	s = baseStream()
	s.Publish.UseCodec("libvpx-vp9")
	s.Publish.Chroma = "yuv444p"
	s.Publish.Transport = "srt"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for libvpx-vp9 over srt")
	}
}

// Two rate-control modes the table allows on one codec have to build two commands.
// abr aims at an average and vbr bounds the burst above it, so an encoder that cannot bound
// the burst implements one of them and the table gaps the other.
//
// Identical arguments for both is the failure this guards: the encode runs as whichever mode
// the builder collapsed onto, while the command, the bitrate estimate
// and every verdict downstream keep naming the mode that was picked.
func TestAbrAndVbrDifferWhereBothAreAllowed(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		built := map[string]string{}
		for _, mode := range []string{capabilities.ModeAbr, capabilities.ModeVbr} {
			s := baseStream()
			s.Publish.UseCodec(c.Name)
			s.Publish.Mode, s.Publish.Chroma = mode, c.EngineChromas(capabilities.EngineFfmpeg)[0]
			// A rate every encoder takes, so the comparison is between the two modes rather
			// than against one row's bitrate ceiling: the fixture's default sits above SVT-AV1's.
			// The ceiling is not twice the target, the figure abr derives for the families that code
			// against a maximum either way.
			s.Publish.BitrateM, s.Publish.MaxrateM = 10, 15
			if capabilities.Validate(capabilities.EngineFfmpeg, s.Publish.Codec(), s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM, s.Publish.Gop, capabilities.Device{}) != nil {
				continue
			}
			args, err := encoderArgs(s, gopFor(s))
			if err != nil {
				t.Fatalf("%s %s: %v", c.Name, mode, err)
			}
			built[mode] = strings.Join(args, " ")
		}
		if len(built) == 2 && built[capabilities.ModeAbr] == built[capabilities.ModeVbr] {
			t.Errorf("%s builds one command for abr and vbr (%q), so one of the two modes is a name for the other",
				c.Name, built[capabilities.ModeAbr])
		}
	}
}

// The ceiling a constant-quality encode is held to, spelled as ffmpeg's capped rate factor.
// Both halves are needed: -maxrate alone states a rate the encoder is never asked to hold
// over any window, and the pair is what makes the quality
// target soften rather than the stream outgrow the link.
func TestConstantQualityCarriesTheStatedCeiling(t *testing.T) {
	for _, codec := range []string{"libx264", "libx265"} {
		s := baseStream()
		s.Publish.UseCodec(codec)
		s.Publish.Mode, s.Publish.Chroma = "crf", "yuv420p"
		s.Publish.MaxrateM, s.Publish.VbvMs = 12, 200
		args, err := encoderArgs(s, gopFor(s))
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}

		line := strings.Join(args, " ")
		for _, want := range []string{"-crf", "-maxrate 12M", "-bufsize"} {
			if !strings.Contains(line, want) {
				t.Errorf("%s crf under a ceiling: %s, want %s on it", codec, line, want)
			}
		}
	}
}

// An encode the settings state no ceiling for is unbounded, as -crf alone is.
// A ceiling invented here would bound a stream the user asked to run free,
// and the field carries zero for exactly that.
func TestConstantQualityWithoutACeilingStatesNoRate(t *testing.T) {
	s := baseStream()
	s.Publish.UseCodec("libx264")
	s.Publish.Mode, s.Publish.Chroma = "crf", "yuv420p"
	s.Publish.MaxrateM = 0
	args, err := encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(args, " ")
	if strings.Contains(line, "-maxrate") || strings.Contains(line, "-bufsize") {
		t.Errorf("libx264 crf with no ceiling: %s, want no rate bound on it", line)
	}
}

// A publish carries more than one tap where more than one thing reads the encoded stream:
// the preview leg and the byte meter that answers what the stream costs.
// One tee holds all of them, since two outputs written
// any other way are two encoders on one capture.
func TestEveryTapIsASlaveOfTheOneTee(t *testing.T) {
	s := baseStream()

	args, err := BuildPublishArgs(s, []Tap{
		{Options: []string{"select=v", "f=rtp", "onfail=ignore"}, URL: "rtp://127.0.0.1:45678"},
		{Options: []string{"select=v", "f=data", "onfail=ignore"}, URL: "tcp://127.0.0.1:45679"},
	})
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	line := strings.Join(args, " ")
	if strings.Count(line, "-f tee") != 1 {
		t.Errorf("not exactly one tee: %s", line)
	}
	for _, want := range []string{"rtp://127.0.0.1:45678", "tcp://127.0.0.1:45679"} {
		if !strings.Contains(line, want) {
			t.Errorf("%s is not a slave of the tee: %s", want, line)
		}
	}
}

// A publish nothing else reads keeps one output, a tee around a single muxer being a stage
// the packets did not need to cross.
func TestNoTapLeavesOneOutput(t *testing.T) {
	s := baseStream()

	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if line := strings.Join(args, " "); strings.Contains(line, "-f tee") {
		t.Errorf("a publish with no tap tees anyway: %s", line)
	}
}
