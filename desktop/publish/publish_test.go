package publish

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/settings"
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
func publishable() settings.Stream {
	s := settings.Defaults()
	s.Capture = "x11grab"
	s.Codec = "libx264"
	s.Mode = "cbr"
	s.Chroma = "yuv420p"
	s.Transport = "srt"
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
	cases := map[string]func(*settings.Stream){
		"apiPort":            func(s *settings.Stream) { s.ApiPort = 19997 },
		"hlsPort":            func(s *settings.Stream) { s.HlsPort = 18888 },
		"uplinkMbps":         func(s *settings.Stream) { s.UplinkMbps = 500 },
		"watchTransport":     func(s *settings.Stream) { s.WatchTransport = "rtsp" },
		"gridTransport":      func(s *settings.Stream) { s.GridTransport = "rtsp" },
		"srtWatchLatencyMs":  func(s *settings.Stream) { s.SrtWatchLatencyMs = 900 },
		"rtspWatchLatencyMs": func(s *settings.Stream) { s.RtspWatchLatencyMs = 400 },
		"rtspWatchProtocol":  func(s *settings.Stream) { s.RtspWatchProtocol = "udp" },
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
	cases := map[string]func(*settings.Stream){
		"name":      func(s *settings.Stream) { s.Name = "other" },
		"relayHost": func(s *settings.Stream) { s.RelayHost = "10.0.0.2" },
		"fps":       func(s *settings.Stream) { s.Fps = 30 },
		"codec":     func(s *settings.Stream) { s.Codec = "libx265" },
		"bitrateM":  func(s *settings.Stream) { s.BitrateM = 40 },
		// Not 2*fps, which is the interval the auto value already resolves to and so
		// the one explicit value that builds the pipeline it replaces.
		"gop":       func(s *settings.Stream) { s.Gop = 90 },
		"transport": func(s *settings.Stream) { s.Transport = "rtsp" },
		"capture":   func(s *settings.Stream) { s.Capture = "ximagesrc" },
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
	after.Capture = "nope"
	if _, err := SamePipeline(before, after); err == nil {
		t.Error("SamePipeline over an unknown capture backend must error")
	}
	if _, err := SamePipeline(after, before); err == nil {
		t.Error("SamePipeline must error whichever side cannot be rendered")
	}
}
