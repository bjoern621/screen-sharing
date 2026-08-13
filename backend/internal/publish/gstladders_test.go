package publish

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// gstEffortProperties is the property each element spells its effort knob with.
//
// Stated rather than derived, being this engine's spelling of a fact the table holds in the
// encoder's own vocabulary: the x26x elements say speed-preset where ffmpeg says -preset,
// and svtav1enc says preset where rav1enc says speed-preset again.
// The table declares the step; which property carries it is the builder's.
var gstEffortProperties = map[string]string{
	"libx264":    "speed-preset",
	"libx265":    "speed-preset",
	"libvpx":     "cpu-used",
	"libvpx-vp9": "cpu-used",
	"libaom-av1": "cpu-used",
	"libsvtav1":  "preset",
	"librav1e":   "speed-preset",
	// The nvcodec elements take the same p1-p7 steps ffmpeg does.
	// Their preset enum carries the ladder beside the presets it deprecates,
	// those entries reading "Default (deprecated, use p1~7 with tune)".
	"h264_nvenc": "preset",
	"hevc_nvenc": "preset",
	"av1_nvenc":  "preset",
	// The qsv elements take oneVPL's target usage as the number its own scale defines,
	// where the ffmpeg engine names the same seven points.
	"h264_qsv": "target-usage",
	"hevc_qsv": "target-usage",
	"av1_qsv":  "target-usage",
	"vp9_qsv":  "target-usage",
}

// The ladders state what the elements spend, the claim
// ffmpeg.TestTheLaddersStateWhatTheBuildersSpend makes about the other builder.
//
// One library reached through two bindings encodes alike, so both engines read one table rather
// than each holding constants of its own: a stream's look must not follow from the capture backend
// that produced it.
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

// A codec this engine builds spends an effort step exactly where its row declares a ladder.
//
// The two directions fail differently: a ladder nothing spends is a control the form offers and the
// encode ignores, which the availability contract rules out,
// and a step spent off a row declaring none is a property value nothing chose.
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

// A tune step travels in whichever property the element spells it with, which is not ffmpeg's
// spelling.
//
// x264enc splits x264's list across two properties: tune takes the tunes that change what it codes,
// psy-tune the ones weighing what the eye sees.
// x265enc takes one enum whose zero entry is spelled by number, leaving the property off meaning
// ssim on that element rather than no tuning.
// The nvcodec elements spell the SDK's tunes in full words, where the row and ffmpeg use its
// abbreviations.
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

// mustGstEncoder builds one element's properties for one mode on a draft the codec accepts:
// the quantizer rides its own scale and the bitrate stays under its ceiling,
// so nothing but the knob under test can refuse it.
func mustGstEncoder(t *testing.T, c capabilities.Codec, mode string) []string {
	t.Helper()

	chromas := c.EngineChromas(capabilities.EngineGst)
	if len(chromas) == 0 {
		t.Skipf("%s codes nothing on this engine", c.Name)
	}

	s := baseStream()
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = c.Name, mode, chromas[len(chromas)-1]
	// Both steps come off the codec's own row, as a fresh installation, the migration and the repair
	// do.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(c.Name, mode)
	s.Publish.Cq = c.CqMaxOn(capabilities.EngineGst) / 2
	if limit := c.BitrateLimitOn(capabilities.EngineGst); limit > 0 && s.Publish.BitrateM > limit {
		s.Publish.BitrateM = limit
	}
	// Under the bound the elements' own properties impose too, which the capability table does not
	// carry: the AV1 and VP9 qsv elements state their rate in an unsigned 16-bit field,
	// and a draft above it is refused before a ladder step is spent (qsvShortRateLimits).
	// The step is the question, so the rate is put where it can be reached.
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

// assertProperty holds one element property to the value the table declares.
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

// tuneProperties is whichever tune property an element carries, and empty where it carries neither:
// x264enc spells one ladder on two properties, and which of them a step lands on is the claim under
// test.
func tuneProperties(encoder []string) string {
	for _, p := range encoder {
		if strings.HasPrefix(p, "tune=") || strings.HasPrefix(p, "psy-tune=") {
			return p
		}
	}
	return ""
}

// elementRateCeilingM is a rate every element in the table accepts, in Mbit/s.
//
// The lowest bound any of them imposes, over the largest factor a mode places its ceiling at above
// the target: the abr mappings ask for headroom above the rate, so a draft exactly at the bound is
// one the ceiling then exceeds.
// One figure for every codec rather than a lookup per row, since what it is for is reaching the
// question: a rate refused before a ladder step is spent answers a different one.
var elementRateCeilingM = qsvShortBitrateKbps / 1000 / qsvAbrPeak
