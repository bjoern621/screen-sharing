package watch

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
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

func TestBuildGridConfigRTSP(t *testing.T) {
	out, err := BuildGridConfig(rtspStream(), h264Live("alice", "bob"), "rtsp", nil)
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
	out, err := BuildGridConfig(srtStream(), h264Live("alice"), "srt", nil)
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

// The sidebar renders whatever the config declares, so a stream carries both the
// legs it could move to and the knobs of the one it is on.
func TestBuildGridConfigDeclaresWatchOptions(t *testing.T) {
	out, err := BuildGridConfig(srtStream(), h264Live("alice"), "srt", nil)
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	got := cfg.Streams[0]
	if !slices.Equal(got.Transports, []string{"rtmp", "rtsp", "srt", "webrtc"}) {
		t.Errorf("transports = %v, want every leg a pipeline receives H.264 over", got.Transports)
	}
	if len(got.Options) != 1 {
		t.Fatalf("got %d options, want the one srt knob: %+v", len(got.Options), got.Options)
	}
	if got.Options[0].Key != "srtWatchLatencyMs" || got.Options[0].Value != "1200" {
		t.Errorf("option = %+v, want the settings' srt latency", got.Options[0])
	}
}

// A choice moves one stream and leaves the rest of the window where it was.
func TestBuildGridConfigPerStreamChoice(t *testing.T) {
	s := srtStream()
	s.RtspPort = 8554
	choices := map[string]WatchChoice{"bob": {
		Transport: "rtsp",
		Options:   map[string]string{"rtspWatchLatencyMs": "400", "rtspWatchProtocol": "udp"},
	}}

	out, err := BuildGridConfig(s, h264Live("alice", "bob"), "srt", choices)
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Streams[0].Transport != "srt" || !strings.Contains(cfg.Streams[0].Source, "srtsrc") {
		t.Errorf("alice = %+v, want the window's leg", cfg.Streams[0])
	}
	bob := cfg.Streams[1]
	if bob.Transport != "rtsp" {
		t.Fatalf("bob transport = %q, want rtsp", bob.Transport)
	}
	if !strings.Contains(bob.Source, "latency=400") || !strings.Contains(bob.Source, "protocols=udp") {
		t.Errorf("bob source = %q, want the chosen rtsp knobs", bob.Source)
	}
	// The knobs the sidebar shows follow the leg the stream moved to.
	if len(bob.Options) != 2 {
		t.Errorf("bob options = %+v, want the two rtsp knobs", bob.Options)
	}
}

// A choice is one stream's business: the settings it is applied to are a copy,
// so the streams built after it are unaffected.
func TestBuildGridConfigChoiceLeavesTheSettings(t *testing.T) {
	choices := map[string]WatchChoice{"alice": {Options: map[string]string{"srtWatchLatencyMs": "80"}}}

	out, err := BuildGridConfig(srtStream(), h264Live("alice", "bob"), "srt", choices)
	if err != nil {
		t.Fatal(err)
	}

	var cfg GridConfig
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.Streams[0].Source, "latency=80") {
		t.Errorf("alice source = %q, want the chosen latency", cfg.Streams[0].Source)
	}
	if !strings.Contains(cfg.Streams[1].Source, "latency=1200") {
		t.Errorf("bob source = %q, want the settings' latency", cfg.Streams[1].Source)
	}
}

func TestBuildGridConfigEmptyRoster(t *testing.T) {
	out, err := BuildGridConfig(rtspStream(), nil, "rtsp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"streams":[]`) {
		t.Errorf("empty roster = %q, want a streams key holding an empty array", out)
	}
}

func TestBuildGridConfigRejectsUnsupportedTransport(t *testing.T) {
	// HLS is served by the relay and read by no source element here, so it is a
	// leg a viewer program opens and a grid window cannot.
	if _, err := BuildGridConfig(rtspStream(), h264Live("alice"), "hls", nil); err == nil {
		t.Fatal("expected error for a transport without a GStreamer watch form")
	}
	if _, err := BuildGridConfig(rtspStream(), h264Live("alice"), "carrier-pigeon", nil); err == nil {
		t.Fatal("expected error for an unknown transport")
	}
	// The transport is checked even when there is nothing to serialize.
	if _, err := BuildGridConfig(rtspStream(), nil, "hls", nil); err == nil {
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

func TestWatchLegRefusesATransportTheFormatIsNotServedOn(t *testing.T) {
	_, _, err := WatchLeg(srtStream(), vp9Live("alice"), "rtsp", WatchChoice{Transport: "srt"})
	if err == nil {
		t.Fatal("expected srt to be refused for a vp9 stream")
	}
	if !strings.Contains(err.Error(), "rtsp") {
		t.Errorf("error = %q, want it to name the transport that carries vp9", err)
	}
}

func TestWatchLegRefusesAnUnusableOption(t *testing.T) {
	cases := map[string]map[string]string{
		"undeclared key":    {"srtWatchLatencyMs": "1200"},
		"not a number":      {"rtspWatchLatencyMs": "soon"},
		"below the minimum": {"rtspWatchLatencyMs": "0"},
		"unknown choice":    {"rtspWatchProtocol": "carrier-pigeon"},
	}
	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := WatchLeg(rtspStream(), h264Live("alice")[0], "rtsp", WatchChoice{Options: options})
			if err == nil {
				t.Fatalf("expected %v to be refused", options)
			}
			if !strings.Contains(err.Error(), "alice") {
				t.Errorf("error = %q, want it to name the stream", err)
			}
		})
	}
}

// A stream that comes back in a format its chosen transport does not carry
// loses the choice, rather than costing every push the roster it appears in.
func TestPruneWatchChoices(t *testing.T) {
	choices := map[string]WatchChoice{
		"alice": {Transport: "srt"},
		"bob":   {Transport: "rtsp"},
		"carol": {Transport: "srt"},
	}
	live := []LiveStream{{Name: "alice", Format: "vp9"}, {Name: "bob", Format: "vp9"}}

	dropped := PruneWatchChoices(srtStream(), live, "rtsp", choices)

	if len(dropped) != 1 || !strings.Contains(dropped[0].Error(), "alice") {
		t.Errorf("dropped = %v, want alice alone", dropped)
	}
	if _, ok := choices["alice"]; ok {
		t.Error("alice kept a leg vp9 is not served on")
	}
	if _, ok := choices["bob"]; !ok {
		t.Error("bob lost a leg that carries vp9")
	}
	// carol is not live, so nothing about her can be judged: her choice waits
	// for the run she comes back in.
	if _, ok := choices["carol"]; !ok {
		t.Error("a stream that is not live lost its leg")
	}
}

// A stream with no choice of its own stays on the leg the window was opened on,
// even where the format is not served over it: the window was opened that way,
// and moving the stream silently would hide the mismatch the sidebar shows.
func TestWatchLegKeepsTheWindowsTransport(t *testing.T) {
	_, name, err := WatchLeg(srtStream(), vp9Live("alice"), "srt", WatchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if name != "srt" {
		t.Errorf("transport = %q, want the window's srt", name)
	}
}
