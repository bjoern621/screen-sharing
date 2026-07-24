package ffmpeg

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/settings"
)

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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseStream()
			s.Codec = tc.codec
			s.Mode = tc.mode
			args, err := encoderArgs(s)
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
}
