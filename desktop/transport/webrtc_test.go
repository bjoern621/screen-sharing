package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/settings"
)

func TestWebRTCRegistered(t *testing.T) {
	tr, ok := Get("webrtc")
	if !ok {
		t.Fatal("webrtc transport not registered")
	}
	if tr.Name() != "webrtc" {
		t.Fatalf("Name() = %q, want webrtc", tr.Name())
	}
}

func TestWebRTCPublishArgs(t *testing.T) {
	s := settings.Stream{Name: "alice", RelayHost: "relay.example", WebrtcPort: 8889}
	args := WebRTC{}.PublishArgs(s)

	want := []string{"-f", "whip", "http://relay.example:8889/alice/whip"}
	if !slices.Equal(args, want) {
		t.Errorf("PublishArgs = %v, want %v", args, want)
	}
}

func TestWebRTCGstSink(t *testing.T) {
	s := settings.Stream{Name: "alice", RelayHost: "relay.example", WebrtcPort: 8889}
	sink := WebRTC{}.GstSink(s)

	// The endpoint is a property of the element's signaller, not of the element, and
	// the audio branch attaches by the mux name, so both have to be spelled exactly.
	for _, want := range []string{
		"whipclientsink",
		"name=" + GstMuxName,
		"signaller::whip-endpoint=http://relay.example:8889/alice/whip",
	} {
		if !slices.Contains(sink, want) {
			t.Errorf("GstSink = %v, missing %q", sink, want)
		}
	}
}

func TestWebRTCGstSource(t *testing.T) {
	s := settings.Stream{Name: "alice", RelayHost: "relay.example", WebrtcPort: 8889}
	src := WebRTC{}.GstSource(s, "bob")

	// The endpoint is the viewer's half of the exchange, and the empty audio caps
	// are what keeps the relay from refusing the offer, so both have to be spelled
	// exactly.
	for _, want := range []string{
		"whepsrc",
		"whep-endpoint=http://relay.example:8889/bob/whep",
		"audio-caps=EMPTY",
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
}

// WebRTC publishes on both engines and is watched by a receiving pipeline alone:
// WHEP is a signaling exchange rather than a URL a viewer program opens, so the
// capability helpers must split the two watch questions apart.
func TestWebRTCCapabilities(t *testing.T) {
	s := settings.Stream{Name: "alice", RelayHost: "relay.example", WebrtcPort: 8889, Transport: "webrtc"}

	if _, ok := GstSink(s); !ok {
		t.Error("GstSink must report true for webrtc")
	}
	if _, ok := WatchURL("webrtc", s, "bob"); ok {
		t.Error("WatchURL must report false for webrtc")
	}
	if _, ok := GstSource("webrtc", s, "bob"); !ok {
		t.Error("GstSource must report true for webrtc")
	}
	if !CanGstWatch("webrtc") {
		t.Error("CanGstWatch must report true for webrtc")
	}
	if slices.Contains(WatchNames(), "webrtc") {
		t.Error("WatchNames lists the transports a viewer program opens, which WHEP is not")
	}
	if !slices.Contains(GstWatchNames(), "webrtc") {
		t.Error("GstWatchNames must list webrtc")
	}
	if _, ok := PublishArgs(s); !ok {
		t.Error("PublishArgs must report true for webrtc")
	}
}
