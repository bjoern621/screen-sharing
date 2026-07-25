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

func TestIsVaapi(t *testing.T) {
	cases := map[string]bool{
		"h264_vaapi": true,
		"vp8_vaapi":  true,
		"h264_qsv":   false, // another hardware family, another builder
		"libx264":    false,
		"vaapi":      false, // not a real codec name
	}
	for codec, want := range cases {
		if got := IsVaapi(codec); got != want {
			t.Errorf("IsVaapi(%q) = %v, want %v", codec, got, want)
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
		{"libx265", "gbrp", true},       // software HEVC codes RGB via Range Extensions
		{"libaom-av1", "gbrp", true},    // libaom codes RGB via AV1's identity matrix
		{"librav1e", "gbrp", false},     // rav1e codes no RGB matrix
		{"libsvtav1", "yuv444p", false}, // SVT-AV1 is 4:2:0 only
		{"libvpx", "yuv444p", false},    // VP8 has one profile, 8-bit 4:2:0
		{"hevc_vaapi", "p010le", true},  // VAAPI HEVC reaches Main 10
		{"hevc_vaapi", "gbrp", false},   // no VAAPI encoder codes RGB
		{"h264_vaapi", "p010le", false}, // no VAAPI driver encodes H.264 10-bit
		{"nope", "yuv420p", false},      // unknown codec supports nothing
	}
	for _, tc := range cases {
		if got := SupportsChroma(tc.codec, tc.chroma); got != tc.want {
			t.Errorf("SupportsChroma(%q,%q) = %v, want %v", tc.codec, tc.chroma, got, tc.want)
		}
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name                                   string
		engine, codec, chroma, transport, mode string
		cq                                     int
		wantErr                                bool
	}{
		{"nvenc hevc over srt", "ffmpeg", "hevc_nvenc", "gbrp", "srt", "lossless", 19, false},
		{"unknown codec", "ffmpeg", "nope", "yuv420p", "srt", "cbr", 19, true},
		{"unimplemented family", "ffmpeg", "hevc_qsv", "yuv420p", "srt", "cbr", 19, true},
		{"chroma the codec rejects", "ffmpeg", "libx264", "gbrp", "srt", "cbr", 19, true},
		{"transport that cannot carry", "ffmpeg", "libvpx-vp9", "yuv444p", "srt", "cbr", 19, true},
		// libvpx counts its quantizer to 63, the H.26x encoders to 51, so the same
		// value passes on one and fails on the other.
		{"vp9 quantizer at 60", "ffmpeg", "libvpx-vp9", "yuv444p", "rtsp", "crf", 60, false},
		{"x264 quantizer at 60", "ffmpeg", "libx264", "yuv420p", "rtsp", "crf", 60, true},
		{"negative quantizer", "ffmpeg", "libx264", "yuv420p", "rtsp", "crf", -1, true},
		// rav1e's quantizer counts to 255, the widest scale in the table.
		{"rav1e quantizer at 200", "ffmpeg", "librav1e", "yuv420p", "rtsp", "crf", 200, false},
		// The quantizer reaches the encoder in crf mode only, so a stale value from
		// another codec's scale must not block a bitrate mode.
		{"out-of-scale quantizer outside crf", "ffmpeg", "libx264", "yuv420p", "rtsp", "cbr", 60, false},
		// VP8 has no lossless mode on either engine; VP9's is ffmpeg's alone.
		{"vp8 lossless", "ffmpeg", "libvpx", "yuv420p", "rtsp", "lossless", 19, true},
		{"vp9 lossless over ffmpeg", "ffmpeg", "libvpx-vp9", "yuv444p", "rtsp", "lossless", 19, false},
		{"vp9 lossless over gstreamer", "gstreamer", "libvpx-vp9", "yuv444p", "rtsp", "lossless", 19, true},
		{"av1 lossless", "gstreamer", "libsvtav1", "yuv420p", "rtsp", "lossless", 19, true},
		// No VAAPI encoder codes bit-exact, on either engine, while its bitrate and
		// constant-quality modes are the drivers' own.
		{"vaapi lossless over ffmpeg", "ffmpeg", "h264_vaapi", "yuv420p", "srt", "lossless", 19, true},
		{"vaapi lossless over gstreamer", "gstreamer", "h264_vaapi", "yuv420p", "srt", "lossless", 19, true},
		{"vaapi cbr", "ffmpeg", "h264_vaapi", "yuv420p", "srt", "cbr", 19, false},
		// The VAAPI AV1 quantizer is a 0-255 index, where its H.26x rows stop at 51.
		{"vaapi av1 quantizer at 200", "ffmpeg", "av1_vaapi", "yuv420p", "rtsp", "crf", 200, false},
		{"vaapi h264 quantizer at 200", "ffmpeg", "h264_vaapi", "yuv420p", "rtsp", "crf", 200, true},
		// AV1 rides RTSP: MPEG-TS has no mapping the relay ingests.
		{"vaapi av1 over srt", "ffmpeg", "av1_vaapi", "yuv420p", "srt", "cbr", 19, true},
	}
	for _, tc := range cases {
		// Bitrate target zero: no row above turns on a codec's bitrate ceiling, which
		// TestValidateBitrateCeiling covers on its own.
		err := Validate(tc.engine, tc.codec, tc.chroma, tc.transport, tc.mode, tc.cq, 0)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: Validate = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

// A codec declaring a bitrate ceiling rejects a target above it, and only in the
// modes that send one. Every codec with no ceiling takes any target.
func TestValidateBitrateCeiling(t *testing.T) {
	for _, c := range Codecs {
		if !c.Implemented || c.BitrateLimitM == 0 {
			continue
		}
		chroma, transport := c.Chromas[0], c.Transports[0]
		if err := Validate("ffmpeg", c.Name, chroma, transport, "cbr", 0, c.BitrateLimitM+1); err == nil {
			t.Errorf("%s must reject a bitrate target above its %d Mbit/s ceiling", c.Name, c.BitrateLimitM)
		}
		if err := Validate("ffmpeg", c.Name, chroma, transport, "cbr", 0, c.BitrateLimitM); err != nil {
			t.Errorf("%s at its ceiling: %v", c.Name, err)
		}
		// crf sends no bitrate, so a stale target must not block the encode.
		if _, gap := ModeGapFor(c.Name, "ffmpeg", "crf"); !gap {
			if err := Validate("ffmpeg", c.Name, chroma, transport, "crf", 0, c.BitrateLimitM*2); err != nil {
				t.Errorf("%s in crf with a stale bitrate target: %v", c.Name, err)
			}
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
