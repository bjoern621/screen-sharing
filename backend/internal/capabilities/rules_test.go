package capabilities

import (
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/rules"
)

// The rules and the gaps are one table read two ways, and they answer alike for every codec,
// engine, option and value there is.
//
// Exhaustive rather than sampled on purpose: the gaps can only be retired once the two agree
// everywhere, and agreement on the rows somebody thought to list retires nothing.
func TestRulesAnswerWhatTheGapsAnswer(t *testing.T) {
	values := map[string][]string{
		OptionChroma:     chromasDeclared(),
		OptionMode:       Modes,
		OptionColorRange: ColorRanges,
		OptionTune:       tunesDeclared(),
	}

	for _, c := range Codecs {
		for _, engine := range Engines {
			for _, option := range Options {
				axis, ok := optionAxes[option]
				if !ok {
					t.Fatalf("option %s names no axis", option)
				}
				for _, value := range values[option] {
					_, gapped := c.OptionGap(engine, option, value)
					v := rules.EvaluateRules(codecFacts(c.Name, engine, ModeCrf), codecRules())
					if refused := !v.ValueEnabled(axis, value); refused != gapped {
						t.Errorf("%s on %s: the gap says %s=%s is gapped=%v, the rules say refused=%v",
							c.Name, engine, option, value, gapped, refused)
					}
				}
			}
		}
	}
}

// A codec an engine has no encoder for is refused as an entry of the codec control itself.
// That is the gap that names no option.
func TestAnEngineWideGapRefusesTheCodecEntry(t *testing.T) {
	for _, c := range Codecs {
		for _, engine := range Engines {
			_, gapped := c.EngineGap(engine)
			v := rules.EvaluateRules(codecFacts(c.Name, engine, ModeCrf), codecRules())
			if refused := !v.ValueEnabled(rules.AxisCodec, c.Name); refused != gapped {
				t.Errorf("%s on %s: the gap says the engine lacks it=%v, the rules say refused=%v",
					c.Name, engine, gapped, refused)
			}
		}
	}
}

// A gap belonging to the format or the library binds on both engines, and a gap belonging to one
// wrapper binds only there.
// The rules carry the distinction by naming the engine axis or leaving it out.
func TestAnEngineWideFactBindsOnBothEngines(t *testing.T) {
	// libvpx signals no colour range on either engine: VP8's keyframe header has no field for it,
	// which is the format's limit rather than one builder's.
	for _, engine := range Engines {
		v := rules.EvaluateRules(codecFacts("libvpx", engine, ModeCrf), codecRules())
		if v.ValueEnabled(rules.AxisColorRange, ColorRangeFull) {
			t.Errorf("full range stayed selectable for libvpx on %s", engine)
		}
	}

	// x265 codes planar RGB through ffmpeg and no GStreamer element takes it, so one value is one
	// engine's alone.
	if v := rules.EvaluateRules(codecFacts("libx265", EngineFfmpeg, ModeCrf), codecRules()); !v.ValueEnabled(rules.AxisChroma, "gbrp") {
		t.Error("planar RGB was refused on the engine that codes it")
	}
	if v := rules.EvaluateRules(codecFacts("libx265", EngineGst, ModeCrf), codecRules()); v.ValueEnabled(rules.AxisChroma, "gbrp") {
		t.Error("planar RGB stayed selectable on the engine whose elements do not take it")
	}
}

// A reason arrives naming the codec it is about.
// A statement that lost its subject is a greyed control the reader cannot act on.
func TestARefusalNamesTheCodecItIsAbout(t *testing.T) {
	v := rules.EvaluateRules(codecFacts("libvpx", EngineGst, ModeCrf), codecRules())

	reasons := v.ValueReasons(rules.AxisColorRange, ColorRangeFull)
	if len(reasons) == 0 {
		t.Fatal("a refused value says why")
	}
	var named string
	for _, a := range reasons[0].GetArgs() {
		if a.GetName().String() == "TEXT_ARG_NAME_CODEC" {
			named = a.GetId()
		}
	}
	if named != "libvpx" {
		t.Errorf("the refusal named codec %q", named)
	}
}

// codecFacts is the axes the codec table's rules match on.
// A rule binds on the axes it names, so naming more here would assert facts the table never
// claimed.
// The device goes in unidentified, which is what makes the gaps and the rules comparable: a gap is
// what the encoder cannot do on any machine, and a driver defect binds only where a driver was
// named, so a codec question with no driver in it reads the gaps alone.
func codecFacts(codec, engine, mode string) rules.Facts {
	return deviceCodecFacts(codec, engine, mode, Device{})
}

func deviceCodecFacts(codec, engine, mode string, device Device) rules.Facts {
	return rules.Facts{
		rules.AxisCodec:            rules.TextValue(codec),
		rules.AxisEngine:           rules.TextValue(engine),
		rules.AxisMode:             rules.TextValue(mode),
		rules.AxisGpuDriver:        rules.TextValue(device.Driver),
		rules.AxisGpuModel:         rules.TextValue(device.Model),
		rules.AxisGpuDriverVersion: rules.NumberValue(device.Version),
	}
}

// The offered range and the accepted value are one answer.
// For every codec and engine, what the rules narrow a control to is the top its column declares.
func TestRulesNarrowToTheCeilingsTheColumnsDeclare(t *testing.T) {
	const offered = 10000

	for _, c := range Codecs {
		for _, engine := range Engines {
			crf := rules.EvaluateRules(codecFacts(c.Name, engine, ModeCrf), codecRules())
			_, high := crf.Bounds(rules.AxisCq, 0, offered)
			want := c.CqMaxOn(engine)
			if want == 0 {
				want = offered
			}
			if high != want {
				t.Errorf("%s on %s: the quantizer is offered to %d, the column says %d",
					c.Name, engine, high, want)
			}

			abr := rules.EvaluateRules(codecFacts(c.Name, engine, ModeAbr), codecRules())
			_, high = abr.Bounds(rules.AxisBitrateM, 0, offered)
			want = c.BitrateLimitOn(engine)
			if want == 0 {
				want = offered
			}
			if high != want {
				t.Errorf("%s on %s: the bitrate is offered to %d, the column says %d",
					c.Name, engine, high, want)
			}
		}
	}
}

// A ceiling binds in the modes that read the knob and nowhere else.
func TestACeilingBindsOnlyInTheModesThatReadIt(t *testing.T) {
	const offered = 10000

	// libsvtav1 declares a bitrate ceiling, and a constant-quality encode sends no target for it to
	// apply to.
	crf := rules.EvaluateRules(codecFacts("libsvtav1", EngineFfmpeg, ModeCrf), codecRules())
	if _, high := crf.Bounds(rules.AxisBitrateM, 0, offered); high != offered {
		t.Errorf("the bitrate narrowed to %d in a mode that aims at no bitrate", high)
	}
	abr := rules.EvaluateRules(codecFacts("libsvtav1", EngineFfmpeg, ModeAbr), codecRules())
	if _, high := abr.Bounds(rules.AxisBitrateM, 0, offered); high != 100 {
		t.Errorf("the bitrate is offered to %d in a mode that aims at one, want 100", high)
	}

	// The quantizer mirrors it: it reaches the encoder in constant quality alone.
	if _, high := abr.Bounds(rules.AxisCq, 0, offered); high != offered {
		t.Errorf("the quantizer narrowed to %d in a mode that sends none", high)
	}
}

// The statement that narrows the control carries the limit itself: no axis holds it, and a reader
// looking at a slider that stops early is owed the number it stops at.
func TestACeilingStatesTheLimitItNarrowsTo(t *testing.T) {
	v := rules.EvaluateRules(codecFacts("libsvtav1", EngineFfmpeg, ModeAbr), codecRules())

	reasons := v.BoundReasons(rules.AxisBitrateM)
	if len(reasons) != 1 {
		t.Fatalf("a narrowed control says why, got %d reasons", len(reasons))
	}
	var limit int64 = -1
	for _, a := range reasons[0].GetArgs() {
		if a.GetName() == screensharev1.TextArgName_TEXT_ARG_NAME_BITRATE_LIMIT_MBPS {
			limit = a.GetNumber()
		}
	}
	if limit != 100 {
		t.Errorf("the statement carried a limit of %d, want the row's own 100", limit)
	}
}

// chromasDeclared is every pixel format any row lists, so a sweep asks about formats a codec does
// not carry as well as the ones it does.
func chromasDeclared() []string {
	var out []string
	for _, c := range Codecs {
		for _, chroma := range c.Chromas {
			if !contains(out, chroma) {
				out = append(out, chroma)
			}
		}
	}
	return out
}

// tunesDeclared is every tune step any ladder carries, for the reason chromasDeclared exists.
func tunesDeclared() []string {
	out := []string{TuneNone}
	for _, c := range Codecs {
		for _, step := range c.Tune.Steps {
			if !contains(out, step) {
				out = append(out, step)
			}
		}
	}
	return out
}
