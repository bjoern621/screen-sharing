package form

import (
	"slices"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What a form promises about a control and what applying it actually costs have to be one
// answer. A form marking a control live is telling the reader nobody watching will be
// dropped; the apply path is what has to deliver that, and the two read the same table.

// liveFlags is every control the resolved form marks live, in the order it draws them.
func liveFlags(t *testing.T, s settings.Settings) []string {
	t.Helper()
	var out []string
	for _, g := range Resolve(fieldTestDeps(), s).GetGroups() {
		for _, f := range g.GetFields() {
			if f.GetLive() {
				out = append(out, f.GetKey())
			}
		}
	}
	return out
}

// liveSettings are settings whose pipeline takes a value while it runs: the GStreamer
// engine, a codec whose element has a bitrate property and a mode that sends it one.
func liveSettings() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "ximagesrc"
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx264", capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)
	return s
}

func TestTheFormMarksWhatTheRunningPipelineTakes(t *testing.T) {
	s := liveSettings()
	marked := liveFlags(t, s)
	if !slices.Contains(marked, KeyBitrateM) {
		t.Errorf("the form marks %v live, and this pipeline takes a new bitrate while it runs", marked)
	}
	if !slices.Equal(marked, publish.LiveFields(s)) {
		t.Errorf("the form marks %v and the apply path takes %v, which are two answers to one question",
			marked, publish.LiveFields(s))
	}
}

// A mode that sends the encoder no rate has none to send it again, so nothing is marked.
// The engine and the codec are unchanged, which is what makes this a statement about the
// mode rather than about either of them.
func TestAModeThatSendsNoRateMarksNothing(t *testing.T) {
	s := liveSettings()
	s.Publish.Mode = capabilities.ModeCrf
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)

	if marked := liveFlags(t, s); len(marked) != 0 {
		t.Errorf("constant quality marks %v live, and it sends the encoder no rate at all", marked)
	}
}

// The ffmpeg engine takes nothing back once it is running, so a form resolved against one
// of its capture backends promises nothing. Only the capture backend differs from the case
// above, which is what says the promise follows the engine.
func TestTheFfmpegEngineMarksNothing(t *testing.T) {
	s := liveSettings()
	s.Publish.Capture = "x11grab"

	if marked := liveFlags(t, s); len(marked) != 0 {
		t.Errorf("the ffmpeg engine marks %v live, and its child takes no value while it runs", marked)
	}
}

// A live control is one the reader can reach. A form that marked a greyed or hidden
// control live would be promising a cheap edit to somebody who cannot make it.
func TestAControlMarkedLiveIsOneTheReaderCanReach(t *testing.T) {
	for _, s := range []settings.Settings{liveSettings(), settings.Defaults()} {
		for _, g := range Resolve(fieldTestDeps(), s).GetGroups() {
			for _, f := range g.GetFields() {
				if !f.GetLive() {
					continue
				}
				if !f.GetVisible() || !f.GetEnabled() {
					t.Errorf("%s is marked live and is %s",
						f.GetKey(), unreachable(f))
				}
			}
		}
	}
}

func unreachable(f *screensharev1.Field) string {
	if !f.GetVisible() {
		return "not drawn"
	}
	return "greyed"
}
