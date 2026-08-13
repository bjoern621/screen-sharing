// Package watch selects and builds the viewer command that plays a stream back.
//
// It mirrors the publish package's engine seam from the other side of the wire:
// each viewer engine serializes a stream into its own command line, asking the transport registry
// for the form it consumes.
//
// ffplay and mpv drive any transport with a URL watch form (transport.Watcher).
// A transport without one, WebRTC, whose playback protocol is WHEP, needs an engine keyed on a
// transport capability of its own; adding one means another engine in this package and no app-layer
// changes.
package watch

import (
	"fmt"
	"os"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// Engine is one viewer program.
// Exe names the executable to run; the app layer resolves it to a path and supervises the process.
type Engine interface {
	Exe() string
	// Command returns the arguments and environment overrides that open streamName in a viewer window,
	// receiving it over the named transport.
	Command(s settings.Settings, streamName, transportName string) (args, env []string, err error)
}

// EnvViewer selects the viewer program: the value "mpv" switches from the default ffplay to mpv,
// for comparing the two on a machine without a rebuild.
const EnvViewer = "SCREENSHARE_VIEWER"

// Select returns the viewer engine for the named watch transport.
// The transport is chosen per viewer, not taken from the publish settings,
// so a stream can be received over any transport the relay serves it on.
//
// A transport no registry row names is an Umgebungsfehler on the way in, since the value is stored
// in a file the user owns, so it leaves as an error rather than an assert.
func Select(transportName string) (Engine, error) {
	t, ok := transport.Get(transportName)
	if !ok {
		return nil, fmt.Errorf("unknown transport %q", transportName)
	}
	if _, ok := t.(transport.Watcher); !ok {
		return nil, fmt.Errorf("no viewer implements transport %q", transportName)
	}
	if os.Getenv(EnvViewer) == "mpv" {
		return mpv{}, nil
	}
	return ffplay{}, nil
}

// WindowTitle returns the title a viewer sets on its window.
// It carries the transport as well as the stream name so a user watching one stream over several
// transports at once can tell the windows apart.
func WindowTitle(streamName, transportName string) string {
	assert.Assert(streamName != "", "a viewer window names the stream it plays", transportName)
	assert.Assert(transportName != "", "a viewer window names the leg it received on", streamName)

	return "watch: " + streamName + " [" + transportName + "]"
}
