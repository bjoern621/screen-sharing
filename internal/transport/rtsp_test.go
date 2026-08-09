package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// rtspStream carries knobs on both legs that differ from the defaults, so a
// serialization that ignores them shows up as the default rather than passing.
func rtspStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:     "relay.example",
			RtspPort: 8554,
		},
		Publish: settings.Publish{
			Name:                "alice",
			Transport:           "rtsp",
			RtspPublishProtocol: "udp",
		},
		Viewer: settings.Viewer{
			RtspWatchLatencyMs: 350,
			RtspWatchProtocol:  "udp",
		},
	}
}

func TestRTSPRegistered(t *testing.T) {
	tr, ok := Get("rtsp")
	if !ok {
		t.Fatal("rtsp transport not registered")
	}
	if tr.Name() != "rtsp" {
		t.Fatalf("Name() = %q, want rtsp", tr.Name())
	}
}

func TestRTSPPublishArgs(t *testing.T) {
	args := RTSP{}.PublishArgs(rtspStream())

	want := []string{"-f", "rtsp", "-rtsp_transport", "udp", "rtsp://relay.example:8554/alice"}
	if !slices.Equal(args, want) {
		t.Errorf("PublishArgs = %v, want %v", args, want)
	}
}

func TestRTSPGstSink(t *testing.T) {
	sink := RTSP{}.GstSink(rtspStream())

	for _, want := range []string{
		"rtspclientsink",
		"name=" + GstMuxName,
		"protocols=udp",
		"location=rtsp://relay.example:8554/alice",
	} {
		if !slices.Contains(sink, want) {
			t.Errorf("GstSink = %v, missing %q", sink, want)
		}
	}
}

func TestRTSPGstSource(t *testing.T) {
	src := RTSP{}.GstSource(rtspStream(), "bob")

	for _, want := range []string{
		"rtspsrc",
		"location=rtsp://relay.example:8554/bob",
		"protocols=udp",
		"latency=350",
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
}

// A stream whose legs disagree is what separates the two fields: a serialization
// reading the other leg's protocol passes every test where they happen to agree.
func TestRTSPProtocolPerLeg(t *testing.T) {
	s := rtspStream()
	s.Publish.RtspPublishProtocol = "tcp"
	s.Viewer.RtspWatchProtocol = "udp"

	if args := (RTSP{}).PublishArgs(s); !slices.Contains(args, "tcp") || slices.Contains(args, "udp") {
		t.Errorf("PublishArgs = %v, want the publish leg's tcp", args)
	}
	if sink := (RTSP{}).GstSink(s); !slices.Contains(sink, "protocols=tcp") {
		t.Errorf("GstSink = %v, want protocols=tcp", sink)
	}
	if src := (RTSP{}).GstSource(s, "bob"); !slices.Contains(src, "protocols=udp") {
		t.Errorf("GstSource = %v, want protocols=udp", src)
	}
}

func TestRTSPValidatePublishSettings(t *testing.T) {
	for _, protocol := range RtspProtocols {
		s := rtspStream()
		s.Publish.RtspPublishProtocol = protocol
		if err := (RTSP{}).ValidatePublishSettings(s); err != nil {
			t.Errorf("ValidatePublishSettings(%q) = %v, want accepted", protocol, err)
		}
	}

	// The empty value is a settings file written before the field existed and
	// migration missed; neither serialization has anything to write for it.
	for _, protocol := range []string{"", "sctp", "TCP"} {
		s := rtspStream()
		s.Publish.RtspPublishProtocol = protocol
		if err := (RTSP{}).ValidatePublishSettings(s); err == nil {
			t.Errorf("ValidatePublishSettings(%q) = nil, want a refusal", protocol)
		}
	}
}

// The package-level entry point is what the publish engines call, so it has to
// reach the transport's own answer rather than pass everything through.
func TestValidatePublishSettingsRefusesThroughRegistry(t *testing.T) {
	s := rtspStream()
	s.Publish.RtspPublishProtocol = "sctp"
	if err := ValidatePublishSettings(s); err == nil {
		t.Error("ValidatePublishSettings = nil, want the rtsp refusal")
	}

	// SRT declares no publish-leg settings of its own, and an unknown transport is
	// ValidatePublish's refusal rather than this one's.
	for _, name := range []string{"srt", "nope"} {
		s := rtspStream()
		s.Publish.Transport = name
		s.Publish.RtspPublishProtocol = "sctp"
		if err := ValidatePublishSettings(s); err != nil {
			t.Errorf("ValidatePublishSettings on %q = %v, want no opinion", name, err)
		}
	}
}

func TestRTSPWatchURL(t *testing.T) {
	url := RTSP{}.WatchURL(rtspStream(), "bob")

	if url != "rtsp://relay.example:8554/bob" {
		t.Errorf("WatchURL = %q, want rtsp://relay.example:8554/bob", url)
	}
}
