package watch

import (
	"fmt"
	"runtime"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// watchURL returns the named transport's viewer URL, shared by both URL players.
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

// isRTSP reports whether url selects ffmpeg's RTSP demuxer, whose transport option cannot ride in
// the URL and must be injected as a player argument.
func isRTSP(url string) bool {
	return strings.HasPrefix(url, "rtsp://")
}

// Player fetches this over HTTP, the leg whose credential is a header rather than part of the
// address (internal/transport, credential.go).
func isHTTP(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// ffplay is the default viewer.
type ffplay struct{}

func (ffplay) Exe() string { return "ffplay" }

// Command builds the ffplay arguments that open streamName in a low-latency viewer window.
//
// -nostats drops ffplay's per-frame status line, whose blocking write to a console would otherwise
// stall the decode loop and expire SRT packets.
// The remaining -loglevel info reaches the run log through a drained pipe, never a console,
// so it cannot stall the decoder.
// It records the negotiated input format and any decode or filtergraph error,
// which is what a viewer that stays on "connecting" leaves behind.
//
// The environment pins SDL to the X11 (XWayland) backend on Linux, whose window the compositor
// renders reliably where the SDL Wayland backend may not.
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
	// The RTP lower transport is the watch-leg setting, the same one the native grid gives rtspsrc,
	// so one stream reaches both viewers the same way.
	// It cannot ride in the URL, unlike SRT's options.
	if isRTSP(url) {
		args = append(args, "-rtsp_transport", s.Viewer.RtspWatchProtocol)
	}
	// libavformat sends these on every request the demuxer makes, which is what a playlist and its
	// segments need.
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

// mpv is the alternative viewer, selected by EnvViewer.
// It renders 4:4:4 and a native Wayland window that ffplay's SDL path does not,
// so it needs no environment overrides.
type mpv struct{}

func (mpv) Exe() string { return "mpv" }

// Command builds the mpv arguments that open streamName in a low-latency viewer window.
//
// --profile=low-latency drops buffering and display sync.
// --no-config isolates the viewer from a user's mpv.conf.
// --force-window=immediate shows the window before the first frame, so a slow SRT handshake still
// gives feedback.
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
	// See the ffplay counterpart for where the RTP lower transport comes from.
	if isRTSP(url) {
		args = append(args, "--rtsp-transport="+s.Viewer.RtspWatchProtocol)
	}
	// See the ffplay counterpart.
	if name, value, ok := transport.CredentialHeader(s); ok && isHTTP(url) {
		args = append(args, "--http-header-fields="+name+": "+value)
	}
	args = append(args, url)

	assert.Assert(args[len(args)-1] == url, "a player is launched on the URL it was built for", url)
	return args, env, nil
}
