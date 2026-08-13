package ffmpeg

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// The flag each encoder spells its effort knob with, and the one it spells its tune with.
//
// Stated rather than derived: they are this engine's spelling of a fact the table holds in the
// encoder's own vocabulary, ffmpeg saying -preset where the GStreamer elements say speed-preset,
// and the aom family -cpu-used where rav1e says -speed.
// The table declares the step; which flag carries it is the builder's.
var (
	effortFlags = map[string]string{
		"libx264":    "-preset",
		"libx265":    "-preset",
		"libvpx":     "-cpu-used",
		"libvpx-vp9": "-cpu-used",
		"libaom-av1": "-cpu-used",
		"libsvtav1":  "-preset",
		"librav1e":   "-speed",
		"hevc_nvenc": "-preset",
		"h264_nvenc": "-preset",
		"av1_nvenc":  "-preset",
		// ffmpeg names the seven points of oneVPL's target-usage scale, so the ladder's numbers reach
		// this engine through the map that spells them (qsvPresets).
		"h264_qsv": "-preset",
		"hevc_qsv": "-preset",
		"av1_qsv":  "-preset",
		"vp9_qsv":  "-preset",
		// All three AMF encoders spell the scale alike, so a step reaches them verbatim.
		"h264_amf": "-quality",
		"hevc_amf": "-quality",
		"av1_amf":  "-quality",
	}
	tuneFlags = map[string]string{
		"libx264":     "-tune",
		"libx265":     "-tune",
		"hevc_nvenc":  "-tune",
		"h264_nvenc":  "-tune",
		"av1_nvenc":   "-tune",
		"h264_vulkan": "-tune",
		"hevc_vulkan": "-tune",
		"av1_vulkan":  "-tune",
	}
)

// A speed decision the table declares and a builder spends is one written in two places, so any
// drift between them fails here rather than in a stream nobody can explain.
func TestTheLaddersStateWhatTheBuildersSpend(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		for _, mode := range capabilities.Modes {
			if !capabilities.Reaches(c.Name, capabilities.EngineFfmpeg, capabilities.OptionMode, mode) {
				continue
			}
			t.Run(c.Name+"/"+mode, func(t *testing.T) {
				args := mustEncoderArgs(t, c, mode)

				if flag, ok := effortFlags[c.Name]; ok {
					step, declared := c.Effort.StepFor(mode)
					if !declared {
						t.Fatalf("%s declares no effort step for %s, and the builder spends one", c.Name, mode)
					}
					assertFlagValue(t, args, flag, effortSpelling(c, step))
				}
				if flag, ok := tuneFlags[c.Name]; ok {
					step, declared := c.Tune.StepFor(mode)
					if !declared || step == "none" {
						// An absent default means the builder leaves the knob unset in this mode.
						if i := slices.Index(args, flag); i >= 0 {
							t.Errorf("the builder tunes for %q where the table leaves the knob unset", args[i+1])
						}
						return
					}
					assertFlagValue(t, args, flag, step)
				}
			})
		}
	}
}

// A default off its own ladder is a control offering one set of values and starting on another.
func TestEveryDefaultIsAStepOfItsLadder(t *testing.T) {
	for _, c := range capabilities.Codecs {
		for _, ladder := range []struct {
			name string
			l    capabilities.Ladder
		}{{"effort", c.Effort}, {"tune", c.Tune}} {
			for mode, step := range ladder.l.Defaults {
				if !ladder.l.Has(step) {
					t.Errorf("%s: the %s ladder starts %s on %q, which is not a step it offers",
						c.Name, ladder.name, mode, step)
				}
			}
		}
	}
}

// A codec whose builder spends a step declares a ladder, and one that declares a ladder has a
// builder that spends it.
// The two maps above are the builders' side of that, so a codec missing from either is a knob the
// form offers and the command drops.
func TestEveryLadderHasABuilderThatSpendsIt(t *testing.T) {
	for _, c := range capabilities.Codecs {
		if !c.Implemented {
			continue
		}
		if _, ok := effortFlags[c.Name]; ok != (len(c.Effort.Steps) > 0) {
			t.Errorf("%s: the builder spends an effort step=%v and the table declares a ladder=%v",
				c.Name, ok, len(c.Effort.Steps) > 0)
		}
		if _, ok := tuneFlags[c.Name]; ok != (len(c.Tune.Steps) > 0) {
			t.Errorf("%s: the builder spends a tune=%v and the table declares a ladder=%v",
				c.Name, ok, len(c.Tune.Steps) > 0)
		}
	}
}

// mustEncoderArgs builds one encoder's arguments for one mode on a draft the codec accepts: the
// quantizer rides its own scale and the bitrate stays under its ceiling, so nothing but the knob
// under test can refuse it.
func mustEncoderArgs(t *testing.T, c capabilities.Codec, mode string) []string {
	t.Helper()

	s := baseStream()
	chromas := c.EngineChromas(capabilities.EngineFfmpeg)
	if len(chromas) == 0 {
		t.Skipf("%s codes nothing on this engine", c.Name)
	}
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = c.Name, mode, chromas[len(chromas)-1]
	// Both steps come off the codec's own row, as a fresh installation, the migration and the repair
	// leave them.
	// A draft carrying another codec's step is what the repair exists to move, and not what this asks
	// about.
	s.Publish.Effort, _ = c.Effort.StepFor(mode)
	s.Publish.Tune, _ = c.Tune.StepFor(mode)
	s.Publish.Cq = c.CqMaxOn(capabilities.EngineFfmpeg) / 2
	if limit := c.BitrateLimitOn(capabilities.EngineFfmpeg); limit > 0 && s.Publish.BitrateM > limit {
		s.Publish.BitrateM = limit
	}

	args, err := encoderArgs(s, gopFor(s))
	if err != nil {
		t.Fatalf("%s in %s: %v", c.Name, mode, err)
	}
	return args
}

// assertFlagValue holds one flag's value in args to what the table declares.
func assertFlagValue(t *testing.T, args []string, flag, want string) {
	t.Helper()

	i := slices.Index(args, flag)
	if i < 0 {
		t.Errorf("the builder passes no %s, where the table declares %q", flag, want)
		return
	}
	if i+1 >= len(args) {
		t.Fatalf("%s is the last argument and carries no value", flag)
	}
	if args[i+1] != want {
		t.Errorf("the builder passes %s %q, where the table declares %q", flag, args[i+1], want)
	}
}

// effortSpelling is how this engine writes one step of a codec's ladder.
//
// Almost every ladder reaches ffmpeg as the step itself, the two engines differing in what the
// option is called and not in what it carries.
// oneVPL is the exception: the scale it defines is a number where ffmpeg names its seven points,
// so this reads the builder's own map rather than a second one, which would be a second spelling
// free to disagree with what the encoder is given.
func effortSpelling(c capabilities.Codec, step string) string {
	if c.Family != capabilities.FamilyQsv {
		return step
	}
	return qsvPresets[step]
}
