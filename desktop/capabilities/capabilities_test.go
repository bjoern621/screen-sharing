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

func TestValidate(t *testing.T) {
	cases := []struct {
		name                           string
		codec, chroma, transport, mode string
		cq                             int
		wantErr                        bool
	}{
		{"nvenc hevc over srt", "hevc_nvenc", "gbrp", "srt", "lossless", 19, false},
		{"unknown codec", "nope", "yuv420p", "srt", "cbr", 19, true},
		{"unimplemented family", "hevc_vaapi", "yuv420p", "srt", "cbr", 19, true},
		{"chroma the codec rejects", "libx264", "gbrp", "srt", "cbr", 19, true},
		{"transport that cannot carry", "libvpx-vp9", "yuv444p", "srt", "cbr", 19, true},
		// libvpx counts its quantizer to 63, the H.26x encoders to 51, so the same
		// value passes on one and fails on the other.
		{"vp9 quantizer at 60", "libvpx-vp9", "yuv444p", "rtsp", "crf", 60, false},
		{"x264 quantizer at 60", "libx264", "yuv420p", "rtsp", "crf", 60, true},
		{"negative quantizer", "libx264", "yuv420p", "rtsp", "crf", -1, true},
		// The quantizer reaches the encoder in crf mode only, so a stale value from
		// another codec's scale must not block a bitrate mode.
		{"out-of-scale quantizer outside crf", "libx264", "yuv420p", "rtsp", "cbr", 60, false},
	}
	for _, tc := range cases {
		err := Validate(tc.codec, tc.chroma, tc.transport, tc.mode, tc.cq)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: Validate = %v, wantErr %v", tc.name, err, tc.wantErr)
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
