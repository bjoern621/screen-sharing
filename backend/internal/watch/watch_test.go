package watch

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

func srtStream() settings.Settings {
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
			TileWatchTransport: "srt",
			SrtWatchLatencyMs:  1200,
		},
	}
}

// rtspStream holds a watch-leg protocol other than the default, so a viewer that forces TCP fails
// these tests rather than passing them by accident.
func rtspStream() settings.Settings {
	s := srtStream()
	s.Publish.Transport = "rtsp"
	s.Viewer.TileWatchTransport = "rtsp"
	s.Relay.RtspPort = 8554
	s.Viewer.RtspWatchLatencyMs = 350
	s.Viewer.RtspWatchProtocol = "udp"
	return s
}

// rtmpStream watches over the leg the relay terminates TLS on wherever it runs, as RTSP is.
func rtmpStream() settings.Settings {
	s := srtStream()
	s.Viewer.TileWatchTransport = "rtmp"
	s.Relay.RtmpPort = 1936
	return s
}

// hlsStream watches over the relay's HTTP leg, whose address carries TLS only where a proxy
// terminates it (settings.Relay.HTTPOrigin).
func hlsStream() settings.Settings {
	s := srtStream()
	s.Viewer.TileWatchTransport = "hls"
	s.Relay.HlsPort = 8888
	return s
}

// acrossTheInternet is the same watch on a relay reached over somebody else's network.
func acrossTheInternet(s settings.Settings) settings.Settings {
	s.Relay.Host = "relay.example"
	return s
}

// watchTlsCase is one leg a player opens, and what the certificate on it is measured against.
type watchTlsCase struct {
	name string
	s    settings.Settings
	leg  string
	// tls marks a leg that terminates TLS, which is where a player states anything at all.
	tls bool
	// verify is what that leg does with the certificate it is handed.
	verify bool
}

// The two deployments per leg, the rule internal/transport holds the pipelines to (tls.go).
// A relay on this network holds a self-signed pair no store carries, and one across somebody else's
// network holds a certificate issued for the name it is reached by.
// A leg carrying no TLS is measured against nothing.
func watchTlsCases() []watchTlsCase {
	return []watchTlsCase{
		{"rtsps across the internet", acrossTheInternet(rtspStream()), "rtsp", true, true},
		{"rtsps on this network", rtspStream(), "rtsp", true, false},
		{"rtmps on this network", rtmpStream(), "rtmp", true, false},
		{"the https leg behind the proxy", acrossTheInternet(hlsStream()), "hls", true, true},
		{"the http leg on this network", hlsStream(), "hls", false, false},
		{"srt, which carries no TLS at all", srtStream(), "srt", false, false},
	}
}

// containsPair reports whether args names this flag with this value beside it.
func containsPair(args []string, flag, value string) bool {
	at := slices.Index(args, flag)
	return at >= 0 && at+1 < len(args) && args[at+1] == value
}

// ffmpeg's tls protocol verifies nothing unless told to, and the option reaches it under whichever
// demuxer opened the address.
// Stated per leg: ffplay refuses to open an input carrying a format option no protocol under it
// reads.
func TestFfplayMeasuresTheRelayCertificateOnEveryTlsLeg(t *testing.T) {
	for _, tc := range watchTlsCases() {
		args, _, err := ffplay{}.Command(tc.s, "bob", tc.leg)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		switch {
		case !tc.tls:
			if slices.Contains(args, "-tls_verify") {
				t.Errorf("%s states a certificate check in %v, and ffplay opens no input carrying an option its protocols do not read",
					tc.name, args)
			}
		case tc.verify:
			if !containsPair(args, "-tls_verify", "1") {
				t.Errorf("%s is watched as %v, which takes whichever certificate arrives", tc.name, args)
			}
		default:
			if !containsPair(args, "-tls_verify", "0") {
				t.Errorf("%s is watched as %v, which validates a certificate nothing issued", tc.name, args)
			}
		}
	}
}

// mpv hands its own flag to the same protocol, and defaults to verifying nothing as well.
func TestMpvMeasuresTheRelayCertificateOnEveryTlsLeg(t *testing.T) {
	for _, tc := range watchTlsCases() {
		args, _, err := mpv{}.Command(tc.s, "bob", tc.leg)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		switch {
		case !tc.tls:
			if slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--tls-verify") }) {
				t.Errorf("%s states a certificate check in %v, and the leg carries no certificate", tc.name, args)
			}
		case tc.verify:
			if !slices.Contains(args, "--tls-verify=yes") {
				t.Errorf("%s is watched as %v, which takes whichever certificate arrives", tc.name, args)
			}
		default:
			if !slices.Contains(args, "--tls-verify=no") {
				t.Errorf("%s is watched as %v, which validates a certificate nothing issued", tc.name, args)
			}
		}
	}
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
	if url := args[len(args)-1]; !strings.HasPrefix(url, "rtsps://") {
		t.Errorf("watch URL = %q, want rtsps:// prefix", url)
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
