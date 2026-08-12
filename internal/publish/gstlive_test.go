package publish

import (
	"bjoernblessin.de/screenshare/internal/rules"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// Every live property is one the pipeline is already carrying, spelled the way that
// element spells it.
//
// It is the claim the whole mechanism rests on: a write that named the wrong property, or
// counted kbit where the element counts bits per second, would set a rate nobody chose on
// a stream that is running. So each row is held to what the codec's own mapping writes -
// the same shape as the ladder tests, and for the same reason.
func TestTheLivePropertiesAreWhatTheBuildersSpend(t *testing.T) {
	for name := range gstCodecs {
		c, ok := capabilities.Get(name)
		if !ok {
			t.Errorf("%s has a GStreamer mapping but no capability row", name)
			continue
		}
		property, live := gstLiveBitrate[name]
		if !live {
			t.Errorf("%s builds a pipeline and declares no live bitrate property", name)
			continue
		}

		mode := ""
		for _, m := range capabilities.Modes {
			if capabilities.TargetsBitrate(m) && capabilities.Reaches(name, capabilities.EngineGst, capabilities.OptionMode, m) {
				mode = m
				break
			}
		}
		if mode == "" {
			continue // A codec with no rate-targeting mode on this engine sends no rate.
		}

		t.Run(name, func(t *testing.T) {
			s := baseStream()
			chromas := c.EngineChromas(capabilities.EngineGst)
			s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = name, mode, chromas[len(chromas)-1]
			s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(name, mode)
			// Under every element's own property range, including the two qsv elements whose
			// bitrate stops at an unsigned 16-bit kbit figure (qsvShortRateLimits).
			s.Publish.BitrateM = 20
			if limit := c.BitrateLimitOn(capabilities.EngineGst); limit > 0 && s.Publish.BitrateM > limit {
				s.Publish.BitrateM = limit
			}

			encoder, _, err := gstEncoder(s, 60, gpupath.MemorySystem)
			if err != nil {
				t.Fatalf("building: %v", err)
			}
			want := property.name + "=" + property.value(s.Publish.BitrateM)
			if !slices.Contains(encoder, want) {
				t.Errorf("the live write is %q, where the element takes %v", want, encoder)
			}
			if !slices.Contains(encoder, "name="+gstEncoderName) {
				t.Errorf("the encoder carries no name for a write to address: %v", encoder)
			}
		})
	}
}

// The state is what the settings say, and empty where they say nothing a running pipeline
// can take: a mode that sends the encoder no rate has none to send it again.
func TestTheLiveStateIsEmptyWhereNoRateIsSent(t *testing.T) {
	s := baseStream()
	// A capture backend this engine runs, because the live table is gated on the engine
	// and the default settings name one the other engine drives.
	s.Publish.Capture = "ximagesrc"
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx264", capabilities.ModeCbr, "yuv420p"
	s.Publish.BitrateM = 12
	state := gstLiveState(s)
	if len(state.Properties) != 1 || state.Properties[0].Value != "12000" {
		t.Errorf("a bitrate mode carries %+v, want one write of 12000 kbit", state.Properties)
	}
	if el := state.Properties[0].Element; el != gstEncoderName {
		t.Errorf("the write addresses %q rather than the encoder", el)
	}

	for _, mode := range []string{capabilities.ModeCrf, capabilities.ModeLossless} {
		s.Publish.Mode = mode
		if state := gstLiveState(s); len(state.Properties) != 0 {
			t.Errorf("%s sends the encoder no rate, yet carries %+v", mode, state.Properties)
		}
	}
}

// The bits-per-second elements are the ones a unit mistake would break silently: a write
// of 12000 where the element counts bits is twelve kilobits a second, not twelve megabits.
func TestTheBitsPerSecondElementsCountInBits(t *testing.T) {
	s := baseStream()
	s.Publish.Capture = "ximagesrc"
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libvpx-vp9", capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)
	s.Publish.BitrateM = 12

	state := gstLiveState(s)
	if len(state.Properties) != 1 || state.Properties[0].Value != "12000000" {
		t.Errorf("vp9 carries %+v, want one write of 12000000 bits per second", state.Properties)
	}
	if !strings.HasPrefix(state.Properties[0].Name, "target-bitrate") {
		t.Errorf("vp9's rate travels in %q", state.Properties[0].Name)
	}
}

// The ffmpeg engine takes nothing back once it is running: neither the process nor the
// command line it was given accepts a value, so a change there is a relaunch whatever it
// touched. The engine is the only difference between the two configurations here, which is
// what says the table is gated on it and not on the codec that happens to be selected.
func TestNothingIsLiveOnTheFfmpegEngine(t *testing.T) {
	s := baseStream()
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx264", capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)
	s.Publish.BitrateM = 20

	s.Publish.Capture = "x11grab"
	if fields := LiveFields(s); len(fields) != 0 {
		t.Errorf("the ffmpeg engine takes %v while it runs, and it takes nothing", fields)
	}
	next := s
	next.Publish.BitrateM = 30
	if live, err := LiveOnly(s, next); err != nil || live {
		t.Errorf("a change on the ffmpeg engine is a relaunch, and this asked for an apply (%v, %v)", live, err)
	}

	s.Publish.Capture = "ximagesrc"
	if fields := LiveFields(s); !slices.Contains(fields, rules.AxisBitrateM) {
		t.Errorf("the same settings on the GStreamer engine take %v, want the bitrate among them", fields)
	}
}

// A configuration no publisher runs is answered as not live rather than guessed at: a
// capture backend with no engine behind it names one, and an apply decided on a guess would
// leave a stream on values the child never took.
func TestAnUnknownCaptureBackendIsNotLive(t *testing.T) {
	s := baseStream()
	s.Publish.Capture = "no-such-backend"
	if fields := LiveFields(s); len(fields) != 0 {
		t.Errorf("a capture backend with no engine reports %v as live", fields)
	}
}

// A bitrate change is an apply and everything else is a relaunch, which is what decides
// whether viewers keep watching or reconnect.
func TestOnlyTheLiveSubsetAvoidsARelaunch(t *testing.T) {
	running := baseStream()
	running.Publish.Codec, running.Publish.Mode, running.Publish.Chroma = "libx264", capabilities.ModeCbr, "yuv420p"
	running.Publish.Capture = "ximagesrc"
	running.Publish.Effort, running.Publish.Tune = settings.LadderSteps(running.Publish.Codec, running.Publish.Mode)
	running.Publish.BitrateM = 20

	next := running
	next.Publish.BitrateM = 30
	live, err := LiveOnly(running, next)
	if err != nil {
		t.Fatal(err)
	}
	if !live {
		t.Error("a bitrate change is what the pipeline takes while it runs, yet it asks for a relaunch")
	}

	for _, change := range []func(*settings.Settings){
		func(s *settings.Settings) { s.Publish.Fps = 30 },
		func(s *settings.Settings) { s.Publish.Chroma = "yuv444p" },
		// 300 rather than 120: the keyframe interval resolves to twice the frame rate when it
		// is left at zero, so 120 at 60 fps is the pipeline this is already running.
		func(s *settings.Settings) { s.Publish.Gop = 300 },
	} {
		other := running
		change(&other)
		if live, _ := LiveOnly(running, other); live {
			t.Errorf("a change no property write carries was treated as live: %+v", other.Publish)
		}
	}
}
