package capabilities

import "testing"

func TestIsNvenc(t *testing.T) {
	cases := map[string]bool{
		"hevc_nvenc": true,
		"h264_nvenc": true,
		"av1_nvenc":  true,
		"libx264":    false,
		"libx265":    false,
		"_nvenc":     false, // not a real codec name
		"nvenc":      false,
	}
	for codec, want := range cases {
		if got := IsNvenc(codec); got != want {
			t.Errorf("IsNvenc(%q) = %v, want %v", codec, got, want)
		}
	}
}

func TestSupportsChroma(t *testing.T) {
	cases := []struct {
		codec, chroma string
		want          bool
	}{
		{"hevc_nvenc", "gbrp", true},
		{"h264_nvenc", "gbrp", false},   // only HEVC codes RGB here
		{"av1_nvenc", "yuv444p", false}, // NVENC AV1 is 4:2:0 only
		{"av1_nvenc", "yuv420p", true},
		{"libx264", "gbrp", false},
		{"libx265", "gbrp", true},  // software HEVC codes RGB via Range Extensions
		{"nope", "yuv420p", false}, // unknown codec supports nothing
	}
	for _, tc := range cases {
		if got := SupportsChroma(tc.codec, tc.chroma); got != tc.want {
			t.Errorf("SupportsChroma(%q,%q) = %v, want %v", tc.codec, tc.chroma, got, tc.want)
		}
	}
}

func TestCarriedBy(t *testing.T) {
	if !CarriedBy("hevc_nvenc", "srt") {
		t.Error("srt must carry hevc_nvenc")
	}
	if CarriedBy("av1_nvenc", "srt") {
		t.Error("srt/MPEG-TS cannot carry av1_nvenc")
	}
	if CarriedBy("libx265", "webrtc") {
		t.Error("webrtc (WHIP is H.264 + Opus) cannot carry libx265")
	}
}
