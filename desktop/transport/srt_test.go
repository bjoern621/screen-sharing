package transport

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/settings"
)

func testStream() settings.Stream {
	return settings.Stream{
		Name:                "alice",
		RelayHost:           "relay.example",
		RelayPort:           8890,
		Transport:           "srt",
		SrtPublishLatencyMs: 300,
		SrtWatchLatencyMs:   1200,
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
		"srt://relay.example:8890",
		"streamid=publish:alice",
		"pkt_size=1316",
		"latency=300000", // ms is converted to SRT's microseconds
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
		"uri=srt://relay.example:8890",
		"mode=caller",
		"streamid=read:bob",
		"latency=1200", // srtsrc takes milliseconds, unlike the ffmpeg URL
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
}

func TestSRTWatchURL(t *testing.T) {
	url := SRT{}.WatchURL(testStream(), "bob")

	for _, want := range []string{
		"srt://relay.example:8890",
		"streamid=read:bob",
		"latency=1200000", // watch latency in microseconds
	} {
		if !strings.Contains(url, want) {
			t.Errorf("watch URL %q missing %q", url, want)
		}
	}
}
