package transport

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

func testStream() settings.Settings {
	return settings.Settings{
		Relay: settings.Relay{
			Host:    "10.0.0.5",
			SrtPort: 8890,
		},
		Publish: settings.Publish{
			Name:                "alice",
			Transport:           "srt",
			SrtPublishLatencyMs: 300,
		},
		Viewer: settings.Viewer{
			SrtWatchLatencyMs: 1200,
		},
	}
}

func TestSRTRegistered(t *testing.T) {
	tr, ok := Get("srt")
	if !ok {
		t.Fatal("srt transport not registered")
	}
	if tr.Name() != "srt" {
		t.Fatalf("Name() = %q, want srt", tr.Name())
	}
	if !slices.Contains(Names(), "srt") {
		t.Errorf("Names() = %v, missing srt", Names())
	}
}

func TestSRTPublishArgs(t *testing.T) {
	args := SRT{}.PublishArgs(testStream())

	if len(args) != 3 || args[0] != "-f" || args[1] != "mpegts" {
		t.Fatalf("PublishArgs prefix = %v, want [-f mpegts URL]", args)
	}

	url := args[2]
	for _, want := range []string{
		"srt://10.0.0.5:8890",
		"streamid=publish:public/alice",
		"pkt_size=1316",
		"latency=300000", // ffmpeg's srt protocol counts microseconds
	} {
		if !strings.Contains(url, want) {
			t.Errorf("publish URL %q missing %q", url, want)
		}
	}
}

func TestSRTGstSource(t *testing.T) {
	src := SRT{}.GstSource(testStream(), "bob")

	for _, want := range []string{
		"srtsrc",
		"uri=srt://10.0.0.5:8890",
		"mode=caller",
		"streamid=read:bob",
		"latency=1200", // srtsrc counts milliseconds, unlike the ffmpeg URL
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
}

func TestSRTWatchURL(t *testing.T) {
	url := SRT{}.WatchURL(testStream(), "bob")

	for _, want := range []string{
		"srt://10.0.0.5:8890",
		"streamid=read:bob",
		"latency=1200000", // the watch knob in microseconds too
	} {
		if !strings.Contains(url, want) {
			t.Errorf("watch URL %q missing %q", url, want)
		}
	}
}
