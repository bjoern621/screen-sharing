package watch

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// h264Live and vp9Live are the two carry cases: a format every watch transport
// serves, and one only RTSP has a payload mapping for.
func h264Live(names ...string) []LiveStream {
	var live []LiveStream
	for _, name := range names {
		live = append(live, LiveStream{Name: name, Format: "h264"})
	}
	return live
}

func vp9Live(name string) LiveStream { return LiveStream{Name: name, Format: "vp9"} }

// gridOn is the settings with the grid window put on one leg, which is what
// BuildGridConfig reads and what a request replaces.
func gridOn(s settings.Stream, name string) settings.Stream {
	s.GridTransport = name
	return s
}

func TestBuildGridConfigRTSP(t *testing.T) {
	out, err := BuildGridConfig(rtspStream(), h264Live("alice", "bob"), GridApp{})
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
	if len(cfg.Streams) != 2 {
		t.Fatalf("got %d streams, want 2", len(cfg.Streams))
	}
	if cfg.Streams[1].Name != "bob" {
		t.Errorf("stream 1 name = %q, want bob", cfg.Streams[1].Name)
	}
	if cfg.Streams[1].Transport != "rtsp" {
		t.Errorf("stream 1 transport = %q, want rtsp", cfg.Streams[1].Transport)
	}
	for i, want := range []string{"alice", "bob"} {
		src := cfg.Streams[i].Source
		if !strings.Contains(src, "rtspsrc") ||
			!strings.Contains(src, "location=rtsp://relay.example:8554/"+want) ||
			!strings.Contains(src, "protocols=udp") ||
			!strings.Contains(src, "latency=350") {
			t.Errorf("stream %d source = %q lacks the rtsp watch form", i, src)
		}
	}
}

func TestBuildGridConfigSRT(t *testing.T) {
	out, err := BuildGridConfig(srtStream(), h264Live("alice"), GridApp{})
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	src := cfg.Streams[0].Source
	for _, want := range []string{"srtsrc", "uri=srt://relay.example:8890", "streamid=read:alice"} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in %q", want, src)
		}
	}
}

// The sidebar renders whatever the config declares, so a stream carries the legs
// it could move to and the knobs of every one of them: the popover swaps its
// controls on the pick rather than on the app's answer.
func TestBuildGridConfigDeclaresWatchOptionsPerLeg(t *testing.T) {
	out, err := BuildGridConfig(srtStream(), h264Live("alice"), GridApp{})
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	got := cfg.Streams[0]
	want := []string{"rtmp", "rtsp", "srt", "webrtc"}
	if !slices.Equal(got.Transports, want) {
		t.Errorf("transports = %v, want every leg a pipeline receives H.264 over", got.Transports)
	}
	if legs := slices.Sorted(maps.Keys(got.Options)); !slices.Equal(legs, want) {
		t.Errorf("option legs = %v, want a knob set per offered leg", legs)
	}
	if len(got.Options["srt"]) != 1 {
		t.Fatalf("srt options = %+v, want the one srt knob", got.Options["srt"])
	}
	if got.Options["srt"][0].Key != "srtWatchLatencyMs" || got.Options["srt"][0].Value != "1200" {
		t.Errorf("srt option = %+v, want the settings' srt latency", got.Options["srt"][0])
	}
	// The leg the window is not on carries its knobs at the settings' values too,
	// which is what the popover shows the moment it is picked.
	if len(got.Options["rtsp"]) != 2 {
		t.Errorf("rtsp options = %+v, want the two rtsp knobs", got.Options["rtsp"])
	}
	// A leg with nothing to tune is declared empty rather than left out, so the
	// window can tell it from one it was never told about.
	if opts, ok := got.Options["webrtc"]; !ok || len(opts) != 0 {
		t.Errorf("webrtc options = %+v (present %v), want an empty declaration", opts, ok)
	}
}

// The leg a stream is on is offered beside the ones its format is served over,
// so a window showing a stream its leg does not carry can still name that leg.
func TestBuildGridConfigDeclaresTheLegInForce(t *testing.T) {
	out, err := BuildGridConfig(srtStream(), []LiveStream{vp9Live("alice")}, GridApp{})
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	got := cfg.Streams[0]
	if !slices.Equal(got.Transports, []string{"rtsp", "webrtc"}) {
		t.Errorf("transports = %v, want the legs vp9 is served on", got.Transports)
	}
	if _, ok := got.Options["srt"]; !ok {
		t.Errorf("options = %v, want the leg in force declared beside them", slices.Sorted(maps.Keys(got.Options)))
	}
}

// The leg is one setting for the window, so every tile moves with it.
func TestBuildGridConfigPutsEveryStreamOnTheGridLeg(t *testing.T) {
	s := gridOn(rtspStream(), "srt")

	out, err := BuildGridConfig(s, h264Live("alice", "bob"), GridApp{})
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	for _, stream := range cfg.Streams {
		if stream.Transport != "srt" || !strings.Contains(stream.Source, "srtsrc") {
			t.Errorf("%s = %+v, want the grid's srt leg", stream.Name, stream)
		}
	}
}

// The relay names its own paths, so an unnamed or repeated one is input this
// side has to survive: the window keys its rows on the name and refuses a
// roster carrying either, and dropping the path keeps the rest of the roster.
func TestBuildGridConfigSkipsUnusableNames(t *testing.T) {
	live := []LiveStream{
		{Name: "alice", Format: "h264"},
		{Name: "", Format: "h264"},
		{Name: "alice", Format: "h264"},
		{Name: "bob", Format: "h264"},
	}

	out, err := BuildGridConfig(rtspStream(), live, GridApp{})
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, stream := range cfg.Streams {
		names = append(names, stream.Name)
	}
	if !slices.Equal(names, []string{"alice", "bob"}) {
		t.Errorf("streams = %v, want alice and bob once each", names)
	}
}

// A roster of nothing but unusable names still builds: the window opens empty
// and the next push fills it, which is what it does on an idle relay.
func TestBuildGridConfigAllNamesUnusable(t *testing.T) {
	out, err := BuildGridConfig(rtspStream(), []LiveStream{{Format: "h264"}}, GridApp{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"streams":[]`) {
		t.Errorf("roster = %q, want a streams key holding an empty array", out)
	}
}

func TestBuildGridConfigEmptyRoster(t *testing.T) {
	out, err := BuildGridConfig(rtspStream(), nil, GridApp{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"streams":[]`) {
		t.Errorf("empty roster = %q, want a streams key holding an empty array", out)
	}
}

// The app state travels with every push, publishing or not: the window draws its
// publish control from it and has no other source for the state.
func TestBuildGridConfigCarriesTheAppState(t *testing.T) {
	out, err := BuildGridConfig(rtspStream(), nil, GridApp{Publishing: true, PublishError: "no encoder"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `"app":{"publishing":true,"publishError":"no encoder"}`; !strings.Contains(out, want) {
		t.Errorf("config = %q, want %s", out, want)
	}

	out, err = BuildGridConfig(rtspStream(), nil, GridApp{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `"app":{"publishing":false,"publishError":""}`; !strings.Contains(out, want) {
		t.Errorf("config = %q, want %s", out, want)
	}
}

func TestBuildGridConfigRejectsUnsupportedTransport(t *testing.T) {
	// HLS is served by the relay and read by no source element here, so it is a
	// leg a viewer program opens and a grid window cannot.
	if _, err := BuildGridConfig(gridOn(rtspStream(), "hls"), h264Live("alice"), GridApp{}); err == nil {
		t.Fatal("expected error for a transport without a GStreamer watch form")
	}
	if _, err := BuildGridConfig(gridOn(rtspStream(), "carrier-pigeon"), h264Live("alice"), GridApp{}); err == nil {
		t.Fatal("expected error for an unknown transport")
	}
	// The transport is checked even when there is nothing to serialize.
	if _, err := BuildGridConfig(gridOn(rtspStream(), "hls"), nil, GridApp{}); err == nil {
		t.Fatal("expected error for an unsupported transport with an empty roster")
	}
}

func TestGstWatchTransportsNarrowsByFormat(t *testing.T) {
	// hls has no source element here, so it is out of every list however the
	// relay serves the format; webrtc is in them, since WHEP needs no player URL.
	if got := GstWatchTransports("h264"); !slices.Equal(got, []string{"rtmp", "rtsp", "srt", "webrtc"}) {
		t.Errorf("h264 = %v, want every leg a pipeline receives it over", got)
	}
	if got := GstWatchTransports("vp9"); !slices.Equal(got, []string{"rtsp", "webrtc"}) {
		t.Errorf("vp9 = %v, want rtsp and webrtc: neither MPEG-TS nor FLV maps VP9", got)
	}
	// A format the poll has not reported yet narrows nothing.
	if got := GstWatchTransports(""); !slices.Equal(got, []string{"rtmp", "rtsp", "srt", "webrtc"}) {
		t.Errorf("unknown format = %v, want every GStreamer watch transport", got)
	}
}

// An accepted request is the settings the app persists: the leg and the knobs of
// that leg, replacing what was held.
func TestApplyWatchLegWritesTheLegAndItsKnobs(t *testing.T) {
	base := gridOn(rtspStream(), "rtsp")

	next, err := ApplyWatchLeg(base, h264Live("alice")[0], GridRequest{
		Stream:    "alice",
		Transport: "srt",
		Options:   map[string]string{"srtWatchLatencyMs": "80"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.GridTransport != "srt" {
		t.Errorf("grid transport = %q, want the chosen srt", next.GridTransport)
	}
	if next.SrtWatchLatencyMs != 80 {
		t.Errorf("srt watch latency = %d, want the 80 the request carried", next.SrtWatchLatencyMs)
	}
	// The leg being left keeps its knobs: a request names one leg's, and the rest
	// of the settings are what they were.
	if next.RtspWatchLatencyMs != base.RtspWatchLatencyMs {
		t.Errorf("rtsp latency = %d, want the settings' %d", next.RtspWatchLatencyMs, base.RtspWatchLatencyMs)
	}
}

// A request naming no transport turns the knobs of the leg in force.
func TestApplyWatchLegKeepsTheLegInForce(t *testing.T) {
	next, err := ApplyWatchLeg(srtStream(), vp9Live("alice"), GridRequest{
		Stream:  "alice",
		Options: map[string]string{"srtWatchLatencyMs": "80"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.GridTransport != "srt" {
		t.Errorf("grid transport = %q, want the window's srt", next.GridTransport)
	}
	if next.SrtWatchLatencyMs != 80 {
		t.Errorf("srt watch latency = %d, want the 80 the request carried", next.SrtWatchLatencyMs)
	}
}

// The sidebar offers the leg in force beside the ones a stream's format is
// served over, so naming it holds it: refusing would leave that stream's knobs
// unreachable, since they travel with the name.
func TestApplyWatchLegTakesTheLegInForceByName(t *testing.T) {
	next, err := ApplyWatchLeg(srtStream(), vp9Live("alice"), GridRequest{
		Stream:    "alice",
		Transport: "srt",
		Options:   map[string]string{"srtWatchLatencyMs": "80"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.GridTransport != "srt" || next.SrtWatchLatencyMs != 80 {
		t.Errorf("leg = %q at %d ms, want srt at 80", next.GridTransport, next.SrtWatchLatencyMs)
	}
}

func TestApplyWatchLegRefusesATransportTheFormatIsNotServedOn(t *testing.T) {
	base := gridOn(srtStream(), "rtsp")

	next, err := ApplyWatchLeg(base, vp9Live("alice"), GridRequest{Stream: "alice", Transport: "srt"})
	if err == nil {
		t.Fatal("expected srt to be refused for a vp9 stream")
	}
	if !strings.Contains(err.Error(), "rtsp") {
		t.Errorf("error = %q, want it to name the transport that carries vp9", err)
	}
	if next != base {
		t.Errorf("settings = %+v, want the ones held", next)
	}
}

func TestApplyWatchLegRefusesAnUnusableOption(t *testing.T) {
	cases := map[string]map[string]string{
		"undeclared key":    {"srtWatchLatencyMs": "1200"},
		"not a number":      {"rtspWatchLatencyMs": "soon"},
		"below the minimum": {"rtspWatchLatencyMs": "0"},
		"unknown choice":    {"rtspWatchProtocol": "carrier-pigeon"},
	}
	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			base := rtspStream()

			next, err := ApplyWatchLeg(base, h264Live("alice")[0], GridRequest{Stream: "alice", Options: options})
			if err == nil {
				t.Fatalf("expected %v to be refused", options)
			}
			if !strings.Contains(err.Error(), "alice") {
				t.Errorf("error = %q, want it to name the stream", err)
			}
			if next != base {
				t.Errorf("settings = %+v, want the ones held", next)
			}
		})
	}
}
