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

func rtspStream() settings.Stream {
	s := srtStream()
	s.Transport = "rtsp"
	s.RtspPort = 8554
	return s
}

func TestSelectDefaultsToFfplay(t *testing.T) {
	t.Setenv(EnvViewer, "")
	engine, err := Select(srtStream())
	if err != nil {
		t.Fatal(err)
	}
	if engine.Exe() != "ffplay" {
		t.Errorf("Exe() = %q, want ffplay", engine.Exe())
	}
}

func TestSelectEnvPicksMpv(t *testing.T) {
	t.Setenv(EnvViewer, "mpv")
	engine, err := Select(srtStream())
	if err != nil {
		t.Fatal(err)
	}
	if engine.Exe() != "mpv" {
		t.Errorf("Exe() = %q, want mpv", engine.Exe())
	}
}

func TestSelectRejectsTransportWithoutWatchForm(t *testing.T) {
	s := srtStream()
	s.Transport = "webrtc"
	if _, err := Select(s); err == nil {
		t.Fatal("expected error for a transport without a watch form")
	}

	s.Transport = "carrier-pigeon"
	if _, err := Select(s); err == nil {
		t.Fatal("expected error for an unknown transport")
	}
}

func TestFfplayCommand(t *testing.T) {
	args, _, err := ffplay{}.Command(srtStream(), "bob")
	if err != nil {
		t.Fatal(err)
	}

	i := slices.Index(args, "-window_title")
	if i < 0 || args[i+1] != WindowTitle("bob") {
		t.Errorf("missing -window_title %q in %v", WindowTitle("bob"), args)
	}
	if url := args[len(args)-1]; !strings.HasPrefix(url, "srt://") {
		t.Errorf("watch URL = %q, want srt:// prefix", url)
	}
	if slices.Contains(args, "-rtsp_transport") {
		t.Errorf("srt must not carry the RTSP transport flag, got %v", args)
	}
}

func TestFfplayCommandRTSPForcesTCP(t *testing.T) {
	args, _, err := ffplay{}.Command(rtspStream(), "bob")
	if err != nil {
		t.Fatal(err)
	}

	i := slices.Index(args, "-rtsp_transport")
	if i < 0 || args[i+1] != "tcp" {
		t.Errorf("missing -rtsp_transport tcp in %v", args)
	}
	if url := args[len(args)-1]; !strings.HasPrefix(url, "rtsp://") {
		t.Errorf("watch URL = %q, want rtsp:// prefix", url)
	}
}

func TestMpvCommandRTSPForcesTCP(t *testing.T) {
	args, _, err := mpv{}.Command(rtspStream(), "bob")
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(args, "--rtsp-transport=tcp") {
		t.Errorf("missing --rtsp-transport=tcp in %v", args)
	}
	if !slices.Contains(args, "--title="+WindowTitle("bob")) {
		t.Errorf("missing --title in %v", args)
	}
}

func TestCommandUnknownTransport(t *testing.T) {
	s := srtStream()
	s.Transport = "carrier-pigeon"
	if _, _, err := (ffplay{}).Command(s, "bob"); err == nil {
		t.Fatal("expected error for unknown transport")
	}
	if _, _, err := (mpv{}).Command(s, "bob"); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestWindowTitle(t *testing.T) {
	if got := WindowTitle("bob"); got != "watch: bob" {
		t.Errorf("WindowTitle = %q, want \"watch: bob\"", got)
	}
}
