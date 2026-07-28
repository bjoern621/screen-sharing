package ffmpeg

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// encodeTimeout bounds one test encode. A single 256x256 frame returns in well under
// a second on every encoder here, so the only thing this catches is a library that
// takes the frame and emits nothing.
const encodeTimeout = 20 * time.Second

func baseStream() settings.Stream {
	return settings.Stream{
		Name:       "alice",
		RelayHost:  "relay.example",
		RelayPort:  8890,
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
	s.Transport = "carrier-pigeon"
	if _, err := BuildPublishArgs(s); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestBuildPublishArgsUnknownCapture(t *testing.T) {
	s := baseStream()
	s.Capture = "telepathy"
	if _, err := BuildPublishArgs(s); err == nil {
		t.Fatal("expected error for unknown capture backend")
	}
}

func TestBuildPublishArgsColorRange(t *testing.T) {
	// YUV chroma carries an explicit color range.
	s := baseStream()
	s.Chroma = "yuv444p"
	args, err := BuildPublishArgs(s)
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
	s.Codec = "hevc_nvenc"
	s.Chroma = "gbrp"
	args, err = BuildPublishArgs(s)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "-color_range") {
		t.Errorf("gbrp must not emit -color_range, got %v", args)
	}
}

func TestBuildPublishArgsGop(t *testing.T) {
	s := baseStream()

	s.Gop = 0 // auto -> 2 * fps
	s.Fps = 60
	args, err := BuildPublishArgs(s)
	if err != nil {
		t.Fatal(err)
	}
	if got := flagValue(args, "-g"); got != "120" {
		t.Errorf("auto -g = %q, want 120 (2*fps)", got)
	}

	s.Gop = 45 // explicit value wins
	args, err = BuildPublishArgs(s)
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
		// SVT-AV1 refuses a ceiling outside constant-quality mode, so VBR carries the
		// target alone, and CBR is a rate-control mode only its own params reach.
		{"svt av1 vbr holds no ceiling", "libsvtav1", "vbr", "-b:v", "-maxrate"},
		{"svt av1 cbr selects rate control 2", "libsvtav1", "cbr", "rc=2:pred-struct=1", "-maxrate"},
		{"svt av1 crf takes no bitrate", "libsvtav1", "crf", "-crf", "-b:v"},
		// rav1e has one bitrate target, no ceiling and no rate buffer.
		{"rav1e crf uses qp", "librav1e", "crf", "-qp", "-crf"},
		{"rav1e vbr holds no ceiling", "librav1e", "vbr", "-b:v", "-maxrate"},
		{"rav1e cbr drops reordering", "librav1e", "cbr", "low_latency=true", "-bufsize"},
		// The VAAPI encoders take one rc_mode per rate-control concept, and the
		// quantizer travels in -qp on the H.26x ones and -global_quality elsewhere.
		{"vaapi crf is CQP", "h264_vaapi", "crf", "-rc_mode CQP -qp", "-b:v"},
		{"vaapi av1 quantizer is global_quality", "av1_vaapi", "crf", "-global_quality", "-qp"},
		{"vaapi cbr pins the ceiling to the target", "h264_vaapi", "cbr", "-rc_mode CBR", "-rc_mode VBR"},
		{"vaapi abr gives no ceiling", "h264_vaapi", "abr", "-rc_mode VBR", "-maxrate"},
		{"vaapi vbr sets a ceiling", "hevc_vaapi", "vbr", "-maxrate", "-rc_mode CBR"},
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
			s.Codec = tc.codec
			s.Mode = tc.mode
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
// only that mode sends, so VBR has nothing to size and the window the settings carry
// stops at the builder. The form greys the field there for that reason (ENGINE_RULES),
// and a value reaching the command in VBR would make the greying a lie.
func TestSvtAv1SizesARateBufferInCbrOnly(t *testing.T) {
	s := baseStream()
	s.Codec, s.VbvMs = "libsvtav1", 500

	s.Mode = "vbr"
	args, err := encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); strings.Contains(joined, "buf-sz") {
		t.Errorf("libsvtav1 vbr = %q, must carry no rate buffer", joined)
	}

	s.Mode = "cbr"
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
		s.Codec, s.Chroma = "libvpx-vp9", chroma
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
		s.Codec, s.Chroma = "hevc_amf", chroma
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
			if _, gap := cap.ModeGap("ffmpeg", mode); gap {
				continue
			}
			t.Run(cap.Name+"/"+mode, func(t *testing.T) {
				s := baseStream()
				engineChromas := cap.EngineChromas("ffmpeg")
				s.Codec, s.Mode, s.Chroma = cap.Name, mode, engineChromas[len(engineChromas)-1]
				// The quantizer target rides each encoder's own scale, and the bitrate
				// target has a ceiling on one encoder. baseStream carries values from
				// another codec's, exactly as saved settings do before normalize runs.
				s.Cq = cap.CqMax / 2
				if cap.BitrateLimitM > 0 && s.BitrateM > cap.BitrateLimitM {
					s.BitrateM = cap.BitrateLimitM
				}
				enc, err := encoderArgs(s, gopFor(s))
				if err != nil {
					t.Fatal(err)
				}

				args := []string{"-hide_banner", "-loglevel", "error"}
				args = append(args, device...)
				args = append(args, "-f", "lavfi", "-i", "nullsrc=s=256x256", "-frames:v", "1")
				if surface {
					upload, err := HwSurfaceFilters(s.Chroma)
					if err != nil {
						t.Fatal(err)
					}
					args = append(args, "-vf", strings.Join(upload, ","))
				}
				args = append(args, enc...)
				// A surface encode's layout is the upload filter's, and its encoder reads
				// no software pixel format at all, exactly as in BuildPublishArgs.
				if !surface {
					args = append(args, "-pix_fmt", s.Chroma)
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
	s.Codec = "h264_vaapi"
	s.Chroma = "yuv420p"
	s.Mode = "cbr"
	args, err := BuildPublishArgs(s)
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
	s.Codec = "h264_vulkan"
	s.Chroma = "yuv420p"
	s.Mode = "cbr"
	args, err := BuildPublishArgs(s)
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

func TestHwSurfaceFilters(t *testing.T) {
	cases := map[string]string{
		"yuv420p": "format=nv12",
		"p010le":  "format=p010",
	}
	for chroma, want := range cases {
		got, err := HwSurfaceFilters(chroma)
		if err != nil {
			t.Fatalf("HwSurfaceFilters(%q): %v", chroma, err)
		}
		if len(got) != 2 || got[0] != want || got[1] != "hwupload" {
			t.Errorf("HwSurfaceFilters(%q) = %v, want [%s hwupload]", chroma, got, want)
		}
	}
	// A chroma neither family's hardware stores is a builder error, not a silent
	// conversion.
	if _, err := HwSurfaceFilters("yuv444p"); err == nil {
		t.Error("HwSurfaceFilters must reject a chroma with no hardware surface layout")
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

func TestBuildPublishArgsAudio(t *testing.T) {
	// Desktop audio on a Linux backend: pulse monitor input plus Opus encode.
	s := baseStream()
	s.Audio = "desktop"
	args, err := BuildPublishArgs(s)
	if err != nil {
		t.Fatal(err)
	}
	// The first -i is the video capture input, so check membership instead.
	if !slices.Contains(args, "pulse") || !slices.Contains(args, pulseMonitorDevice) {
		t.Errorf("missing pulse monitor input, got %v", args)
	}
	if got := flagValue(args, "-c:a"); got != "libopus" {
		t.Errorf("-c:a = %q, want libopus", got)
	}

	// Audio off (or a pre-audio settings file): no audio args at all.
	for _, audio := range []string{"none", ""} {
		s.Audio = audio
		args, err = BuildPublishArgs(s)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(args, "pulse") || slices.Contains(args, "-c:a") {
			t.Errorf("audio %q must not emit audio args, got %v", audio, args)
		}
	}

	// Windows capture backends have no desktop audio path.
	s = baseStream()
	s.Capture = "gdigrab"
	s.Audio = "desktop"
	if _, err := BuildPublishArgs(s); err == nil {
		t.Fatal("expected error for desktop audio with a Windows capture backend")
	}

	s.Capture = "x11grab"
	s.Audio = "microphone"
	if _, err := BuildPublishArgs(s); err == nil {
		t.Fatal("expected error for an unknown audio source")
	}
}

func TestBuildPublishArgsIncompatibleCodec(t *testing.T) {
	// libx264 cannot encode gbrp: the capability check must reject it.
	s := baseStream()
	s.Codec = "libx264"
	s.Chroma = "gbrp"
	if _, err := BuildPublishArgs(s); err == nil {
		t.Fatal("expected error for libx264 + gbrp")
	}

	// SRT cannot carry AV1.
	s = baseStream()
	s.Codec = "av1_nvenc"
	s.Chroma = "yuv420p"
	if _, err := BuildPublishArgs(s); err == nil {
		t.Fatal("expected error for av1_nvenc over srt")
	}

	// VP9 cannot travel over SRT: MPEG-TS has no VP9 mapping, so the table lists
	// rtsp only.
	s = baseStream()
	s.Codec = "libvpx-vp9"
	s.Chroma = "yuv444p"
	s.Transport = "srt"
	if _, err := BuildPublishArgs(s); err == nil {
		t.Fatal("expected error for libvpx-vp9 over srt")
	}
}
