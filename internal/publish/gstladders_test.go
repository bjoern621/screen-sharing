package publish

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The property each element spells its effort knob with.
//
// They are stated here rather than derived because they are this engine's spelling of a
// fact the table holds in the encoder's own vocabulary: the x26x elements say
// speed-preset where ffmpeg says -preset, and svtav1enc says preset where rav1enc says
// speed-preset again. What the table declares is the step; which property carries it is
// the builder's.
var gstEffortProperties = map[string]string{
	"libx264":    "speed-preset",
	"libx265":    "speed-preset",
	"libvpx":     "cpu-used",
	"libvpx-vp9": "cpu-used",
	"libaom-av1": "cpu-used",
	"libsvtav1":  "preset",
	"librav1e":   "speed-preset",
	// The nvcodec elements take the same p1-p7 steps ffmpeg does. Their preset enum
	// carries the ladder beside the presets it deprecates, and the deprecated entries say
	// so themselves: "Default (deprecated, use p1~7 with tune)".
	"h264_nvenc": "preset",
	"hevc_nvenc": "preset",
	"av1_nvenc":  "preset",
	// The qsv elements take oneVPL's target usage as the number the scale itself defines,
	// where the ffmpeg engine names the same seven points.
	"h264_qsv": "target-usage",
	"hevc_qsv": "target-usage",
	"av1_qsv":  "target-usage",
	"vp9_qsv":  "target-usage",
}

// The ladders state what the elements spend, which is the same claim
// ffmpeg.TestTheLaddersStateWhatTheBuildersSpend makes about the other builder.
//
// One library reached through two bindings has to encode alike, so the two engines read
// one table rather than each holding its own constants: a stream's look must not depend on
// which capture backend produced it.
func TestTheLaddersStateWhatTheElementsSpend(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		property, spends := gstEffortProperties[c.Name]
		if !spends {
			continue
		}
		for _, mode := range capabilities.Modes {
			if !capabilities.Reaches(c.Name, capabilities.EngineGst, capabilities.OptionMode, mode) {
				continue
			}
			t.Run(c.Name+"/"+mode, func(t *testing.T) {
				step, declared := c.Effort.StepFor(mode)
				if !declared {
					t.Fatalf("%s declares no effort step for %s, and the element spends one", c.Name, mode)
				}
				assertProperty(t, mustGstEncoder(t, c, mode), property, step)
			})
		}
	}
}

// A codec this engine builds spends an effort step exactly when its row declares a ladder.
//
// The two directions are different failures. A ladder nothing spends is a control the form
// offers and the encode ignores, which is what the availability contract rules out; a step
// spent off a row that declares none is a property value nothing chose.
func TestEveryLadderIsSpent(t *testing.T) {
	for name := range gstCodecs {
		c, ok := capabilities.Get(name)
		if !ok {
			t.Errorf("%s has a GStreamer mapping but no capability row", name)
			continue
		}
		_, spends := gstEffortProperties[name]
		if declared := len(c.Effort.Steps) > 0; spends != declared {
			t.Errorf("%s: the element spends an effort step=%v and the row declares a ladder=%v",
				name, spends, declared)
		}
	}
}

// A tune step travels in whichever property the element spells it with, which is not what
// ffmpeg spells it as.
//
// x264enc splits x264's list across two properties: tune takes the three tunes that change
// what it codes, psy-tune the five that weigh what the eye sees. x265enc takes one enum
// whose zero entry has to be spelled by number, because leaving the property off means ssim
// on that element rather than no tuning. The nvcodec elements spell the SDK's four in full
// words, where the row and ffmpeg use the SDK's abbreviations.
func TestTheTuneStepTravelsInTheElementsOwnProperty(t *testing.T) {
	for _, tc := range []struct {
		codec, step, want string
	}{
		{"libx264", "zerolatency", "tune=zerolatency"},
		{"libx264", "grain", "psy-tune=grain"},
		{"libx264", capabilities.TuneNone, ""},
		{"libx265", "zerolatency", "tune=zerolatency"},
		{"libx265", "fastdecode", "tune=fastdecode"},
		{"libx265", capabilities.TuneNone, "tune=0"},
		{"hevc_nvenc", "hq", "tune=high-quality"},
		{"hevc_nvenc", "ll", "tune=low-latency"},
		{"hevc_nvenc", "ull", "tune=ultra-low-latency"},
		{"hevc_nvenc", "lossless", "tune=lossless"},
	} {
		c, ok := capabilities.Get(tc.codec)
		if !ok {
			t.Fatalf("no capability row for %s", tc.codec)
		}
		if !c.Tune.Has(tc.step) {
			t.Errorf("%s no longer declares the tune step %q", tc.codec, tc.step)
			continue
		}

		s := baseStream()
		s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = tc.codec, capabilities.ModeCrf, "yuv420p"
		s.Publish.Cq = c.CqMaxOn(capabilities.EngineGst) / 2
		s.Publish.Tune = tc.step

		encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
		if err != nil {
			t.Fatalf("%s tuned for %s: %v", tc.codec, tc.step, err)
		}
		if got := tuneProperties(encoder); got != tc.want {
			t.Errorf("%s tuned for %s carries %q, want %q", tc.codec, tc.step, got, tc.want)
		}
	}
}

// mustGstEncoder builds one element's properties for one mode, on a draft the codec
// accepts: the quantizer rides its own scale and the bitrate stays under its ceiling, so
// what comes back is refused for nothing but the knob under test.
func mustGstEncoder(t *testing.T, c capabilities.Codec, mode string) []string {
	t.Helper()

	chromas := c.EngineChromas(capabilities.EngineGst)
	if len(chromas) == 0 {
		t.Skipf("%s codes nothing on this engine", c.Name)
	}

	s := baseStream()
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = c.Name, mode, chromas[len(chromas)-1]
	// The two ladder steps come off the codec's own row, which is what a fresh
	// installation, the migration and the repair all do.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(c.Name, mode)
	s.Publish.Cq = c.CqMaxOn(capabilities.EngineGst) / 2
	if limit := c.BitrateLimitOn(capabilities.EngineGst); limit > 0 && s.Publish.BitrateM > limit {
		s.Publish.BitrateM = limit
	}
	// Under the bound the element's own properties impose as well, which the capability
	// table does not carry: two of the qsv elements state their rate in an unsigned 16-bit
	// field, and a draft above it is refused before any ladder step is spent
	// (qsvShortRateLimits). What this asks about is the step, so the rate is put where the
	// question can be reached.
	if elementRateCeilingM > 0 && s.Publish.BitrateM > elementRateCeilingM {
		s.Publish.BitrateM = elementRateCeilingM
		s.Publish.MaxrateM = elementRateCeilingM
	}

	encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
	if err != nil {
		t.Fatalf("%s in %s: %v", c.Name, mode, err)
	}
	return encoder
}

// assertProperty holds one element property's value to what the table declares.
func assertProperty(t *testing.T, encoder []string, property, want string) {
	t.Helper()

	for _, p := range encoder {
		if value, ok := strings.CutPrefix(p, property+"="); ok {
			if value != want {
				t.Errorf("the element takes %s=%s, where the table declares %q", property, value, want)
			}
			return
		}
	}
	t.Errorf("the element takes no %s, where the table declares %q", property, want)
}

// tuneProperties is whichever tune property an element carries, and the empty string where
// it carries neither: two properties spell one ladder on x264enc, and which one a step
// lands on is the claim under test.
func tuneProperties(encoder []string) string {
	for _, p := range encoder {
		if strings.HasPrefix(p, "tune=") || strings.HasPrefix(p, "psy-tune=") {
			return p
		}
	}
	return ""
}

// elementRateCeilingM is a rate every element in the table accepts, in megabits.
//
// It is the lowest bound any of them imposes, divided by the largest factor a mode places a
// ceiling at above the target: the abr mappings ask for headroom above the rate, so a draft
// exactly at the bound is one the ceiling then exceeds. One figure for every codec rather
// than a lookup per row, because what it is for is reaching the question - this asks about
// ladder steps, and a rate refused before the step is spent answers a different one.
var elementRateCeilingM = qsvShortBitrateKbps / 1000 / qsvAbrPeak
