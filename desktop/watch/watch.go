// Package watch selects and builds the viewer command that plays a stream
// back. It mirrors the publish package's engine seam from the other side of
// the wire: each viewer engine serializes a stream into its own command line,
// asking the transport registry for the form it consumes.
//
// ffplay and mpv drive any transport with a URL watch form (transport.Watcher).
// A transport without one (WebRTC, whose playback protocol is WHEP) needs an
// engine keyed on a transport capability of its own; adding one means another
// engine in this package, no app-layer changes.
package watch

import (
	"fmt"
	"os"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// Engine is one viewer program. Exe names the executable to run; the app layer
// resolves it to a path and supervises the process.
type Engine interface {
	Exe() string
	// Command returns the arguments and environment overrides that open
	// streamName in a viewer window.
	Command(s settings.Stream, streamName string) (args, env []string, err error)
}

// EnvViewer selects the viewer program: the value "mpv" switches from the
// default ffplay to mpv, for comparing the two on a machine without a rebuild.
const EnvViewer = "SCREENSHARE_VIEWER"

// Select returns the viewer engine for the configured transport.
func Select(s settings.Stream) (Engine, error) {
	t, ok := transport.Get(s.Transport)
	if !ok {
		return nil, fmt.Errorf("unknown transport %q", s.Transport)
	}
	if _, ok := t.(transport.Watcher); !ok {
		return nil, fmt.Errorf("no viewer implements transport %q", s.Transport)
	}
	if os.Getenv(EnvViewer) == "mpv" {
		return mpv{}, nil
	}
	return ffplay{}, nil
}

// WindowTitle returns the title a viewer sets on its window, so a user running
// several viewers can tell which stream each window shows.
func WindowTitle(streamName string) string {
	return "watch: " + streamName
}
