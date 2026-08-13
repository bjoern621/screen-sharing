package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
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
	s := settings.Settings{
		Relay: settings.Relay{
			Host:       "relay.example",
			WebrtcPort: 8889,
		},
		Publish: settings.Publish{
			Name: "alice",
		},
	}
	args := WebRTC{}.PublishArgs(s)

	want := []string{"-f", "whip", "http://relay.example:8889/alice/whip"}
	if !slices.Equal(args, want) {
		t.Errorf("PublishArgs = %v, want %v", args, want)
	}
}

func TestWebRTCGstSink(t *testing.T) {
	s := settings.Settings{
		Relay: settings.Relay{
			Host:       "relay.example",
			WebrtcPort: 8889,
		},
		Publish: settings.Publish{
			Name: "alice",
		},
	}
	sink := WebRTC{}.GstSink(s)

	// The endpoint is a property of the element's signaller, not of the element,
	// and the audio branch attaches by the mux name, so both have to be spelled exactly.
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
	s := settings.Settings{
		Relay: settings.Relay{
			Host:       "relay.example",
			WebrtcPort: 8889,
		},
		Publish: settings.Publish{
			Name: "alice",
		},
	}
	src := WebRTC{}.GstSource(s, "bob")

	// The endpoint is the viewer's half of the exchange, and the empty audio caps are what keeps the
	// relay from refusing the offer, so both have to be spelled exactly.
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
// WHEP is a signaling exchange rather than a URL a viewer program opens, so the capability helpers
// must split the two watch questions apart.
func TestWebRTCCapabilities(t *testing.T) {
	s := settings.Settings{
		Relay: settings.Relay{
			Host:       "relay.example",
			WebrtcPort: 8889,
		},
		Publish: settings.Publish{
			Name:      "alice",
			Transport: "webrtc",
		},
	}

	if _, ok := GstSink(s); !ok {
		t.Error("GstSink must report true for webrtc")
	}
	if _, ok := WatchURL("webrtc", s, "bob"); ok {
		t.Error("WatchURL must report false for webrtc")
	}
	if _, ok := GstSource("webrtc", s, "bob"); !ok {
		t.Error("GstSource must report true for webrtc")
	}
	if !CanWatch("webrtc", capabilities.EngineGst) {
		t.Error("CanWatch must report true for webrtc on the gstreamer engine")
	}
	if CanWatch("webrtc", capabilities.EngineFfmpeg) {
		t.Error("CanWatch must report false for webrtc on the ffmpeg engine, whose viewers open a URL")
	}
	if slices.Contains(WatchNames(capabilities.EngineFfmpeg), "webrtc") {
		t.Error("the players' watch list holds the transports a viewer program opens, which WHEP is not")
	}
	if !slices.Contains(WatchNames(capabilities.EngineGst), "webrtc") {
		t.Error("the receiving pipeline's watch list must hold webrtc")
	}
	if _, ok := PublishArgs(s); !ok {
		t.Error("PublishArgs must report true for webrtc")
	}
	for _, engine := range capabilities.Engines {
		if !CanPublish("webrtc", engine) {
			t.Errorf("CanPublish must report true for webrtc on the %s engine", engine)
		}
	}

	// The leg no player opens is the one a browser does, from the page the relay serves:
	// the exchange runs in the page's own RTCPeerConnection, so the address is the path and never the
	// whep endpoint the receiving pipeline posts to.
	page, ok := BrowserURL("webrtc", s, "bob")
	if !ok {
		t.Fatal("BrowserURL must report true for webrtc")
	}
	if want := "http://relay.example:8889/bob/"; page != want {
		t.Errorf("BrowserURL = %q, want %q", page, want)
	}
	if !CanWatch("webrtc", EngineBrowser) {
		t.Error("CanWatch must report true for webrtc on the browser engine")
	}
}

// The two engines negotiate different video sets over the same WHIP endpoint:
// ffmpeg's whip muxer has one H.264 payloader, and whipclientsink payloads whatever webrtcbin
// offers.
// Collapsing the two carriages into one list would have to take the narrower of them,
// which refuses the GStreamer engine two formats it serializes correctly, and the narrowing is
// invisible at the point it costs a publish.
// So the difference is pinned here rather than left to hold by coincidence.
func TestWebRTCPublishCarriagesDifferPerEngine(t *testing.T) {
	ffmpeg, ok := PublishCarriage("webrtc", capabilities.EngineFfmpeg)
	if !ok {
		t.Fatal("webrtc states no ffmpeg publish carriage")
	}
	gst, ok := PublishCarriage("webrtc", capabilities.EngineGst)
	if !ok {
		t.Fatal("webrtc states no gstreamer publish carriage")
	}

	if slices.Equal(ffmpeg.Video, gst.Video) {
		t.Errorf("both engines publish %v over WHIP, which makes one of the two lists a copy", ffmpeg.Video)
	}
	for _, format := range []string{"vp9", "vp8"} {
		if slices.Contains(ffmpeg.Video, format) {
			t.Errorf("ffmpeg's whip muxer has no %s payloader, so it must not be carried there", format)
		}
		if !slices.Contains(gst.Video, format) {
			t.Errorf("whipclientsink payloads %s, so the gstreamer carriage must hold it", format)
		}
	}
	// Opus is the whole audio set on both, which follows from what WebRTC negotiates rather than from
	// either engine's muxer.
	for _, c := range []Carriage{ffmpeg, gst} {
		if !slices.Equal(c.Audio, []string{"opus"}) {
			t.Errorf("a WebRTC publish carriage holds %v, want opus alone", c.Audio)
		}
	}
}
