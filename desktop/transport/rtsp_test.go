package transport

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/settings"
)

func rtspStream() settings.Stream {
	return settings.Stream{
		Name:      "alice",
		RelayHost: "relay.example",
		RtspPort:  8554,
		Transport: "rtsp",
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

	want := []string{"-f", "rtsp", "-rtsp_transport", "tcp", "rtsp://relay.example:8554/alice"}
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
		"protocols=tcp",
		"latency=200",
	} {
		if !slices.Contains(src, want) {
			t.Errorf("GstSource = %v, missing %q", src, want)
		}
	}
}

func TestRTSPWatchURL(t *testing.T) {
	url := RTSP{}.WatchURL(rtspStream(), "bob")

	if url != "rtsp://relay.example:8554/bob" {
		t.Errorf("WatchURL = %q, want rtsp://relay.example:8554/bob", url)
	}
}
