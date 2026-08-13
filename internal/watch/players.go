package watch

import (
	"fmt"
	"runtime"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

func watchURL(s settings.Settings, streamName, transportName string) (string, error) {
	assert.Assert(streamName != "", "a watch URL names the stream it opens", transportName)
	assert.Assert(transportName != "", "a watch URL names the leg it opens on", streamName)

	url, ok := transport.WatchURL(transportName, s, streamName)
	if !ok {
		return "", fmt.Errorf("transport %q has no watch form", transportName)
	}

	assert.Assert(url != "", "a transport with a watch form yields a URL", transportName, streamName)
	return url, nil
}

// isRTSP marks the leg whose lower-transport option cannot ride in the URL and is passed as a
// player argument instead.
func isRTSP(url string) bool {
	return strings.HasPrefix(url, "rtsp://")
}

// isHTTP marks the legs whose credential is a header rather than part of the address
// (internal/transport, credential.go).
func isHTTP(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// ffplay is the viewer Select falls back to.
type ffplay struct{}

func (ffplay) Exe() string { return "ffplay" }

// Command opens streamName in a low-latency ffplay window.
//
// -nostats drops the per-frame status line, whose blocking write to a console would stall the
// decode loop and expire SRT packets.
// The -loglevel info left standing reaches the run log through a drained pipe rather than a
// console, so it cannot stall the decoder, and it records the negotiated input format and any
// decode or filtergraph error, which is what a viewer stuck on "connecting" leaves behind.
//
// The environment pins SDL to the X11 (XWayland) backend on Linux: the compositor renders that
// window reliably where the SDL Wayland backend may not.
func (ffplay) Command(s settings.Settings, streamName, transportName string) (args, env []string, err error) {
	url, err := watchURL(s, streamName, transportName)
	if err != nil {
		return nil, nil, err
	}

	args = []string{
		"-hide_banner", "-loglevel", "info", "-nostats",
		"-fflags", "nobuffer", "-flags", "low_delay", "-framedrop",
		"-window_title", WindowTitle(streamName, transportName),
	}
	// The RTP lower transport is the watch-leg setting, the same one a receiving pipeline gives
	// rtspsrc, so one stream reaches every viewer alike.
	// Unlike SRT's options, it cannot ride in the URL.
	if isRTSP(url) {
		args = append(args, "-rtsp_transport", s.Viewer.RtspWatchProtocol)
	}
	// libavformat repeats these on every request the demuxer makes, which a playlist and its segments
	// both need.
	if name, value, ok := transport.CredentialHeader(s); ok && isHTTP(url) {
		args = append(args, "-headers", name+": "+value+"\r\n")
	}
	args = append(args, url)

	if runtime.GOOS == "linux" {
		env = []string{"SDL_VIDEODRIVER=x11"}
	}

	assert.Assert(args[len(args)-1] == url, "a player is launched on the URL it was built for", url)
	return args, env, nil
}

// mpv is the viewer EnvViewer switches to.
// It renders 4:4:4 and a native Wayland window that ffplay's SDL path does not, so it takes no
// environment overrides.
type mpv struct{}

func (mpv) Exe() string { return "mpv" }

// Command opens streamName in a low-latency mpv window.
//
// --profile=low-latency drops buffering and display sync.
// --no-config keeps a user's mpv.conf out of the viewer.
// --force-window=immediate puts the window up before the first frame, so a slow SRT handshake still
// shows something.
func (mpv) Command(s settings.Settings, streamName, transportName string) (args, env []string, err error) {
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
	// The RTP lower transport, as in the ffplay counterpart.
	if isRTSP(url) {
		args = append(args, "--rtsp-transport="+s.Viewer.RtspWatchProtocol)
	}
	// The credential header, as in the ffplay counterpart.
	if name, value, ok := transport.CredentialHeader(s); ok && isHTTP(url) {
		args = append(args, "--http-header-fields="+name+": "+value)
	}
	args = append(args, url)

	assert.Assert(args[len(args)-1] == url, "a player is launched on the URL it was built for", url)
	return args, env, nil
}
