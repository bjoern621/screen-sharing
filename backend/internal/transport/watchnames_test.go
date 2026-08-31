package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The player and pipeline lists differ by one leg no player URL expresses: WHEP is a signaling
// exchange rather than an address.
func TestWatchNamesArePerEngine(t *testing.T) {
	players := WatchNames(capabilities.EngineFfmpeg)
	pipeline := WatchNames(capabilities.EngineGst)

	for _, want := range []string{"srt", "rtsp", "rtmp", "hls"} {
		if !slices.Contains(players, want) {
			t.Errorf("WatchNames(ffmpeg) = %v, missing %q", players, want)
		}
	}
	if slices.Contains(players, "webrtc") {
		t.Errorf("WatchNames(ffmpeg) = %v, must exclude webrtc, which no viewer program opens", players)
	}
	for _, want := range []string{"srt", "rtsp", "rtmp", "webrtc", "hls"} {
		if !slices.Contains(pipeline, want) {
			t.Errorf("WatchNames(gstreamer) = %v, missing %q", pipeline, want)
		}
	}
	if slices.Contains(pipeline, "moq") {
		t.Errorf("WatchNames(gstreamer) = %v, must exclude moq, which no source element here subscribes", pipeline)
	}
	for _, engine := range WatchEngines {
		if names := WatchNames(engine); !slices.IsSorted(names) {
			t.Errorf("WatchNames(%s) = %v, want sorted", engine, names)
		}
	}
}

// The browser's list is neither of the others: the legs the relay serves a page for.
// WHEP and MoQ are on it and no player opens either, and SRT is off it although both other readers
// take it, a browser reaching a relay listener over HTTP and nothing else.
func TestBrowserWatchNamesAreTheLegsWithAPage(t *testing.T) {
	browser := WatchNames(EngineBrowser)

	if want := []string{"hls", "moq", "webrtc"}; !slices.Equal(browser, want) {
		t.Errorf("WatchNames(browser) = %v, want %v", browser, want)
	}
	for _, name := range browser {
		if _, ok := BrowserURL(name, settings.Settings{Relay: settings.Relay{Host: "10.0.0.5"}}, "bob"); !ok {
			t.Errorf("%s states a browser carriage and yields no page address", name)
		}
	}
	for _, name := range Names() {
		_, page := BrowserURL(name, settings.Settings{Relay: settings.Relay{Host: "10.0.0.5"}}, "bob")
		if page != slices.Contains(browser, name) {
			t.Errorf("%s: a page address and a browser carriage disagree", name)
		}
	}
}

// Two pages on one address is one leg opened where the other was asked for.
// The proxied deployment is where that happens: HTTPOrigin drops the port under Tls, so a page told
// from its neighbour by nothing but a listener number collapses onto it.
func TestBrowserPageAddressesStayDistinct(t *testing.T) {
	for _, host := range []string{"10.0.0.5", "relay.example.com"} {
		s := settings.Settings{Relay: settings.Relay{
			Host: host, WebrtcPort: 8889, HlsPort: 8888, MoqPort: 8892,
		}}

		opens := map[string]string{}
		for _, name := range WatchNames(EngineBrowser) {
			page, ok := BrowserURL(name, s, "public/bob")
			if !ok {
				t.Fatalf("%s states a browser carriage and yields no page address", name)
			}
			if other, taken := opens[page]; taken {
				t.Errorf("%s and %s both open %q on %s, so one of them plays the other's leg",
					other, name, page, host)
			}
			opens[page] = name
		}
	}
}

// PublishNames fills the publish dropdown, per engine.
// A protocol the relay serves and does not ingest has no publish form on either engine and stays
// off both lists, rather than appearing greyed with a reason no capture backend could lift.
// RTMP is the per-engine difference, the enhanced-RTMP tags the relay ingests coming out of the flv
// muxer where flvmux writes the legacy ones alone.
func TestPublishNamesArePerEngine(t *testing.T) {
	ffmpeg := PublishNames(capabilities.EngineFfmpeg)
	gst := PublishNames(capabilities.EngineGst)

	for _, want := range []string{"srt", "rtsp", "rtmp", "webrtc"} {
		if !slices.Contains(ffmpeg, want) {
			t.Errorf("PublishNames(ffmpeg) = %v, missing %q", ffmpeg, want)
		}
	}
	for _, want := range []string{"srt", "rtsp", "webrtc"} {
		if !slices.Contains(gst, want) {
			t.Errorf("PublishNames(gstreamer) = %v, missing %q", gst, want)
		}
	}
	if slices.Contains(gst, "rtmp") {
		t.Errorf("PublishNames(gstreamer) = %v, must exclude rtmp, which flvmux cannot write the relay's tags for", gst)
	}
	for _, engine := range capabilities.Engines {
		names := PublishNames(engine)
		if slices.Contains(names, "hls") {
			t.Errorf("PublishNames(%s) = %v, must exclude watch-only hls", engine, names)
		}
		if !slices.IsSorted(names) {
			t.Errorf("PublishNames(%s) = %v, want sorted", engine, names)
		}
	}
}
