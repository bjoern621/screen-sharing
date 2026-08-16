package transport

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// rtspStream sets every knob away from its default, so a serialization ignoring one renders the
// default rather than passing.
//
// The two legs hold different lower transports, which is what tells a serialization reading its own
// leg from one reading the other.
// Only the publish value is one this app accepts: every relay serves RTSPS alone and both legs
// interleave there (encryption_test.go), so the watch value here is what a hand-edited file holds
// and what the repair walks off.
func rtspStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:     "10.0.0.5",
			RtspPort: 8322,
		},
		Publish: settings.Publish{
			Name:                "alice",
			Transport:           "rtsp",
			RtspPublishProtocol: "tcp",
		},
		Viewer: settings.Viewer{
			RtspWatchLatencyMs: 350,
			RtspWatchProtocol:  "udp",
		},
	}
}

// rtspPath is where the fixture's stream lives on the relay: every relay authenticates, so a
// machine in no group publishes under the prefix anybody may watch.
const rtspPath = "public/alice"

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

	want := []string{"-f", "rtsp", "-rtsp_transport", "tcp", "-tls_verify", "0", "rtsps://10.0.0.5:8322/" + rtspPath}
	if !slices.Equal(args, want) {
		t.Errorf("PublishArgs = %v, want %v", args, want)
	}
}

func TestRTSPGstSink(t *testing.T) {
	sink := RTSP{}.GstSink(rtspStream())

	for _, want := range []string{
		"rtspclientsink",
		"name=" + GstMuxName,
		"protocols=tcp",
		"location=rtsps://10.0.0.5:8322/" + rtspPath,
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
		"location=rtsps://10.0.0.5:8322/bob",
		"protocols=udp",
		"latency=350",
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
}

// The source hands out a pad per track and a launch line's decoder has room for one, so the picture
// is pinned by caps rather than left to the announcement order.
// A session announcing audio first would otherwise decode the sound into the render chain and leave
// the picture unlinked, which is a tile that draws nothing.
func TestRTSPGstSourcePinsTheDecoderToThePicture(t *testing.T) {
	src := RTSP{}.GstSource(rtspStream(), "bob")

	if !slices.Contains(src, "application/x-rtp,media=video") {
		t.Errorf("GstSource = %v, which leaves the decoder to take whichever track came first", src)
	}
	if at := slices.Index(src, "!"); at < 0 || at != len(src)-2 {
		t.Errorf("GstSource = %v, want the capsfilter as the last element of the fragment", src)
	}
}

// A token written into the location never reaches SETUP: rtspsrc addresses a track at the SDP's
// control attribute joined onto the session URL, and that join keeps neither the query nor the last
// path segment, so the relay answers 401 for a path nothing serves and the tile draws nothing.
func TestRTSPGstSourceCarriesTheTokenBesideTheAddress(t *testing.T) {
	s := rtspStream()
	s.Relay.Token = "a-token"

	src := RTSP{}.GstSource(s, "bob")

	for _, want := range []string{
		"location=rtsps://10.0.0.5:8322/bob",
		"user-id=jwt",
		"user-pw=a-token",
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
	for _, arg := range src {
		if strings.Contains(arg, "jwt=a-token") {
			t.Errorf("GstSource = %v, which writes the token into %q, where SETUP loses it", src, arg)
		}
	}
}

// The publish legs keep the query, ffmpeg and rtspclientsink carrying it through every request of
// the session.
func TestRTSPPublishLegsKeepTheTokenInTheAddress(t *testing.T) {
	s := rtspStream()
	s.Relay.Token = "a-token"

	if args := (RTSP{}).PublishArgs(s); !slices.Contains(args, "rtsps://10.0.0.5:8322/"+rtspPath+"?jwt=a-token") {
		t.Errorf("PublishArgs = %v, want the token in the address", args)
	}
	if sink := (RTSP{}).GstSink(s); !slices.Contains(sink, "location=rtsps://10.0.0.5:8322/"+rtspPath+"?jwt=a-token") {
		t.Errorf("GstSink = %v, want the token in the address", sink)
	}
}

// Legs set to different protocols are what separates the two fields: a serialization reading the
// other leg's passes wherever the two agree.
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
	s := rtspStream()
	s.Publish.RtspPublishProtocol = EncryptedRtspProtocol
	if err := (RTSP{}).ValidatePublishSettings(s); err != nil {
		t.Errorf("ValidatePublishSettings(%q) = %v, want accepted", EncryptedRtspProtocol, err)
	}

	// The empty value is a settings file the migration missed, and neither serialization has anything
	// to write for it.
	for _, protocol := range []string{"", "sctp", "TCP"} {
		s := rtspStream()
		s.Publish.RtspPublishProtocol = protocol
		if err := (RTSP{}).ValidatePublishSettings(s); err == nil {
			t.Errorf("ValidatePublishSettings(%q) = nil, want a refusal", protocol)
		}
	}
}

// The publish engines call the package-level entry point, so it reaches the transport's own answer
// rather than passing everything through.
func TestValidatePublishSettingsRefusesThroughRegistry(t *testing.T) {
	s := rtspStream()
	s.Publish.RtspPublishProtocol = "sctp"
	if err := ValidatePublishSettings(s); err == nil {
		t.Error("ValidatePublishSettings = nil, want the rtsp refusal")
	}

	// SRT declares no protocol field, and an unknown transport is ValidatePublish's refusal rather
	// than this one's.
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

	if url != "rtsps://10.0.0.5:8322/bob" {
		t.Errorf("WatchURL = %q, want rtsps://10.0.0.5:8322/bob", url)
	}
}
