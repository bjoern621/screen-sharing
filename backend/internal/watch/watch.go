// Package watch builds the viewer command that plays a stream back.
//
// The publish package's engine seam from the other side of the wire:
// each viewer engine serializes a stream into its own command line,
// asking the transport registry for the form it consumes.
//
// ffplay and mpv drive any transport with a URL watch form (transport.Watcher).
// WebRTC has none, its playback protocol being WHEP,
// and would take an engine keyed on a transport capability of its own:
// another engine in this package, and no app-layer change.
package watch

import (
	"fmt"
	"os"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// Engine is one viewer program.
// Exe names the executable; the app layer resolves it to a path and supervises the process.
type Engine interface {
	Exe() string
	// Command opens streamName in a viewer window, received over the named transport.
	Command(s settings.Settings, streamName, transportName string) (args, env []string, err error)
}

// EnvViewer selects the viewer program:
// "mpv" switches away from the default ffplay, for comparing the two on a machine without a rebuild.
const EnvViewer = "SCREENSHARE_VIEWER"

// Select returns the viewer engine for the named watch transport.
// The leg is per viewer rather than the publish settings',
// so a stream can be received over any transport the relay serves it on.
//
// A transport no registry row names is an Umgebungsfehler, the value living in a file the user owns,
// so it leaves as an error rather than an assert.
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

// WindowTitle carries the transport beside the stream name,
// so a user watching one stream over several legs at once can tell the windows apart.
func WindowTitle(streamName, transportName string) string {
	assert.Assert(streamName != "", "a viewer window names the stream it plays", transportName)
	assert.Assert(transportName != "", "a viewer window names the leg it received on", streamName)

	return "watch: " + streamName + " [" + transportName + "]"
}
