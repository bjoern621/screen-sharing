package watch

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildGridConfigRTSP(t *testing.T) {
	out, err := BuildGridConfig(rtspStream(), []string{"alice", "bob"}, "rtsp")
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
	for i, want := range []string{"alice", "bob"} {
		src := cfg.Streams[i].Source
		if !strings.Contains(src, "rtspsrc") ||
			!strings.Contains(src, "location=rtsp://relay.example:8554/"+want) ||
			!strings.Contains(src, "protocols=tcp") {
			t.Errorf("stream %d source = %q lacks the rtsp watch form", i, src)
		}
	}
}

func TestBuildGridConfigSRT(t *testing.T) {
	out, err := BuildGridConfig(srtStream(), []string{"alice"}, "srt")
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

func TestBuildGridConfigRejectsEmptyAndUnsupported(t *testing.T) {
	if _, err := BuildGridConfig(rtspStream(), nil, "rtsp"); err == nil {
		t.Fatal("expected error for an empty stream list")
	}
	if _, err := BuildGridConfig(rtspStream(), []string{"alice"}, "webrtc"); err == nil {
		t.Fatal("expected error for a transport without a GStreamer watch form")
	}
	if _, err := BuildGridConfig(rtspStream(), []string{"alice"}, "carrier-pigeon"); err == nil {
		t.Fatal("expected error for an unknown transport")
	}
}
