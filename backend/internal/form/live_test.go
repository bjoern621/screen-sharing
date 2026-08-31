package form

import (
	"bjoernblessin.de/screenshare/internal/platform"
	"slices"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What the form promises about a control and what applying it costs are one answer,
// read off one table: a control marked live promises nobody watching is dropped,
// and the apply path delivers it.
// Which controls those are moves with the settings, so the flag is answered per field per resolve
// and no shell holds a list of live keys (docs/field-availability.md).

// liveFlags is every control the resolved form marks live, as controls rather than as entries:
// a repeated control drawn per entry is one control, and the apply path names it once.
func liveFlags(t *testing.T, s settings.Settings) []string {
	t.Helper()
	var out []string
	for _, g := range Resolve(fieldTestDeps(), s).GetGroups() {
		for _, f := range g.GetFields() {
			key := keyTemplate(f.GetKey())
			if f.GetLive() && !slices.Contains(out, key) {
				out = append(out, key)
			}
		}
	}
	return out
}

// liveSettings is a pipeline that takes a value while it runs: the GStreamer engine,
// a codec whose element has a bitrate property, a mode that sends it one,
// and a source in the mix whose level the mixer takes.
func liveSettings() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "ximagesrc"
	s.Publish.UseCodec("libx264")
	s.Publish.Mode, s.Publish.Chroma = capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec(), s.Publish.Mode)
	s.Publish.AudioSources = settings.Recording(platform.AudioSourceDesktop)
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

// A mode that sends the encoder no rate has none to send again.
// The engine and the codec are liveSettings' own, so this is a statement about the mode rather
// than about either of them.
// The mix's own levels stay marked: they reach the mixer,
// which does not care how the picture is coded.
func TestAModeThatSendsNoRateMarksNoBitrate(t *testing.T) {
	s := liveSettings()
	s.Publish.Mode = capabilities.ModeCrf
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec(), s.Publish.Mode)

	marked := liveFlags(t, s)
	if slices.Contains(marked, KeyBitrateM) {
		t.Errorf("constant quality marks %v live, and it sends the encoder no rate at all", marked)
	}
	if !slices.Contains(marked, KeyAudioSourceGain) {
		t.Errorf("constant quality marks %v live, and the mixer takes a level whatever codes the picture", marked)
	}
}

// The ffmpeg engine takes nothing once its child is running, so a form resolved
// against one of its capture backends marks nothing.
// Only the capture backend differs from liveSettings, so the promise follows the engine.
func TestTheFfmpegEngineMarksNothing(t *testing.T) {
	s := liveSettings()
	s.Publish.Capture = "x11grab"

	if marked := liveFlags(t, s); len(marked) != 0 {
		t.Errorf("the ffmpeg engine marks %v live, and its child takes no value while it runs", marked)
	}
}

// A greyed or hidden control marked live would promise a cheap edit to somebody who cannot make it.
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
