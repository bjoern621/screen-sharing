// Package player is the decode seam: one stream's receive pipeline, seen from
// the grid.
//
// A player owns its pipeline from source to sink and exposes the decoded video
// as a GdkPaintable a GtkPicture draws. The grid depends on this contract, not
// on GStreamer, mirroring how the app's publish side hides its engines behind
// publish.Publisher. Backends register themselves (see player/gstreamer), so
// another decoder is one package and one registration.
package player

import (
	"fmt"
	"slices"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// Player is one stream's running receive pipeline.
type Player interface {
	// Paintable is the sink's render target. It is valid from construction; it
	// paints nothing until the first frame arrives.
	Paintable() *gdk.Paintable
	// SetVolume sets the audio branch's volume, 0 to 1. A no-op until OnAudio
	// fired.
	SetVolume(v float64)
	// SetMuted mutes or unmutes the audio branch. A no-op until OnAudio fired.
	SetMuted(muted bool)
	// Stats reports the decode figures for the stats overlay. Fields the
	// pipeline has not learned yet are zero.
	Stats() Stats
	// Stop tears the pipeline down. Safe to call after OnEnd fired.
	Stop()
}

// Events are one player's lifecycle callbacks. They fire on pipeline threads,
// not on the UI loop; the caller hops the ones that touch widgets.
type Events struct {
	// OnLive fires once, when the first decoded frame reaches the paintable.
	// Its meaning matches the web grid's "connected": a frame on the surface,
	// not a transport coming up.
	OnLive func()
	// OnAudio fires once, when the stream turns out to carry audio and the
	// audio branch is playing. A tile only offers its volume control after
	// this, like the web tile hides VolumeControl on video-only sinks.
	OnAudio func()
	// OnEnd fires once on a fatal pipeline error or end of stream, with a
	// human-readable message. The pipeline is already stopped; a dead receive
	// pipeline has nothing to recover, so the tile keeps its last frame under
	// the error label.
	OnEnd func(message string)
}

// Factory opens the receive pipeline for one configured stream. The grid takes a
// factory rather than a concrete constructor, so its tiles can be driven by
// another backend or by a stub in a test.
type Factory func(st roster.Stream, ev Events) (Player, error)

// backends holds the registered decode backends by name.
var backends = map[string]Factory{}

// Register adds a decode backend. Registering the same name twice is a
// programming error.
func Register(name string, f Factory) {
	assert.Assert(name != "", "a backend registers under a name")
	assert.IsNotNil(f, "a backend registers a factory", name)
	_, exists := backends[name]
	assert.Assert(!exists, "player backend registered twice", name)

	backends[name] = f
	logger.Debugf("player backend %q registered", name)
}

// For returns the factory registered under name.
func For(name string) (Factory, error) {
	f, ok := backends[name]
	if !ok {
		return nil, fmt.Errorf("unknown player backend %q, have %v", name, Names())
	}
	return f, nil
}

// Names lists the registered backends, sorted for a stable order in the flag
// help the process prints.
func Names() []string {
	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
