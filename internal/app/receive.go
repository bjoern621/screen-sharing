package app

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/wire"
)

// StartReceive opens a decode for one stream on one leg, inside this process.
//
// It is the tile path's counterpart of StartWatch, and the difference between the two
// is where the frames end up: a watch spawns a player window this process does not draw
// in, and a receive builds a GStreamer pipeline here, from where the frame channel will
// hand the frames to the shell (docs/viewer-architecture.md).
//
// What it opens is a decode and not a tile. Nothing here knows where the frames are
// drawn or beside which others: that is the shell's, and it is why the grid this
// replaces was the wrong surface.
//
// The leg is passed in rather than read off the settings for the reason StartWatch
// passes one in: the setting is what a shell offers by default, and the call is what
// was chosen. The render chain is not, because it is one value for every decode - a
// chain falls back where a driver cannot run it, which is a property of the machine.
func (a *App) StartReceive(streamName, transportName string) error {
	assert.Assert(streamName != "", "a decode names the stream it receives")
	assert.Assert(transportName != "", "a decode names the leg it receives over", streamName)

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	if err := a.carriesStream(streamName, transportName, capabilities.EngineGst); err != nil {
		return err
	}

	source, ok := transport.GstSource(transportName, s, streamName)
	if !ok {
		return fmt.Errorf("transport %q has no GStreamer watch form, so no pipeline can receive over it", transportName)
	}

	// Announced after the lock is released, for the reason StartWatch defers its own:
	// the state is read back through the map this function holds the lock on.
	defer a.emitReceiveState()

	a.procMu.Lock()
	defer a.procMu.Unlock()

	key := WatchKey{Name: streamName, Transport: transportName}
	if _, present := a.receivers[key]; present {
		return fmt.Errorf("already receiving %s over %s", streamName, transportName)
	}

	stream := receive.Stream{
		Name:      streamName,
		Transport: transportName,
		Source:    joinSource(source),
	}
	receiver, err := receive.New(stream, receive.Open{Chain: s.Viewer.RenderChain}, receive.Events{
		// A first frame changes what the state reports - the chain that ran, the memory
		// the pads negotiated - so it is announced like every other change.
		OnLive: a.emitReceiveState,
		OnEnd: func(message string) {
			a.receiveEnded(key, message)
		},
	})
	if err != nil {
		return err
	}

	a.receivers[key] = receiver
	logger.Infof("receiving '%s' over %s", streamName, transportName)
	return nil
}

// StopReceive closes one running decode. A stream nothing is decoding is not an error,
// for the reason StopWatch takes a viewer that is already gone: a stop is what the user
// asked for and it is already true.
func (a *App) StopReceive(streamName, transportName string) {
	defer a.emitReceiveState()

	a.procMu.Lock()
	receiver, present := a.receivers[WatchKey{Name: streamName, Transport: transportName}]
	delete(a.receivers, WatchKey{Name: streamName, Transport: transportName})
	a.procMu.Unlock()

	if !present {
		return
	}
	// Outside the lock: a teardown blocks on the pipeline reaching NULL, and every
	// other method that touches the receivers would wait behind it.
	receiver.Stop()
	logger.Infof("stopped receiving '%s' over %s", streamName, transportName)
}

// receiveEnded drops a pipeline that ended on its own and says why.
//
// The pipeline has already stopped itself by the time this runs, so nothing is torn
// down here: a dead receive pipeline has nothing to recover. The exit says why that one
// ended and the state beside it says which decodes are left, which is the same pair a
// viewer that closed on its own produces.
func (a *App) receiveEnded(key WatchKey, message string) {
	a.procMu.Lock()
	_, present := a.receivers[key]
	delete(a.receivers, key)
	a.procMu.Unlock()

	if !present {
		// A stop the user asked for takes the receiver out first, and the teardown then
		// ends the bus watch. Announcing an exit here would report a decode ending that
		// the shell already knows it closed.
		return
	}
	logger.Warnf("receiving '%s' over %s ended: %s", key.Name, key.Transport, message)
	a.emit(wire.ReceiveExitEvent(wire.WatchKey{StreamName: key.Name, Transport: key.Transport}, message))
	a.emitReceiveState()
}

// ReceiveState is what every running decode turned out to be, read off the pipelines
// rather than remembered.
//
// Nothing here is cached, which is the same rule the publish stats follow: a chain
// falls back at build time and the memory features settle when the pads negotiate, so a
// state assembled from what a caller believed it started would report the chain that was
// asked for rather than the one that ran.
func (a *App) ReceiveState() []wire.ReceiveStream {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	out := make([]wire.ReceiveStream, 0, len(a.receivers))
	for key, receiver := range a.receivers {
		stats := receiver.Stats()
		out = append(out, wire.ReceiveStream{
			Stream:       wire.WatchKey{StreamName: key.Name, Transport: key.Transport},
			Live:         stats.Frames > 0,
			Chain:        stats.Chain,
			DecodeMemory: stats.DecodeMemory,
			RenderMemory: stats.RenderMemory,
			Decoder:      stats.Decoder,
			Hardware:     stats.Hardware,
		})
	}
	return out
}

// emitReceiveState announces the running decodes.
//
// It reads them back through ReceiveState rather than being handed a set, so what is
// announced is what a read would answer with rather than what a caller believed it had
// just done. It takes procMu, so a caller holding that lock defers this rather than
// calling it in place.
func (a *App) emitReceiveState() {
	a.emit(wire.ReceiveStateEvent(a.ReceiveState()))
}

// joinSource is the transport's source elements as one launch-line fragment.
//
// The transport yields them as arguments because that is what a spawned
// gst-launch-1.0 takes, and this side parses a line instead, so the two forms meet
// here rather than the transport declaring both. A space is all that is between them:
// the arguments are one element and its properties, and an entry that is a second
// element carries its own "!" the way the publish sinks do.
func joinSource(elements []string) string {
	assert.Assert(len(elements) > 0, "a watching transport yields source elements")
	return strings.Join(elements, " ")
}
