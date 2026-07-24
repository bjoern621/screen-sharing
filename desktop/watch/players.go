package watch

import (
	"fmt"
	"runtime"
	"strings"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// watchURL returns the named transport's viewer URL, shared by both URL players.
func watchURL(s settings.Stream, streamName, transportName string) (string, error) {
	url, ok := transport.WatchURL(transportName, s, streamName)
	if !ok {
		return "", fmt.Errorf("transport %q has no watch form", transportName)
	}
	return url, nil
}

// isRTSP reports whether url selects ffmpeg's RTSP demuxer, whose transport
// option cannot ride in the URL and must be injected as a player argument.
func isRTSP(url string) bool {
	return strings.HasPrefix(url, "rtsp://")
}

// ffplay is the default viewer.
type ffplay struct{}

func (ffplay) Exe() string { return "ffplay" }

// Command builds the ffplay arguments that open streamName in a low-latency
// viewer window.
//
// -nostats drops ffplay's per-frame status line, whose blocking write to a
// console would otherwise stall the decode loop and expire SRT packets. The
// remaining -loglevel info reaches the run log through a drained pipe, never a
// console, so it cannot stall the decoder. It records the negotiated input
// format and any decode or filtergraph error, which is what a viewer that stays
// on "connecting" leaves behind.
//
// The environment pins SDL to the X11 (XWayland) backend on Linux, whose window
// the compositor renders reliably where the SDL Wayland backend may not.
func (ffplay) Command(s settings.Stream, streamName, transportName string) (args, env []string, err error) {
	url, err := watchURL(s, streamName, transportName)
	if err != nil {
		return nil, nil, err
	}

	args = []string{
		"-hide_banner", "-loglevel", "info", "-nostats",
		"-fflags", "nobuffer", "-flags", "low_delay", "-framedrop",
		"-window_title", WindowTitle(streamName, transportName),
	}
	// TCP-interleaved RTP, for the same reason the publish side forces it: the
	// UDP default negotiates per-track ports that NAT drops, and lost RTP over
	// UDP is never retransmitted.
	if isRTSP(url) {
		args = append(args, "-rtsp_transport", "tcp")
	}
	args = append(args, url)

	if runtime.GOOS == "linux" {
		env = []string{"SDL_VIDEODRIVER=x11"}
	}
	return args, env, nil
}

// mpv is the alternative viewer, selected by EnvViewer. It renders 4:4:4 and a
// native Wayland window that ffplay's SDL path does not, so it needs no
// environment overrides.
type mpv struct{}

func (mpv) Exe() string { return "mpv" }

// Command builds the mpv arguments that open streamName in a low-latency
// viewer window.
//
// --profile=low-latency drops buffering and display sync. --no-config isolates
// the viewer from a user's mpv.conf. --force-window=immediate shows the window
// before the first frame, so a slow SRT handshake still gives feedback.
func (mpv) Command(s settings.Stream, streamName, transportName string) (args, env []string, err error) {
	url, err := watchURL(s, streamName, transportName)
	if err != nil {
		return nil, nil, err
	}

	args = []string{
		"--no-config",
		"--msg-level=all=v",
		"--profile=low-latency",
		"--force-window=immediate",
		"--network-timeout=10",
		"--title=" + WindowTitle(streamName, transportName),
	}
	// See the ffplay counterpart for why RTSP is forced onto TCP.
	if isRTSP(url) {
		args = append(args, "--rtsp-transport=tcp")
	}
	args = append(args, url)
	return args, env, nil
}
