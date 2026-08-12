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
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// encodeTimeout bounds one test encode. A single 256x256 frame returns in well under
// a second on every encoder here, so the only thing this catches is a library that
// takes the frame and emits nothing.
const encodeTimeout = 20 * time.Second

func baseStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:    "relay.example",
			SrtPort: 8890,
		},
		Publish: settings.Publish{
			Name:       "alice",
			Transport:  "srt",
			Codec:      "libx264",
			Mode:       "crf",
			Chroma:     "yuv444p",
			ColorRange: "pc",
			Fps:        60,
			Cq:         19,
			BitrateM:   150,
			MaxrateM:   200,
			Capture:    "x11grab",
			// No ladder step. The tests below reach every codec off this one draft, and a
			// step is one encoder's own identifier, so a fixture naming one would hand most
			// of them a step from an encoder they never heard of. What an unnamed step means
			// is the codec's declared one, which is what the builder resolves it to and what
			// a test asking about anything else wants.
			DrmMap:        "auto",
			CaptureMemory: gpupath.MemoryAuto,
			// The audio source is off here, so the codec matters only to the cases that turn it on.
			// It is filled all the same, exactly as migrateStream fills it, because the builder
			// validates the codec of every stream that names a source.
			AudioCodec: "opus",
		},
	}
}

// flagValue returns the argument following flag, or "" if flag is absent.
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

// Every grabber takes the rate as an input option and the keyframe interval follows
// from it, so a non-positive rate would reach ffmpeg as "-framerate 0" and "-g 0".
func TestBuildPublishArgsRefusesANonPositiveFps(t *testing.T) {
	for _, fps := range []int{0, -1} {
		s := baseStream()
		s.Publish.Fps = fps
		if _, err := BuildPublishArgs(s, nil); err == nil {
			t.Errorf("fps %d was accepted", fps)
		}
	}
}

// A monitor index no output carries names a screen this machine does not have.
// Capturing the whole desktop instead would publish something other than what the
// form shows selected, with nothing saying so.
func TestX11grabRefusesAMonitorIndexNoOutputCarries(t *testing.T) {
	s := baseStream()
	s.Publish.Monitor = 9999
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("a monitor index no output carries was accepted")
	}
}

// x11grab reads an X screen, and an environment naming none is no session to capture.
// The old ":0.0" guess captured whichever display happened to answer.
func TestX11grabRefusesAnUnsetDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	s := baseStream()
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("an unset DISPLAY was accepted")
	}
}

// The DRM download strategy exists to override the driver guess, so a name no row
// carries cannot resolve to that guess: the setting would run as its own opposite.
// The refusal is the table's, not the machine's, so it holds without a DRM node.
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

// The effort step is a free-form string in the settings and nothing upstream bounds it:
// capabilities.Validate covers the codec, pixel format, mode and the two rate
// figures, none of which this is.
func TestAStepOutsideTheLadderIsRefused(t *testing.T) {
	// The empty string is deliberately absent: it means the codec's own declared step,
	// which is what a draft holding none is entitled to and what the builder resolves it
	// to. A step off the ladder is refused, including one that belongs to another
	// encoder's ladder, which is what a draft that changed codec is holding.
	for _, tc := range []struct{ codec, step string }{
		{"hevc_nvenc", "p8"},
		{"hevc_nvenc", "slow"},
		{"libx264", "p7"},
		{"libsvtav1", "14"},
	} {
		s := baseStream()
		s.Publish.Codec, s.Publish.Chroma, s.Publish.Effort = tc.codec, "yuv420p", tc.step
		if _, err := encoderArgs(s, gopFor(s)); err == nil {
			t.Errorf("%s took the step %q", tc.codec, tc.step)
		}
	}
	// Every step a codec's own row declares builds a command.
	for _, codec := range []string{"hevc_nvenc", "libx264", "libsvtav1"} {
		c, ok := capabilities.Get(codec)
		if !ok {
			t.Fatalf("no capability row for %s", codec)
		}
		for _, step := range c.Effort.Steps {
			s := baseStream()
			s.Publish.Codec, s.Publish.Chroma, s.Publish.Effort = codec, "yuv420p", step
			s.Publish.Tune, _ = c.Tune.StepFor(s.Publish.Mode)
			if _, err := encoderArgs(s, gopFor(s)); err != nil {
				t.Errorf("%s step %q: %v", codec, step, err)
			}
		}
	}
}

// A mode that pins the step runs the pinned one whatever the draft holds, and the form
// greys the control there and names it. Both read the codec's row, so the step the
// encode spends and the step the sentence names cannot come apart.
func TestNvencCbrPinsTheDeclaredStep(t *testing.T) {
	c, ok := capabilities.Get("hevc_nvenc")
	if !ok {
		t.Fatal("no capability row for hevc_nvenc")
	}
	want, declared := c.Effort.StepFor("cbr")
	if !declared || !c.Effort.PinsIn("cbr") {
		t.Fatalf("hevc_nvenc no longer pins a declared step in cbr")
	}

	s := baseStream()
	s.Publish.Codec, s.Publish.Chroma, s.Publish.Mode, s.Publish.Effort = "hevc_nvenc", "yuv420p", "cbr", "p7"
	args, err := encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatal(err)
	}
	if got := flagValue(args, "-preset"); got != want {
		t.Errorf("-preset = %q, want the pinned %q", got, want)
	}
}

func TestBuildPublishArgsColorRange(t *testing.T) {
	// YUV chroma carries an explicit color range.
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

	// gbrp is inherently full range, so no -color_range is emitted. Only
	// hevc_nvenc encodes gbrp, so switch to it for this case.
	s.Publish.Codec = "hevc_nvenc"
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

	s.Publish.Gop = 0 // auto -> 2 * fps
	s.Publish.Fps = 60
	args, err := BuildPublishArgs(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := flagValue(args, "-g"); got != "120" {
		t.Errorf("auto -g = %q, want 120 (2*fps)", got)
	}

	s.Publish.Gop = 45 // explicit value wins
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
		want   string // substring that must appear in the joined args
		reject string // substring that must not appear ("" to skip)
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
		// VP8's screen coding tools are a different libvpx option from VP9's
		// tune-content, and VP8 has neither row threading nor a lossless mode.
		{"vp8 tunes for screen content", "libvpx", "cbr", "-screen-content-mode 1", "-tune-content"},
		{"vp8 crf is constant quality", "libvpx", "crf", "-crf", "-minrate"},
		{"vp8 cbr pins the rate", "libvpx", "cbr", "-minrate", "-crf"},
		{"aom av1 encodes realtime", "libaom-av1", "cbr", "-usage realtime", ""},
		{"aom av1 crf is constant quality", "libaom-av1", "crf", "-crf", "-minrate"},
		// SVT-AV1 refuses a ceiling outside constant-quality mode, which is why the
		// table gaps vbr on both engines and abr carries the target alone. CBR is a
		// rate-control mode only its own params reach.
		{"svt av1 abr holds no ceiling", "libsvtav1", "abr", "-b:v", "-maxrate"},
		{"svt av1 cbr selects rate control 2", "libsvtav1", "cbr", "rc=2:pred-struct=1", "-maxrate"},
		{"svt av1 crf takes no bitrate", "libsvtav1", "crf", "-crf", "-b:v"},
		// rav1e has one bitrate target, no ceiling and no rate buffer, so vbr is
		// gapped on both engines and abr is the bursting mode it does implement.
		{"rav1e crf uses qp", "librav1e", "crf", "-qp", "-crf"},
		{"rav1e abr holds no ceiling", "librav1e", "abr", "-b:v", "-maxrate"},
		{"rav1e cbr drops reordering", "librav1e", "cbr", "low_latency=true", "-bufsize"},
		// The VAAPI encoders take one rc_mode per rate-control concept, and the
		// quantizer travels in -qp on the H.26x ones and -global_quality elsewhere.
		{"vaapi crf is CQP", "h264_vaapi", "crf", "-rc_mode CQP -qp", "-b:v"},
		{"vaapi av1 quantizer is global_quality", "av1_vaapi", "crf", "-global_quality", "-qp"},
		{"vaapi cbr pins the ceiling to the target", "h264_vaapi", "cbr", "-rc_mode CBR", "-rc_mode VBR"},
		{"vaapi abr gives no ceiling", "h264_vaapi", "abr", "-rc_mode VBR", "-maxrate"},
		{"vaapi vbr sets a ceiling", "hevc_vaapi", "vbr", "-maxrate", "-rc_mode CBR"},
		// The QSV encoders have no rate-control option at all: oneVPL's method follows
		// from which rate options carry a value, so each mode is a shape rather than a
		// name. A quantizer with the qscale flag selects CQP, a ceiling equal to the
		// target CBR, and one above it VBR.
		{"qsv crf carries a quantizer alone", "h264_qsv", "crf", "-q:v", "-b:v"},
		{"qsv cbr pins the ceiling to the target", "h264_qsv", "cbr", "-b:v 150M -maxrate 150M", ""},
		{"qsv abr caps its ceiling above the target", "hevc_qsv", "abr", "-b:v 150M -maxrate 300M", "-bufsize"},
		{"qsv vbr takes the configured ceiling", "hevc_qsv", "vbr", "-maxrate 200M", ""},
		// cbr is the mode that trades the encoder's pipeline depth for delay, and the
		// only one to touch it.
		{"qsv cbr shortens the pipeline", "av1_qsv", "cbr", "-async_depth 1", ""},
		{"qsv vbr keeps the default pipeline", "av1_qsv", "vbr", "-b:v", "-async_depth"},
		{"qsv cbr encodes for speed", "h264_qsv", "cbr", "-preset veryfast", "-preset medium"},
		{"qsv crf encodes for quality", "h264_qsv", "crf", "-preset medium", "-preset veryfast"},
		{"qsv pins b-pictures off", "hevc_qsv", "vbr", "-bf 0", ""},
		// The AMF encoders take one -rc mode per rate-control concept, and their
		// bursting modes are peak-constrained VBR, so even ABR states a ceiling.
		{"amf crf is cqp", "h264_amf", "crf", "-rc cqp -qp_i", "-b:v"},
		{"amf cbr pins the ceiling to the target", "h264_amf", "cbr", "-rc cbr", "vbr_peak"},
		{"amf abr caps its peak above the target", "hevc_amf", "abr", "-rc vbr_peak -b:v 150M -maxrate 300M", ""},
		{"amf vbr takes the configured ceiling", "hevc_amf", "vbr", "-maxrate 200M", "-rc cbr"},
		// AMF's low-latency usage presets drop the H.264 IDR period, so no mode selects
		// one; cbr states its live character through the quality scale instead.
		{"amf cbr keeps the transcoding usage", "h264_amf", "cbr", "-usage transcoding", "lowlatency"},
		{"amf cbr encodes for speed", "h264_amf", "cbr", "-quality speed", "-quality quality"},
		{"amf vbr encodes for quality", "h264_amf", "vbr", "-quality quality", "-quality speed"},
		// AMF's H.264 and AV1 encoders have a B-picture pattern; its HEVC one does not,
		// so the option that switches them off must not reach it.
		{"amf h264 pins b-frames off", "h264_amf", "vbr", "-bf 0", ""},
		{"amf av1 pins b-frames off", "av1_amf", "vbr", "-bf 0", ""},
		{"amf hevc has no b-frame option", "hevc_amf", "vbr", "-rc vbr_peak", "-bf"},
		// Only AMF's H.264 encoder has to be told to repeat its parameter sets, and it
		// repeats them once per GOP: baseStream runs 60 fps with no explicit interval,
		// so the automatic two seconds is 120 frames.
		{"amf h264 repeats its parameter sets per gop", "h264_amf", "cbr", "-header_spacing 120", ""},
		{"amf hevc needs no header spacing", "hevc_amf", "cbr", "-rc cbr", "-header_spacing"},
		{"amf av1 has no header spacing option", "av1_amf", "cbr", "-rc cbr", "-header_spacing"},
		// The Vulkan encoders take one -rc_mode per rate-control concept, and their
		// bursting modes always code against a ceiling, so even ABR states one.
		{"vulkan crf is cqp", "h264_vulkan", "crf", "-rc_mode cqp -qp", "-b:v"},
		{"vulkan cbr pins the ceiling to the target", "h264_vulkan", "cbr", "-rc_mode cbr -b:v 150M -maxrate 150M", ""},
		{"vulkan abr caps its ceiling above the target", "hevc_vulkan", "abr", "-rc_mode vbr -b:v 150M -maxrate 300M", ""},
		{"vulkan vbr takes the configured ceiling", "hevc_vulkan", "vbr", "-maxrate 200M", "-rc_mode cbr"},
		// Every mode declares the stream a live one of screen content, and cbr is the
		// one that trades quality for keeping up with it.
		{"vulkan states its content type", "av1_vulkan", "cbr", "-usage stream -content desktop", ""},
		{"vulkan cbr tunes for latency", "h264_vulkan", "cbr", "-tune ll", "-tune hq"},
		{"vulkan vbr tunes for quality", "h264_vulkan", "vbr", "-tune hq", "-tune ll"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseStream()
			s.Publish.Codec = tc.codec
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

// The rate buffer is SVT-AV1's CBR knob alone: buf-sz rides in -svtav1-params, which
// only that mode sends, so the bursting mode has nothing to size and the window the
// settings carry stops at the builder. The form greys the field there for that reason
// (ENGINE_RULES), and a value reaching the command in abr would make the greying a
// lie.
func TestSvtAv1SizesARateBufferInCbrOnly(t *testing.T) {
	s := baseStream()
	s.Publish.Codec, s.Publish.VbvMs = "libsvtav1", 500

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

// VP9 splits its profiles by subsampling and bit depth, and libvpx refuses a pixel
// format the selected profile cannot carry, so the profile has to follow the chroma
// for the codec's four chromas to be encodable at all.
func TestVp9ProfileFollowsChroma(t *testing.T) {
	want := map[string]string{
		"yuv420p": "0",
		"yuv444p": "1",
		"gbrp":    "1",
		"p010le":  "2",
	}
	for chroma, profile := range want {
		s := baseStream()
		s.Publish.Codec, s.Publish.Chroma = "libvpx-vp9", chroma
		args, err := encoderArgs(s, gopFor(s))
		if err != nil {
			t.Fatal(err)
		}
		if got := flagValue(args, "-profile:v"); got != profile {
			t.Errorf("libvpx-vp9 at %s: -profile:v = %q, want %q", chroma, got, profile)
		}
	}
}

// AMF announces the Main profile whatever surface it is handed, so a 10-bit encode
// would ship a Main-profile bitstream carrying 10-bit samples. The profile therefore
// follows the chroma, and its two HEVC rows are the only place in this builder where
// it does.
func TestAmfHevcProfileFollowsChroma(t *testing.T) {
	want := map[string]string{
		"yuv420p": "main",
		"p010le":  "main10",
	}
	for chroma, profile := range want {
		s := baseStream()
		s.Publish.Codec, s.Publish.Chroma = "hevc_amf", chroma
		args, err := encoderArgs(s, gopFor(s))
		if err != nil {
			t.Fatal(err)
		}
		if got := flagValue(args, "-profile"); got != profile {
			t.Errorf("hevc_amf at %s: -profile = %q, want %q", chroma, got, profile)
		}
	}
}

// The encoder arguments are a wire format shared with ffmpeg: an option that does not
// exist, a value out of range, or a rate-control combination the library refuses is a
// publish that dies on launch, and none of it holds against a compiler. So this
// encodes one frame per codec and mode with the real arguments the builder produces.
//
// The capability table drives the loop, so a codec added there without a mapping, or
// a mode declared reachable that the library rejects, fails here.
func TestEncoderArgsAgainstFfmpeg(t *testing.T) {
	exe, err := FindExe("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	for _, cap := range capabilities.Codecs {
		if !cap.Implemented {
			continue
		}
		// A VAAPI or Vulkan encode reads GPU surfaces, so its arguments only run with
		// the device and the upload filter the publish command puts in front of them.
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
				s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = cap.Name, mode, engineChromas[len(engineChromas)-1]
				// The quantizer target rides each encoder's own scale, and the bitrate
				// target has a ceiling on one encoder. baseStream carries values from
				// another codec's, exactly as saved settings do before normalize runs.
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
					upload, err := HwSurfaceFilters(s.Publish.Codec, s.Publish.Chroma)
					if err != nil {
						t.Fatal(err)
					}
					args = append(args, "-vf", strings.Join(upload, ","))
				}
				args = append(args, enc...)
				// A surface encode's layout is the upload filter's, and its encoder reads
				// no software pixel format at all, exactly as in BuildPublishArgs.
				if !surface {
					args = append(args, "-pix_fmt", s.Publish.Chroma)
				}
				args = append(args, "-f", "null", "-")

				ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
				defer cancel()
				out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
				if err != nil {
					// An absent hardware encoder is the machine's answer, not the
					// builder's; encoders.Detect greys those in the UI for the same
					// reason. A software encoder is the build's answer and every one of
					// them ships in an ffmpeg worth publishing with, so a failure there
					// is the arguments.
					if cap.Family == "software" {
						t.Errorf("ffmpeg %s: %v\n%s", strings.Join(args, " "), err, out)
					}
					t.Skipf("%s does not run on this machine", cap.Name)
				}
			})
		}
	}
}

// A VAAPI publish opens its device ahead of the input and ends the capture chain in
// the upload, and pins no software pixel format, since the encoder reads GPU
// surfaces. The device is real hardware, so the shape is only asserted where one
// exists; VaapiFilters carries the chroma mapping and is checked below regardless.
func TestBuildPublishArgsVaapi(t *testing.T) {
	if _, err := VaapiDevice(); err != nil {
		t.Skip("no VAAPI render node on this machine")
	}

	s := baseStream()
	s.Publish.Codec = "h264_vaapi"
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

// A Vulkan publish has the same shape and a device of its own: created under a name
// ahead of the input, then handed to the filter graph the upload attaches to.
func TestBuildPublishArgsVulkan(t *testing.T) {
	s := baseStream()
	s.Publish.Codec = "h264_vulkan"
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

// A QSV publish is the third surface shape: a device created under a name, as on Vulkan,
// and an upload that sizes its own frame pool.
func TestBuildPublishArgsQsv(t *testing.T) {
	s := baseStream()
	s.Publish.Codec = "hevc_qsv"
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
	// A chroma neither family's hardware stores is a builder error, not a silent
	// conversion.
	if _, err := HwSurfaceFilters("h264_vaapi", "yuv444p"); err == nil {
		t.Error("HwSurfaceFilters must reject a chroma with no hardware surface layout")
	}
}

// The upload element follows the family, not the chroma: a QSV encoder takes surfaces
// from the pool the upload allocated and holds several of them, so its pool carries
// frames beyond what the filter graph itself needs.
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

// Only the families whose encoders read GPU surfaces open a device, and the ones that
// read system memory must not: a device option there would be a filter graph the
// software encoders cannot take frames from.
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
		// The device itself is real hardware, so only the verdict is asserted; an
		// error means the family is absent from this machine, not from the table.
		_, got, _ := HwSurfaceDevice(codec)
		if got != want {
			t.Errorf("HwSurfaceDevice(%q) surface = %v, want %v", codec, got, want)
		}
	}
	if _, _, err := HwSurfaceDevice("nope"); err == nil {
		t.Error("HwSurfaceDevice must reject a codec the table does not carry")
	}
}

// The audio track is coded by whichever element the capability table states for this engine,
// so the command names that element rather than one spelled here.
// The two engines code the same codec with different libraries, and a name written into the
// test would be one engine's answer asserted for both.
func TestBuildPublishArgsAudio(t *testing.T) {
	// Desktop audio on a Linux backend: the pulse monitor input plus the encode the selected
	// codec's row declares.
	// SRT carries both codecs, so the transport never decides which of them this covers.
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
		// The first -i is the video capture input, so check membership instead.
		if !slices.Contains(args, "pulse") || !slices.Contains(args, platform.AudioMonitorDevice) {
			t.Errorf("missing pulse monitor input, got %v", args)
		}
		if got := flagValue(args, "-c:a"); got != enc.Element {
			t.Errorf("%s: -c:a = %q, want the element the table states, %q", a.Name, got, enc.Element)
		}
		// The rate and the bitrate follow the codec too, since an encoder that codes at one
		// rate alone is handed another rate's samples otherwise.
		if got := flagValue(args, "-ar"); got != strconv.Itoa(a.Rate) {
			t.Errorf("%s: -ar = %q, want %d", a.Name, got, a.Rate)
		}
		if got := flagValue(args, "-b:a"); got != strconv.Itoa(a.BitrateK)+"k" {
			t.Errorf("%s: -b:a = %q, want %dk", a.Name, got, a.BitrateK)
		}
	}

	// Audio off, or a settings file from before the option: no audio args at all, whatever
	// codec the settings carry.
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

	// A backend whose platform serves no monitor source refuses desktop audio rather than
	// publishing a silent track. That refusal is not this builder's: the source table
	// answers it and publish holds the backend's operating system, so it is asserted in
	// publish (TestABackendWhosePlatformServesNoMonitorSourceRefusesDesktopAudio) against
	// the engine entry points a run and the displayed command both go through.

	s := baseStream()
	s.Publish.AudioSources = settings.Recording("microphone")
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for an unknown audio source")
	}

	// An audio codec no row carries is refused before a command is built, since the encoder
	// name would otherwise be read off an absent row.
	s = baseStream()
	s.Publish.AudioCodec = "mp3"
	s.Publish.AudioSources = settings.Recording("desktop")
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for an audio codec the table does not carry")
	}

	// A codec the publish leg cannot carry is the transport's refusal, and this engine has to
	// make it: WebRTC negotiates Opus and no AAC at all.
	s = baseStream()
	s.Publish.Transport, s.Publish.Codec, s.Publish.Chroma = "webrtc", "libx264", "yuv420p"
	s.Publish.AudioCodec = "aac"
	s.Publish.AudioSources = settings.Recording("desktop")
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for an AAC track over webrtc")
	}
}

func TestBuildPublishArgsIncompatibleCodec(t *testing.T) {
	// libx264 cannot encode gbrp: the capability check must reject it.
	s := baseStream()
	s.Publish.Codec = "libx264"
	s.Publish.Chroma = "gbrp"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for libx264 + gbrp")
	}

	// SRT cannot carry AV1.
	s = baseStream()
	s.Publish.Codec = "av1_nvenc"
	s.Publish.Chroma = "yuv420p"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for av1_nvenc over srt")
	}

	// VP9 cannot travel over SRT: MPEG-TS has no VP9 mapping, so the table lists
	// rtsp only.
	s = baseStream()
	s.Publish.Codec = "libvpx-vp9"
	s.Publish.Chroma = "yuv444p"
	s.Publish.Transport = "srt"
	if _, err := BuildPublishArgs(s, nil); err == nil {
		t.Fatal("expected error for libvpx-vp9 over srt")
	}
}

// Two rate-control modes the table allows for one codec have to produce two
// commands. abr aims at an average and vbr bounds the burst above it, so a codec
// whose encoder cannot bound the burst implements one of the two and not both, and
// the table says so with a mode gap.
//
// Building both and getting the same arguments back is the failure this guards: the
// encode runs as whichever mode the builder collapsed onto, while the command, the
// bitrate estimate and every verdict downstream keep naming the mode that was picked.
// Before the gaps existed, five software codecs did exactly that on the GStreamer
// engine and two did it on this one.
func TestAbrAndVbrDifferWhereBothAreAllowed(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		built := map[string]string{}
		for _, mode := range []string{capabilities.ModeAbr, capabilities.ModeVbr} {
			s := baseStream()
			s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = c.Name, mode, c.EngineChromas(capabilities.EngineFfmpeg)[0]
			// A rate every encoder takes, so what this compares is the two modes and
			// not one codec's rate ceiling: the default sits above SVT-AV1's. The
			// ceiling is not twice the target, which is the value abr derives for the
			// families that code against a maximum either way.
			s.Publish.BitrateM, s.Publish.MaxrateM = 10, 15
			if capabilities.Validate(capabilities.EngineFfmpeg, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM) != nil {
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
