package publish

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// A capture backend carries every transport its engine can serialize through, so
// the two engines part where a transport implements one publish form only: RTMP
// has an ffmpeg muxer and no GStreamer counterpart, which leaves it off every
// GStreamer backend and on every ffmpeg one.
//
// Every registered backend is named, so a row added without an expectation fails
// here rather than shipping a transport list nothing checked.
func TestTransportsFor(t *testing.T) {
	cases := map[string][]string{
		"x11grab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"kmsgrab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"ddagrab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"gdigrab":               {"rtmp", "rtsp", "srt", "webrtc"},
		"avfoundation":          {"rtmp", "rtsp", "srt", "webrtc"},
		"portal":                {"rtsp", "srt", "webrtc"},
		"ximagesrc":             {"rtsp", "srt", "webrtc"},
		"avfvideosrc":           {"rtsp", "srt", "webrtc"},
		"d3d11screencapturesrc": {"rtsp", "srt", "webrtc"},
	}
	for capture := range captureBackends {
		if _, ok := cases[capture]; !ok {
			t.Errorf("capture backend %q states no expected transport list", capture)
		}
	}
	for capture, want := range cases {
		got, err := TransportsFor(capture)
		if err != nil {
			t.Errorf("TransportsFor(%q) error: %v", capture, err)
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("TransportsFor(%q) = %v, want %v", capture, got, want)
		}
	}
}

func TestTransportsForUnknownCapture(t *testing.T) {
	if _, err := TransportsFor("nope"); err == nil {
		t.Error("TransportsFor unknown capture must error")
	}
}

// publishable is a stream both engines render a pipeline for, on the software encoder
// every machine has and the rate-control mode that reads the most knobs.
func publishable() settings.Settings {
	s := baseStream()
	s.Publish.Capture = "x11grab"
	s.Publish.Codec = "libx264"
	s.Publish.Mode = "cbr"
	s.Publish.Chroma = "yuv420p"
	s.Publish.Transport = "srt"
	return s
}

func TestSamePipelineHoldsForUnchangedSettings(t *testing.T) {
	s := publishable()
	same, err := SamePipeline(s, s)
	if err != nil {
		t.Fatalf("SamePipeline error: %v", err)
	}
	if !same {
		t.Error("unchanged settings publish through the same pipeline")
	}
}

// The watch leg and the figures the form warns from are settings of the app rather than
// of the pipeline, so moving one leaves a running publish alone. A builder that starts
// reading one of these fails here, which is the point: the same comparison decides
// whether the stream is restarted under the user.
func TestSamePipelineIgnoresWhatNoPipelineReads(t *testing.T) {
	cases := map[string]func(*settings.Settings){
		"apiPort":            func(s *settings.Settings) { s.Relay.ApiPort = 19997 },
		"hlsPort":            func(s *settings.Settings) { s.Relay.HlsPort = 18888 },
		"uplinkMbps":         func(s *settings.Settings) { s.Publish.UplinkMbps = 500 },
		"watchTransport":     func(s *settings.Settings) { s.Viewer.PlayerWatchTransport = "rtsp" },
		"gridTransport":      func(s *settings.Settings) { s.Viewer.TileWatchTransport = "rtsp" },
		"srtWatchLatencyMs":  func(s *settings.Settings) { s.Viewer.SrtWatchLatencyMs = 900 },
		"rtspWatchLatencyMs": func(s *settings.Settings) { s.Viewer.RtspWatchLatencyMs = 400 },
		"rtspWatchProtocol":  func(s *settings.Settings) { s.Viewer.RtspWatchProtocol = "udp" },
	}
	for field, move := range cases {
		t.Run(field, func(t *testing.T) {
			before := publishable()
			after := before
			move(&after)
			if after == before {
				t.Fatal("the case moves the field it names")
			}
			same, err := SamePipeline(before, after)
			if err != nil {
				t.Fatalf("SamePipeline error: %v", err)
			}
			if !same {
				t.Error("no publish pipeline reads this field, so moving it keeps the pipeline")
			}
		})
	}
}

// What the pipeline is built from moves the pipeline. Each of these reaches a different
// part of the command (the relay path, the source, the encoder, the sink), so the
// comparison is held to the whole line rather than to the encoder arguments alone.
func TestSamePipelineSeesWhatThePipelineIsBuiltFrom(t *testing.T) {
	cases := map[string]func(*settings.Settings){
		"name":      func(s *settings.Settings) { s.Publish.Name = "other" },
		"relayHost": func(s *settings.Settings) { s.Relay.Host = "10.0.0.2" },
		"fps":       func(s *settings.Settings) { s.Publish.Fps = 30 },
		"codec":     func(s *settings.Settings) { s.Publish.Codec = "libx265" },
		"bitrateM":  func(s *settings.Settings) { s.Publish.BitrateM = 40 },
		// Not 2*fps, which is the interval the auto value already resolves to and so
		// the one explicit value that builds the pipeline it replaces.
		"gop":       func(s *settings.Settings) { s.Publish.Gop = 90 },
		"transport": func(s *settings.Settings) { s.Publish.Transport = "rtsp" },
		"capture":   func(s *settings.Settings) { s.Publish.Capture = "ximagesrc" },
	}
	for field, move := range cases {
		t.Run(field, func(t *testing.T) {
			before := publishable()
			after := before
			move(&after)
			same, err := SamePipeline(before, after)
			if err != nil {
				t.Fatalf("SamePipeline error: %v", err)
			}
			if same {
				t.Error("the pipeline is built from this field, so moving it needs a new one")
			}
		})
	}
}

// A settings object no engine can render names a pipeline that cannot run. Answering
// "the same" for it would report a stream as carrying settings it could never carry.
func TestSamePipelineRefusesSettingsNoEngineRenders(t *testing.T) {
	before := publishable()
	after := before
	after.Publish.Capture = "nope"
	if _, err := SamePipeline(before, after); err == nil {
		t.Error("SamePipeline over an unknown capture backend must error")
	}
	if _, err := SamePipeline(after, before); err == nil {
		t.Error("SamePipeline must error whichever side cannot be rendered")
	}
}

// A backend whose platform serves no monitor source refuses desktop audio rather than
// publishing a silent track, whichever engine runs it.
//
// The refusal is asserted through Command, because that is the one path a run and the
// displayed line both take: an engine that only refused inside Start would show a user
// a command the publish button cannot execute. The verdict is the source table's, so
// what a greyed option says before publishing and what a refused publish says rest on
// one answer rather than two that drift; the refusal names the backend and the source
// rather than quoting the table's statement, because it is an operational error and
// the statement is what the greyed option shows (api/proto/screenshare/v1/text.proto).
func TestABackendWhosePlatformServesNoMonitorSourceRefusesDesktopAudio(t *testing.T) {
	for capture := range captureBackends {
		available, _ := AudioAvailable(capture, platform.AudioSourceDesktop)

		s := baseStream()
		s.Publish.Capture, s.Publish.Transport, s.Publish.Audio = capture, "rtsp", platform.AudioSourceDesktop
		_, err := Command(s)

		if available {
			// A backend on a serving platform is left to fail, or not, on everything else a
			// pipeline needs; what is asserted is only that the audio source was not what
			// refused it.
			if err != nil && strings.Contains(err.Error(), "desktop audio") {
				t.Errorf("%s runs on a platform serving desktop audio and must not refuse it: %v", capture, err)
			}
			continue
		}

		if err == nil {
			t.Errorf("the %s backend reaches no desktop audio and must refuse it", capture)
			continue
		}
		if !strings.Contains(err.Error(), capture) || !strings.Contains(err.Error(), platform.AudioSourceDesktop) {
			t.Errorf("%s: refusal %q must name the backend and the source it cannot record", capture, err)
		}
	}
}

// Every registered backend is answerable for, and the absent source is refused by none:
// a stream with no second track asks nothing of the machine, so a platform cannot be
// missing the piece that serves it.
func TestEveryCaptureBackendAnswersForEveryAudioSource(t *testing.T) {
	for capture := range captureBackends {
		available, reason := AudioAvailable(capture, platform.AudioSourceNone)
		if !available {
			t.Errorf("%s: the absent audio source is served everywhere, got %v", capture, reason)
		}
		for _, source := range []string{platform.AudioSourceNone, platform.AudioSourceDesktop} {
			available, reason := AudioAvailable(capture, source)
			if available != (reason == nil) {
				t.Errorf("%s/%s: an unserved source says what is missing, available=%v reason=%v",
					capture, source, available, reason)
			}
		}
	}
}
