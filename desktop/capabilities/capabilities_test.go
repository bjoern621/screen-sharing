package capabilities

import (
	"slices"
	"testing"
)

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
		codec, engine, chroma string
		want                  bool
	}{
		{"hevc_nvenc", "ffmpeg", "gbrp", true},
		{"h264_nvenc", "ffmpeg", "gbrp", false},   // only HEVC codes RGB here
		{"av1_nvenc", "ffmpeg", "yuv444p", false}, // NVENC AV1 is 4:2:0 only
		{"av1_nvenc", "ffmpeg", "yuv420p", true},
		{"libx264", "ffmpeg", "gbrp", false},
		{"libx265", "ffmpeg", "gbrp", true},       // software HEVC codes RGB via Range Extensions
		{"libaom-av1", "ffmpeg", "gbrp", true},    // libaom codes RGB via AV1's identity matrix
		{"librav1e", "ffmpeg", "gbrp", false},     // rav1e codes no RGB matrix
		{"libsvtav1", "ffmpeg", "yuv444p", false}, // SVT-AV1 is 4:2:0 only
		{"libvpx", "ffmpeg", "yuv444p", false},    // VP8 has one profile, 8-bit 4:2:0
		{"hevc_vaapi", "ffmpeg", "p010le", true},  // VAAPI HEVC reaches Main 10
		{"hevc_vaapi", "ffmpeg", "gbrp", false},   // no VAAPI encoder codes RGB
		{"h264_vaapi", "ffmpeg", "p010le", false}, // no VAAPI driver encodes H.264 10-bit
		{"nope", "ffmpeg", "yuv420p", false},      // unknown codec supports nothing
		// Planar RGB is the ffmpeg engine's alone: every GStreamer encoder element
		// negotiates YUV, so each RGB-coding codec keeps the format on one engine.
		{"hevc_nvenc", "gstreamer", "gbrp", false},
		{"libx265", "gstreamer", "gbrp", false},
		{"libvpx-vp9", "gstreamer", "gbrp", false},
		{"libaom-av1", "gstreamer", "gbrp", false},
		{"libx265", "gstreamer", "yuv444p", true}, // only the RGB format is gapped
		// 10-bit libaom AV1: ffmpeg's wrapper codes it, av1enc takes 8-bit input only.
		// The other two software AV1 encoders carry 10-bit on both engines.
		{"libaom-av1", "ffmpeg", "p010le", true},
		{"libaom-av1", "gstreamer", "p010le", false},
		{"libsvtav1", "gstreamer", "p010le", true},
		{"librav1e", "gstreamer", "p010le", true},
	}
	for _, tc := range cases {
		if got := SupportsChroma(tc.codec, tc.engine, tc.chroma); got != tc.want {
			t.Errorf("SupportsChroma(%q,%q,%q) = %v, want %v", tc.codec, tc.engine, tc.chroma, got, tc.want)
		}
	}
}

// The chromas a codec reaches on one engine are its table order minus that engine's
// gaps, so a caller that needs a working format (the encoder tests, the frontend's
// fallback walk) can take the first entry.
func TestEngineChromas(t *testing.T) {
	cases := []struct {
		codec, engine string
		want          []string
	}{
		{"libx265", "ffmpeg", []string{"gbrp", "yuv444p", "yuv420p", "p010le"}},
		{"libx265", "gstreamer", []string{"yuv444p", "yuv420p", "p010le"}},
		{"libaom-av1", "ffmpeg", []string{"gbrp", "yuv444p", "yuv420p", "p010le"}},
		{"libaom-av1", "gstreamer", []string{"yuv444p", "yuv420p"}},
		{"libx264", "gstreamer", []string{"yuv444p", "yuv420p", "p010le"}},
	}
	for _, tc := range cases {
		c, ok := Get(tc.codec)
		if !ok {
			t.Errorf("%s is not in the table", tc.codec)
			continue
		}
		if got := c.EngineChromas(tc.engine); !slices.Equal(got, tc.want) {
			t.Errorf("%s on %s encodes %v, want %v", tc.codec, tc.engine, got, tc.want)
		}
	}
}

// A gap belongs to one axis: the pixel-format lookup must not answer a rate-control
// question or the reverse, and a codec with no engine-wide gap runs on both engines.
func TestGapAxesDoNotCross(t *testing.T) {
	aom, _ := Get("libaom-av1")
	if _, gap := aom.ModeGap("gstreamer", "p010le"); gap {
		t.Error("a chroma gap must not answer a rate-control lookup")
	}
	if _, gap := aom.ChromaGap("gstreamer", "lossless"); gap {
		t.Error("a mode gap must not answer a pixel-format lookup")
	}
	// A gap naming neither axis takes the codec off that engine. No row needs one,
	// since both builders map every implemented codec, so the shape is checked on a
	// value rather than on the table.
	oneEngine := Codec{
		Name: "hypothetical",
		Gaps: []Gap{{Engine: "gstreamer", Reason: "no element encodes it"}},
	}
	if _, gap := oneEngine.EngineGap("gstreamer"); !gap {
		t.Error("a gap naming neither chroma nor mode must take the codec off its engine")
	}
	if _, gap := oneEngine.EngineGap("ffmpeg"); gap {
		t.Error("an engine gap must not bind on the other engine")
	}
	if _, gap := oneEngine.ChromaGap("gstreamer", "yuv420p"); gap {
		t.Error("an engine gap must not answer a pixel-format lookup")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name                        string
		engine, codec, chroma, mode string
		cq                          int
		wantErr                     bool
	}{
		{"nvenc hevc lossless", "ffmpeg", "hevc_nvenc", "gbrp", "lossless", 19, false},
		{"unknown codec", "ffmpeg", "nope", "yuv420p", "cbr", 19, true},
		{"unimplemented family", "ffmpeg", "hevc_qsv", "yuv420p", "cbr", 19, true},
		{"chroma the codec rejects", "ffmpeg", "libx264", "gbrp", "cbr", 19, true},
		// Planar RGB reaches the ffmpeg encoders and none of the GStreamer elements,
		// so the same codec and chroma passes on one engine and fails on the other.
		{"nvenc hevc rgb over gstreamer", "gstreamer", "hevc_nvenc", "gbrp", "crf", 19, true},
		{"x265 rgb over ffmpeg", "ffmpeg", "libx265", "gbrp", "crf", 19, false},
		{"x265 rgb over gstreamer", "gstreamer", "libx265", "gbrp", "crf", 19, true},
		{"x265 4:4:4 over gstreamer", "gstreamer", "libx265", "yuv444p", "crf", 19, false},
		// 10-bit libaom AV1 is the ffmpeg engine's alone (av1enc takes 8-bit input).
		{"libaom 10-bit over ffmpeg", "ffmpeg", "libaom-av1", "p010le", "crf", 19, false},
		{"libaom 10-bit over gstreamer", "gstreamer", "libaom-av1", "p010le", "crf", 19, true},
		{"libaom 8-bit over gstreamer", "gstreamer", "libaom-av1", "yuv420p", "crf", 19, false},
		// libvpx counts its quantizer to 63, the H.26x encoders to 51, so the same
		// value passes on one and fails on the other.
		{"vp9 quantizer at 60", "ffmpeg", "libvpx-vp9", "yuv444p", "crf", 60, false},
		{"x264 quantizer at 60", "ffmpeg", "libx264", "yuv420p", "crf", 60, true},
		{"negative quantizer", "ffmpeg", "libx264", "yuv420p", "crf", -1, true},
		// rav1e's quantizer counts to 255, the widest scale in the table.
		{"rav1e quantizer at 200", "ffmpeg", "librav1e", "yuv420p", "crf", 200, false},
		// The quantizer reaches the encoder in crf mode only, so a stale value from
		// another codec's scale must not block a bitrate mode.
		{"out-of-scale quantizer outside crf", "ffmpeg", "libx264", "yuv420p", "cbr", 60, false},
		// VP8 has no lossless mode on either engine; VP9's is ffmpeg's alone.
		{"vp8 lossless", "ffmpeg", "libvpx", "yuv420p", "lossless", 19, true},
		{"vp9 lossless over ffmpeg", "ffmpeg", "libvpx-vp9", "yuv444p", "lossless", 19, false},
		{"vp9 lossless over gstreamer", "gstreamer", "libvpx-vp9", "yuv444p", "lossless", 19, true},
		{"av1 lossless", "gstreamer", "libsvtav1", "yuv420p", "lossless", 19, true},
		// No VAAPI encoder codes bit-exact, on either engine, while its bitrate and
		// constant-quality modes are the drivers' own.
		{"vaapi lossless over ffmpeg", "ffmpeg", "h264_vaapi", "yuv420p", "lossless", 19, true},
		{"vaapi lossless over gstreamer", "gstreamer", "h264_vaapi", "yuv420p", "lossless", 19, true},
		{"vaapi cbr", "ffmpeg", "h264_vaapi", "yuv420p", "cbr", 19, false},
		// The VAAPI AV1 quantizer is a 0-255 index, where its H.26x rows stop at 51.
		{"vaapi av1 quantizer at 200", "ffmpeg", "av1_vaapi", "yuv420p", "crf", 200, false},
		{"vaapi h264 quantizer at 200", "ffmpeg", "h264_vaapi", "yuv420p", "crf", 200, true},
	}
	for _, tc := range cases {
		// Bitrate target zero: no row above turns on a codec's bitrate ceiling, which
		// TestValidateBitrateCeiling covers on its own.
		err := Validate(tc.engine, tc.codec, tc.chroma, tc.mode, tc.cq, 0)
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
		chroma := c.EngineChromas("ffmpeg")[0]
		if err := Validate("ffmpeg", c.Name, chroma, "cbr", 0, c.BitrateLimitM+1); err == nil {
			t.Errorf("%s must reject a bitrate target above its %d Mbit/s ceiling", c.Name, c.BitrateLimitM)
		}
		if err := Validate("ffmpeg", c.Name, chroma, "cbr", 0, c.BitrateLimitM); err != nil {
			t.Errorf("%s at its ceiling: %v", c.Name, err)
		}
		// crf sends no bitrate, so a stale target must not block the encode.
		if _, gap := c.ModeGap("ffmpeg", "crf"); !gap {
			if err := Validate("ffmpeg", c.Name, chroma, "crf", 0, c.BitrateLimitM*2); err != nil {
				t.Errorf("%s in crf with a stale bitrate target: %v", c.Name, err)
			}
		}
	}
}

// A relay path reports its track's format and never says which encoder produced
// it, so the watch side reads formats rather than codec names. Every format an
// implemented row produces has to be one HasFormat knows, and a format no row
// produces must not answer, since that answer is what keeps a stale relay
// snapshot from narrowing a viewer's choice to nothing.
func TestHasFormat(t *testing.T) {
	for _, format := range Formats() {
		if !HasFormat(format) {
			t.Errorf("format %s is produced here but unknown to HasFormat", format)
		}
	}
	if HasFormat("mpeg2") {
		t.Error("a format no row produces must not answer")
	}
	if HasFormat("") {
		t.Error("the empty format must not answer")
	}
}
