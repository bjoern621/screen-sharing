package watch

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/settings"
)

func srtStream() settings.Stream {
	return settings.Stream{
		Name:                "alice",
		RelayHost:           "relay.example",
		RelayPort:           8890,
		Transport:           "srt",
		SrtWatchLatencyMs:   1200,
		SrtPublishLatencyMs: 300,
	}
}

// rtspStream picks a watch-leg protocol that is not the default, so a viewer
// still forcing TCP fails these tests instead of passing them by accident.
func rtspStream() settings.Stream {
	s := srtStream()
	s.Transport = "rtsp"
	s.RtspPort = 8554
	s.RtspWatchLatencyMs = 350
	s.RtspWatchProtocol = "udp"
	return s
}

func TestSelectDefaultsToFfplay(t *testing.T) {
	t.Setenv(EnvViewer, "")
	engine, err := Select("srt")
	if err != nil {
		t.Fatal(err)
	}
	if engine.Exe() != "ffplay" {
		t.Errorf("Exe() = %q, want ffplay", engine.Exe())
	}
}

func TestSelectEnvPicksMpv(t *testing.T) {
	t.Setenv(EnvViewer, "mpv")
	engine, err := Select("srt")
	if err != nil {
		t.Fatal(err)
	}
	if engine.Exe() != "mpv" {
		t.Errorf("Exe() = %q, want mpv", engine.Exe())
	}
}

func TestSelectRejectsTransportWithoutWatchForm(t *testing.T) {
	if _, err := Select("webrtc"); err == nil {
		t.Fatal("expected error for a transport without a watch form")
	}
	if _, err := Select("carrier-pigeon"); err == nil {
		t.Fatal("expected error for an unknown transport")
	}
}

func TestFfplayCommand(t *testing.T) {
	args, _, err := ffplay{}.Command(srtStream(), "bob", "srt")
	if err != nil {
		t.Fatal(err)
	}

	i := slices.Index(args, "-window_title")
	if i < 0 || args[i+1] != WindowTitle("bob", "srt") {
		t.Errorf("missing -window_title %q in %v", WindowTitle("bob", "srt"), args)
	}
	if url := args[len(args)-1]; !strings.HasPrefix(url, "srt://") {
		t.Errorf("watch URL = %q, want srt:// prefix", url)
	}
	if slices.Contains(args, "-rtsp_transport") {
		t.Errorf("srt must not carry the RTSP transport flag, got %v", args)
	}
}

func TestFfplayCommandRTSPTakesWatchProtocol(t *testing.T) {
	args, _, err := ffplay{}.Command(rtspStream(), "bob", "rtsp")
	if err != nil {
		t.Fatal(err)
	}

	i := slices.Index(args, "-rtsp_transport")
	if i < 0 || args[i+1] != "udp" {
		t.Errorf("missing -rtsp_transport udp in %v", args)
	}
	if url := args[len(args)-1]; !strings.HasPrefix(url, "rtsp://") {
		t.Errorf("watch URL = %q, want rtsp:// prefix", url)
	}
}

func TestMpvCommandRTSPTakesWatchProtocol(t *testing.T) {
	args, _, err := mpv{}.Command(rtspStream(), "bob", "rtsp")
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(args, "--rtsp-transport=udp") {
		t.Errorf("missing --rtsp-transport=udp in %v", args)
	}
	if !slices.Contains(args, "--title="+WindowTitle("bob", "rtsp")) {
		t.Errorf("missing --title in %v", args)
	}
}

func TestCommandUnknownTransport(t *testing.T) {
	if _, _, err := (ffplay{}).Command(srtStream(), "bob", "carrier-pigeon"); err == nil {
		t.Fatal("expected error for unknown transport")
	}
	if _, _, err := (mpv{}).Command(srtStream(), "bob", "carrier-pigeon"); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestWindowTitle(t *testing.T) {
	if got := WindowTitle("bob", "srt"); got != "watch: bob [srt]" {
		t.Errorf("WindowTitle = %q, want \"watch: bob [srt]\"", got)
	}
}
