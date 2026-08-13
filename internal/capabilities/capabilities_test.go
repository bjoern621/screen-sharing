package capabilities

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"slices"
	"testing"
)

// The family is a column of the table and never a reading of the codec's name,
// so a name that ends in a family's spelling belongs to whatever family its row declares,
// and a name that is not in the table has none at all.
func TestFamilyComesFromTheTable(t *testing.T) {
	cases := map[string]string{
		"hevc_nvenc": FamilyNvenc,
		"libx264":    FamilySoftware,
		"h264_qsv":   FamilyQsv,
		"_nvenc":     "", // not a real codec name
		"nvenc":      "",
		"vaapi":      "",
	}
	for codec, want := range cases {
		c, ok := Get(codec)
		if ok != (want != "") {
			t.Errorf("Get(%q) found = %v, want %v", codec, ok, want != "")
			continue
		}
		if ok && c.Family != want {
			t.Errorf("%s is family %q, want %q", codec, c.Family, want)
		}
	}
}

// Every row's family is one the builders can key a table by.
// A row declaring a family outside the set reaches no family-wide mapping and would fall to
// whatever the builder does with an unknown one.
func TestEveryRowDeclaresAKnownFamily(t *testing.T) {
	for _, c := range Codecs {
		if !slices.Contains(Families, c.Family) {
			t.Errorf("%s declares family %q, which is not one of %v", c.Name, c.Family, Families)
		}
	}
}

// gappableValues is the value space of every option a gap may name, which a gap's value has to come
// out of.
// Chroma has no entry: its values are per codec, so a chroma gap is checked against the row's own
// Chromas instead.
var gappableValues = map[string][]string{
	OptionMode:       Modes,
	OptionColorRange: {"pc", "tv"},
}

// A gap binds by matching the value a lookup is given, so everything it names has to be spelled the
// way the lookups are asked.
// A gap naming an engine, an option or a value outside the set matches nothing,
// and the capability it was written to withhold is then offered as if the encoder had it.
func TestEveryGapNamesAKnownOptionAndValue(t *testing.T) {
	for _, c := range Codecs {
		for _, g := range c.Gaps {
			if g.Engine != "" && !slices.Contains(Engines, g.Engine) {
				t.Errorf("%s has a gap on engine %q, which is not one of %v", c.Name, g.Engine, Engines)
			}
			// The option and the value come as a pair: an option with no value withholds nothing,
			// and a value with no option reads as the engine-wide gap, which takes the codec off the engine
			// entirely.
			if (g.Option == "") != (g.Value == "") {
				t.Errorf("%s has a gap on option %q with value %q: an option gap names both", c.Name, g.Option, g.Value)
				continue
			}
			if g.Option == "" {
				continue
			}
			if !slices.Contains(Options, g.Option) {
				t.Errorf("%s has a gap on option %q, which is not one of %v", c.Name, g.Option, Options)
				continue
			}
			// A chroma gap withholds a format the row offers.
			// One naming a format the row does not list withholds nothing, and states a reason for an option
			// that was never on offer.
			values, ok := gappableValues[g.Option]
			if !ok {
				values = c.Chromas
			}
			if !slices.Contains(values, g.Value) {
				t.Errorf("%s has a gap on %s %q, which is not one of %v", c.Name, g.Option, g.Value, values)
			}
		}
	}
}

// Every option a gap may name has to state how its refusal reads, or Validate would have a gap it
// can match and no way to report it.
func TestEveryOptionStatesItsRefusal(t *testing.T) {
	for _, option := range Options {
		if _, ok := optionRefusals[option]; !ok {
			t.Errorf("option %q has no refusal phrase", option)
		}
	}
	for option := range optionRefusals {
		if !slices.Contains(Options, option) {
			t.Errorf("refusal phrase for %q, which is not a gappable option", option)
		}
	}
}

// A rate-control mode the table does not know reaches no gap and no quantizer scale,
// and the builders would run it as CBR, so it is refused where every other unknown value is.
func TestValidateRejectsAnUnknownMode(t *testing.T) {
	if err := Validate(EngineFfmpeg, "libx264", options("yuv420p", "constant-effort", "pc"), 19, 20); err == nil {
		t.Error("Validate must reject a rate-control mode the table does not carry")
	}
	for _, mode := range Modes {
		if _, gap := mustGet(t, "libx264").OptionGap(EngineFfmpeg, OptionMode, mode); gap {
			continue
		}
		if err := Validate(EngineFfmpeg, "libx264", options("yuv420p", mode, "pc"), 19, 20); err != nil {
			t.Errorf("libx264 in %s: %v", mode, err)
		}
	}
}

// options is one validation's option values, keyed as Validate is given them.
func options(chroma, mode, colorRange string) map[string]string {
	return map[string]string{
		OptionChroma:     chroma,
		OptionMode:       mode,
		OptionColorRange: colorRange,
	}
}

func mustGet(t *testing.T, name string) Codec {
	t.Helper()
	c, ok := Get(name)
	if !ok {
		t.Fatalf("%s is not in the table", name)
	}
	return c
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
		// Planar RGB is the ffmpeg engine's on every RGB-coding codec but one: x265enc,
		// vp9enc and av1enc negotiate YUV alone, so those keep the format on one engine.
		// The nvcodec HEVC elements are the exception, taking a GBR sink format and coding it in the
		// Range Extensions profile, so hevc_nvenc reaches RGB on both.
		{"hevc_nvenc", "gstreamer", "gbrp", true},
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
		// QSV reaches 10-bit where the format has a Main-10 equivalent, and reaches it on both engines:
		// the qsv elements take the same semi-planar surfaces the encoders read.
		// Neither 4:4:4 nor RGB is on any QSV row.
		{"hevc_qsv", "gstreamer", "p010le", true},
		{"av1_qsv", "ffmpeg", "p010le", true},
		{"h264_qsv", "ffmpeg", "p010le", false}, // no QSV H.264 encoder codes High 10
		{"vp9_qsv", "ffmpeg", "p010le", false},
		{"hevc_qsv", "ffmpeg", "yuv444p", false},
	}
	for _, tc := range cases {
		if got := SupportsChroma(tc.codec, tc.engine, tc.chroma); got != tc.want {
			t.Errorf("SupportsChroma(%q,%q,%q) = %v, want %v", tc.codec, tc.engine, tc.chroma, got, tc.want)
		}
	}
}

// The chromas a codec reaches on one engine are its table order minus that engine's gaps,
// so a caller that needs a working format (the encoder tests, the frontend's fallback walk) can
// take the first entry.
func TestEngineChromas(t *testing.T) {
	cases := []struct {
		codec, engine string
		want          []string
	}{
		// The two software H.26x rows are where the middle subsampling lives: x264 codes High 4:2:2 and
		// x265 Main 4:2:2 10, and no hardware encoder here reaches either.
		{"libx265", "ffmpeg", []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"}},
		{"libx265", "gstreamer", []string{"yuv444p", "yuv422p", "yuv420p", "p010le"}},
		{"libaom-av1", "ffmpeg", []string{"gbrp", "yuv444p", "yuv420p", "p010le"}},
		{"libaom-av1", "gstreamer", []string{"yuv444p", "yuv420p"}},
		{"libx264", "gstreamer", []string{"yuv444p", "yuv422p", "yuv420p", "p010le"}},
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

// A gap belongs to the option it names: the pixel-format lookup must not answer a rate-control
// question or the reverse, however alike the two values are spelled.
// One lookup serves every option, so this is what keeps it keyed by both.
func TestGapsDoNotCrossOptions(t *testing.T) {
	aom, _ := Get("libaom-av1")
	if _, gap := aom.OptionGap("gstreamer", OptionMode, "p010le"); gap {
		t.Error("a chroma gap must not answer a rate-control lookup")
	}
	if _, gap := aom.OptionGap("gstreamer", OptionChroma, "lossless"); gap {
		t.Error("a mode gap must not answer a pixel-format lookup")
	}
	// A gap naming no option takes the codec off that engine.
	// No row needs one, since both builders map every implemented codec, so the shape is checked on a
	// value rather than on the table.
	oneEngine := Codec{
		Name: "hypothetical",
		Gaps: []Gap{{Engine: "gstreamer", Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_ELEMENTS_NO_PLANAR_RGB}},
	}
	if _, gap := oneEngine.EngineGap("gstreamer"); !gap {
		t.Error("a gap naming no option must take the codec off its engine")
	}
	if _, gap := oneEngine.EngineGap("ffmpeg"); gap {
		t.Error("an engine gap must not bind on the other engine")
	}
	if _, gap := oneEngine.OptionGap("gstreamer", OptionChroma, "yuv420p"); gap {
		t.Error("an engine gap must not answer a pixel-format lookup")
	}
}

func TestValidate(t *testing.T) {
	// A case that names no colour range is not about one, and full range is what the settings default
	// to, so that is what the loop below validates it with.
	cases := []struct {
		name                        string
		engine, codec, chroma, mode string
		colorRange                  string
		cq                          int
		wantErr                     bool
	}{
		{"nvenc hevc lossless", "ffmpeg", "hevc_nvenc", "gbrp", "lossless", "", 19, false},
		{"unknown codec", "ffmpeg", "nope", "yuv420p", "cbr", "", 19, true},
		{"unimplemented family", "ffmpeg", "hevc_v4l2m2m", "yuv420p", "cbr", "", 19, true},
		{"chroma the codec rejects", "ffmpeg", "libx264", "gbrp", "cbr", "", 19, true},
		// Planar RGB reaches the ffmpeg encoders and all but one GStreamer element,
		// so the same codec and chroma passes on one engine and fails on the other for the software rows.
		// The nvcodec HEVC elements take GBR, so that row passes on both.
		{"nvenc hevc rgb over gstreamer", "gstreamer", "hevc_nvenc", "gbrp", "crf", "", 19, false},
		{"x265 rgb over ffmpeg", "ffmpeg", "libx265", "gbrp", "crf", "", 19, false},
		{"x265 rgb over gstreamer", "gstreamer", "libx265", "gbrp", "crf", "", 19, true},
		{"x265 4:4:4 over gstreamer", "gstreamer", "libx265", "yuv444p", "crf", "", 19, false},
		// 10-bit libaom AV1 is the ffmpeg engine's alone (av1enc takes 8-bit input).
		{"libaom 10-bit over ffmpeg", "ffmpeg", "libaom-av1", "p010le", "crf", "", 19, false},
		{"libaom 10-bit over gstreamer", "gstreamer", "libaom-av1", "p010le", "crf", "", 19, true},
		// Limited range, because the case is about the chroma: av1enc reaches 8-bit and signals no colour
		// description, which the two rows below are about.
		{"libaom 8-bit over gstreamer", "gstreamer", "libaom-av1", "yuv420p", "crf", "tv", 19, false},
		// libvpx counts its quantizer to 63, the H.26x encoders to 51, so the same value passes on one
		// and fails on the other.
		{"vp9 quantizer at 60", "ffmpeg", "libvpx-vp9", "yuv444p", "crf", "", 60, false},
		{"x264 quantizer at 60", "ffmpeg", "libx264", "yuv420p", "crf", "", 60, true},
		{"negative quantizer", "ffmpeg", "libx264", "yuv420p", "crf", "", -1, true},
		// rav1e's quantizer counts to 255, the widest scale in the table.
		{"rav1e quantizer at 200", "ffmpeg", "librav1e", "yuv420p", "crf", "", 200, false},
		// The quantizer reaches the encoder in crf mode only, so a stale value from another codec's scale
		// must not block a bitrate mode.
		{"out-of-scale quantizer outside crf", "ffmpeg", "libx264", "yuv420p", "cbr", "", 60, false},
		// VP8 has no lossless mode on either engine; VP9's is ffmpeg's alone.
		{"vp8 lossless", "ffmpeg", "libvpx", "yuv420p", "lossless", "", 19, true},
		{"vp9 lossless over ffmpeg", "ffmpeg", "libvpx-vp9", "yuv444p", "lossless", "", 19, false},
		{"vp9 lossless over gstreamer", "gstreamer", "libvpx-vp9", "yuv444p", "lossless", "", 19, true},
		{"av1 lossless", "gstreamer", "libsvtav1", "yuv420p", "lossless", "", 19, true},
		// No VAAPI encoder codes bit-exact, on either engine, while its bitrate and constant-quality
		// modes are the drivers' own.
		{"vaapi lossless over ffmpeg", "ffmpeg", "h264_vaapi", "yuv420p", "lossless", "", 19, true},
		{"vaapi lossless over gstreamer", "gstreamer", "h264_vaapi", "yuv420p", "lossless", "", 19, true},
		{"vaapi cbr", "ffmpeg", "h264_vaapi", "yuv420p", "cbr", "", 19, false},
		// The VAAPI AV1 quantizer is a 0-255 index, where its H.26x rows stop at 51.
		{"vaapi av1 quantizer at 200", "ffmpeg", "av1_vaapi", "yuv420p", "crf", "", 200, false},
		{"vaapi h264 quantizer at 200", "ffmpeg", "h264_vaapi", "yuv420p", "crf", "", 200, true},
		// The va elements signal no colour description, so a full-range VAAPI stream would be read as
		// limited by every viewer.
		// The same hardware reaches full range through the ffmpeg engine, which tags the frames.
		{"vaapi full range over gstreamer", "gstreamer", "h264_vaapi", "yuv420p", "cbr", "pc", 19, true},
		{"vaapi limited range over gstreamer", "gstreamer", "h264_vaapi", "yuv420p", "cbr", "tv", 19, false},
		{"vaapi full range over ffmpeg", "ffmpeg", "h264_vaapi", "yuv420p", "cbr", "pc", 19, false},
		{"software full range over gstreamer", "gstreamer", "libx264", "yuv420p", "cbr", "pc", 19, false},
		// av1enc signals no colour description either, and the same library reaches full range through
		// the ffmpeg engine.
		{"libaom full range over gstreamer", "gstreamer", "libaom-av1", "yuv420p", "cbr", "pc", 19, true},
		{"libaom full range over ffmpeg", "ffmpeg", "libaom-av1", "yuv420p", "cbr", "pc", 19, false},
		// VP8 has no colour range field at all, so no engine reaches full range with it.
		{"vp8 full range over ffmpeg", "ffmpeg", "libvpx", "yuv420p", "cbr", "pc", 19, true},
		{"vp8 full range over gstreamer", "gstreamer", "libvpx", "yuv420p", "cbr", "pc", 19, true},
		{"vp8 limited range over ffmpeg", "ffmpeg", "libvpx", "yuv420p", "cbr", "tv", 19, false},
	}
	for _, tc := range cases {
		colorRange := tc.colorRange
		if colorRange == "" {
			colorRange = "pc"
		}
		// Bitrate target zero: no row above turns on a codec's bitrate ceiling, which
		// TestValidateBitrateCeiling covers on its own.
		err := Validate(tc.engine, tc.codec, options(tc.chroma, tc.mode, colorRange), tc.cq, 0)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: Validate = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

// A codec declaring a bitrate ceiling rejects a target above it, and only in the modes that send
// one.
// Every codec with no ceiling takes any target.
func TestValidateBitrateCeiling(t *testing.T) {
	for _, c := range Codecs {
		limit := c.BitrateLimitOn(EngineFfmpeg)
		if !c.Implemented || limit == 0 {
			continue
		}
		chroma := c.EngineChromas(EngineFfmpeg)[0]
		if err := Validate(EngineFfmpeg, c.Name, options(chroma, "cbr", "pc"), 0, limit+1); err == nil {
			t.Errorf("%s must reject a bitrate target above its %d Mbit/s ceiling", c.Name, limit)
		}
		if err := Validate(EngineFfmpeg, c.Name, options(chroma, "cbr", "pc"), 0, limit); err != nil {
			t.Errorf("%s at its ceiling: %v", c.Name, err)
		}
		// crf sends no bitrate, so a stale target must not block the encode.
		if _, gap := c.OptionGap(EngineFfmpeg, OptionMode, "crf"); !gap {
			if err := Validate(EngineFfmpeg, c.Name, options(chroma, "crf", "pc"), 0, limit*2); err != nil {
				t.Errorf("%s in crf with a stale bitrate target: %v", c.Name, err)
			}
		}
	}
}

// The two numeric limits are read per engine, and a missing entry reads as zero,
// which both lookups spell "no bound declared".
// A row naming one engine and not the other therefore validates unbounded on the engine it forgot:
// a quantizer off the encoder's scale and a bitrate above its ceiling both reach the command,
// and the refusal that should have named the value never fires.
// So a limit covers every engine or none of them, and names no engine outside the set,
// where a misspelled key is the same silent zero.
func TestNumericLimitsCoverEveryEngineOrNone(t *testing.T) {
	for _, c := range Codecs {
		limits := map[string]map[string]int{
			"CqMax":         c.CqMax,
			"BitrateLimitM": c.BitrateLimitM,
		}
		for field, byEngine := range limits {
			for engine := range byEngine {
				if !slices.Contains(Engines, engine) {
					t.Errorf("%s declares %s for engine %q, which is not one of %v", c.Name, field, engine, Engines)
				}
			}
			if len(byEngine) == 0 {
				continue
			}
			for _, engine := range Engines {
				if _, ok := byEngine[engine]; !ok {
					t.Errorf("%s declares %s on some engines and not on %s, which then reads as no bound at all",
						c.Name, field, engine)
				}
			}
		}
	}
}

// A relay path reports its track's format and never says which encoder produced it,
// so the watch side reads formats rather than codec names.
// Every format an implemented row produces has to be one HasFormat knows, and a format no row
// produces must not answer, since that answer is what keeps a stale relay snapshot from narrowing a
// viewer's choice to nothing.
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
