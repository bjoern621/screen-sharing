package app

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/colour"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/wire"
)

// StartReceive opens a decode for one stream on one leg, inside this process.
//
// It is the tile path's counterpart of StartWatch, and the difference between the two is where the
// frames end up: a watch spawns a player window this process does not draw in,
// and a receive builds a GStreamer pipeline here, from where the frame channel will hand the frames
// to the shell (docs/viewer-architecture.md).
//
// What it opens is a decode and not a tile.
// Nothing here knows where the frames are drawn or beside which others: that is the shell's,
// and it is why the grid this replaces was the wrong surface.
//
// The leg is passed in rather than read off the settings for the reason StartWatch passes one in:
// the setting is what a shell offers by default, and the call is what was chosen.
// The render chain is not, because it is one value for every decode - a chain falls back where a
// driver cannot run it, which is a property of the machine.
//
// Tone mapping is passed in for the opposite reason: whether a stream carries more range than a
// display shows is a property of that stream, and two tiles on one stream may want different
// answers.
// It is a build choice - the rung is an element of the pipeline - so a decode already running with
// the other answer is rebuilt, which is what makes this call name the state it wants rather than a
// transition.
func (a *App) StartReceive(streamName, transportName string, toneMap bool) error {
	assert.Assert(streamName != "", "a decode names the stream it receives")
	assert.Assert(transportName != "", "a decode names the leg it receives over", streamName)

	// Announced after the lock is released, for the reason StartWatch defers its own:
	// the state is read back through the map this function holds the lock on.
	// It runs on the refusals below too, where it announces the set unchanged - every event carries a
	// whole state, so a duplicate is harmless and the alternative is an effect that sometimes
	// announces nothing.
	defer a.emitReceiveState()

	key := WatchKey{Name: streamName, Transport: transportName}

	// What a decode asking for tone mapping is built with, which on a machine with no rung is not what
	// was asked.
	// The running decode is compared against this rather than against the request,
	// because a request this machine cannot reach is a state no rebuild brings any closer:
	// held the other way round, the same pipeline would be torn down on every call.
	wanted := receive.WillToneMap(toneMap)

	// Read before anything is validated, and that order is the whole of what makes this method safe to
	// repeat.
	// Asking for a decode that is already open is not an error, because it is not a second decode:
	// this method names the state it wants and that state is already true.
	// It used to refuse, which made a retry unsafe to send - a shell whose answer was lost could not
	// find out whether it had been heard without risking a refusal for having been,
	// and so had nothing to do but wait for an answer that was never coming.
	//
	// Validating first would put the same trap back one step further out: the relay's snapshot moves
	// under a running decode - the publisher republishes in a format this leg does not carry - and the
	// resend would then fail a precondition on behalf of a state that already holds.
	if a.receiving(key, wanted) {
		logger.Debugf("'%s' over %s is already being received", streamName, transportName)
		return nil
	}

	// A decode running with the other answer about tone mapping is not the state this call names,
	// so it is taken down and built again.
	// Again rather than adjusted, because the rung is an element of the pipeline and there is no
	// property to write: the tile goes dark for as long as one decode takes to open.
	if replaced := a.replacedReceiver(key, wanted); replaced != nil {
		logger.Infof("rebuilding the decode of '%s' over %s %s tone mapping",
			streamName, transportName, withOrWithout(wanted))
		replaced.Stop()
	}

	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	s, err := a.settingsForCommand(s)
	if err != nil {
		return err
	}

	if err := a.carriesStream(streamName, transportName, capabilities.EngineGst); err != nil {
		return err
	}

	source, ok := transport.GstSource(transportName, s, streamName)
	if !ok {
		return fmt.Errorf("transport %q has no GStreamer watch form, so no pipeline can receive over it", transportName)
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if _, present := a.receivers[key]; present {
		// The same question again, under the lock this time: the read above is what keeps a repeat cheap
		// and this is what keeps two starts racing for one pair from building two pipelines.
		return nil
	}

	stream := receive.Stream{
		Name:      streamName,
		Transport: transportName,
		Source:    joinSource(source),
	}
	open := receive.Open{Chain: s.Viewer.RenderChain, ToneMap: toneMap}
	receiver, err := receive.New(stream, open, receive.Events{
		// A first frame changes what the state reports - the chain that ran, the memory the pads
		// negotiated - so it is announced like every other change.
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

// receiving reports whether a decode is open for the pair and was built with the tone mapping this
// call would build with.
//
// It reads the map rather than anything a caller believed it had started, which is what lets it
// stand in front of the validation StartReceive would otherwise run over a state that already
// holds.
//
// Both halves are the state the call names, so a decode running with the other answer reads as
// absent here: it is not the decode that was asked for.
func (a *App) receiving(key WatchKey, toneMap bool) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	receiver, present := a.receivers[key]
	return present && receiver.ToneMap() == toneMap
}

// replacedReceiver takes the decode open for this pair out of the set where it was built with
// another answer about tone mapping, and hands it back for the caller to stop.
//
// Taken out under the lock and stopped outside it, which is StopReceive's own order:
// a teardown blocks on the pipeline reaching NULL and every other method that touches the receivers
// would wait behind it.
func (a *App) replacedReceiver(key WatchKey, toneMap bool) *receive.Receiver {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	receiver, present := a.receivers[key]
	if !present || receiver.ToneMap() == toneMap {
		return nil
	}
	delete(a.receivers, key)
	return receiver
}

// withOrWithout reads a build choice into the line that reports it.
func withOrWithout(on bool) string {
	if on {
		return "with"
	}
	return "without"
}

// StopReceive closes one running decode.
// A stream nothing is decoding is not an error, for the reason StopWatch takes a viewer that is
// already gone: a stop is what the user asked for and it is already true.
func (a *App) StopReceive(streamName, transportName string) {
	assert.Assert(streamName != "", "a stop names the stream whose decode it closes")
	assert.Assert(transportName != "", "a stop names the leg the decode receives over", streamName)

	defer a.emitReceiveState()

	key := WatchKey{Name: streamName, Transport: transportName}

	a.procMu.Lock()
	receiver, present := a.receivers[key]
	delete(a.receivers, key)
	a.procMu.Unlock()

	if !present {
		return
	}
	// Outside the lock: a teardown blocks on the pipeline reaching NULL, and every other method that
	// touches the receivers would wait behind it.
	receiver.Stop()
	logger.Infof("stopped receiving '%s' over %s", streamName, transportName)
}

// SetReceiveAudio sets how loud one decode plays, and whether it plays at all.
//
// The volume belongs to the decode rather than to a window drawing it, because the audio branch is
// one element inside one pipeline: two windows on one decode share it, and a per-window volume
// would be two controls over one element each showing a value the other had overwritten
// (docs/viewer-architecture.md).
//
// Safe to repeat in the sense the whole control contract is: the receiver holds what it was asked
// for and writes it onto the branch, so the same call on a decode already at that loudness leaves
// the same state behind, and one that arrives before the decoder has exposed an audio pad is
// applied when it does rather than lost.
//
// A decode that does not exist is refused.
// That is not the case a repeat produces - a stop takes the decode away and a volume for it
// afterwards is a request about something absent, not a request for a state that already holds.
func (a *App) SetReceiveAudio(streamName, transportName string, volume float64, muted bool) error {
	assert.Assert(streamName != "", "a loudness names the stream it belongs to")
	assert.Assert(transportName != "", "a loudness names the leg the decode receives over", streamName)

	// Announced after the write, because the loudness is part of what the receive state reports and a
	// shell that did not make the change learns it there.
	defer a.emitReceiveState()

	a.procMu.Lock()
	receiver, present := a.receivers[WatchKey{Name: streamName, Transport: transportName}]
	a.procMu.Unlock()

	if !present {
		return fmt.Errorf("nothing is receiving '%s' over %s", streamName, transportName)
	}

	// Bounded here rather than refused, so a slider that ran past its end is a value the backend
	// brings back rather than an error a reader has to understand.
	receiver.SetAudio(min(max(volume, 0), 1), muted)
	return nil
}

// AudioLevels is how loud every decode carrying audio is, at this instant.
//
// Read off the pipelines rather than accumulated, for the reason ReceiveState is:
// what a reader is told is what a read answers now.
// A decode with no audio branch, or one whose branch has posted no measurement yet,
// has no entry at all - absence and silence are different facts, and a floor invented here would
// erase the difference.
func (a *App) AudioLevels() []wire.AudioLevel {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	out := make([]wire.AudioLevel, 0, len(a.receivers))
	for key, receiver := range a.receivers {
		peak, rms, ok := receiver.Level()
		if !ok {
			continue
		}
		out = append(out, wire.AudioLevel{
			Stream: wire.WatchKey{StreamName: key.Name, Transport: key.Transport},
			PeakDB: peak,
			RMSDB:  rms,
		})
	}
	return out
}

// receiveEnded drops a pipeline that ended on its own and says why.
//
// The pipeline has already stopped itself by the time this runs, so nothing is torn down here:
// a dead receive pipeline has nothing to recover.
// The exit says why that one ended and the state beside it says which decodes are left,
// which is the same pair a viewer that closed on its own produces.
func (a *App) receiveEnded(key WatchKey, message string) {
	a.procMu.Lock()
	_, present := a.receivers[key]
	delete(a.receivers, key)
	a.procMu.Unlock()

	if !present {
		// A stop the user asked for takes the receiver out first, and the teardown then ends the bus
		// watch.
		// Announcing an exit here would report a decode ending that the shell already knows it closed.
		return
	}
	logger.Warnf("receiving '%s' over %s ended: %s", key.Name, key.Transport, message)
	a.emit(wire.ReceiveExitEvent(wire.WatchKey{StreamName: key.Name, Transport: key.Transport}, message))
	a.emitReceiveState()
}

// ReceiveState is what every running decode turned out to be, read off the pipelines rather than
// remembered.
//
// Nothing here is cached, which is the same rule the publish stats follow:
// a chain falls back at build time and the memory features settle when the pads negotiate,
// so a state assembled from what a caller believed it started would report the chain that was asked
// for rather than the one that ran.
func (a *App) ReceiveState() []wire.ReceiveStream {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	// One read for every decode: what this machine can roll an HDR stream down with is the machine's
	// answer and the same for all of them, and a tile reads it beside the stream it is deciding about.
	offer := receive.ToneMapping()

	out := make([]wire.ReceiveStream, 0, len(a.receivers))
	for key, receiver := range a.receivers {
		stats := receiver.Stats()
		volume, muted, hasAudio := receiver.Audio()
		out = append(out, wire.ReceiveStream{
			Stream:         wire.WatchKey{StreamName: key.Name, Transport: key.Transport},
			Live:           stats.Frames > 0,
			Chain:          stats.Chain,
			DecodeMemory:   stats.DecodeMemory,
			RenderMemory:   stats.RenderMemory,
			Decoder:        stats.Decoder,
			Hardware:       stats.Hardware,
			HasAudio:       hasAudio,
			Volume:         volume,
			Muted:          muted,
			Transfer:       stats.Transfer,
			HDR:            colour.IsHDR(stats.Transfer),
			ToneMap:        stats.ToneMap,
			CanToneMap:     offer.Available,
			ToneMapMissing: offer.MissingElement,
		})
	}
	return out
}

// emitReceiveState announces the running decodes.
//
// It reads them back through ReceiveState rather than being handed a set, so what is announced is
// what a read would answer with rather than what a caller believed it had just done.
// It takes procMu, so a caller holding that lock defers this rather than calling it in place.
func (a *App) emitReceiveState() {
	a.emit(wire.ReceiveStateEvent(a.ReceiveState()))
}

// joinSource is the transport's source elements as one launch-line fragment.
//
// The transport yields them as arguments because that is what a spawned gst-launch-1.0 takes,
// and this side parses a line instead, so the two forms meet here rather than the transport
// declaring both.
// A space is all that is between them: the arguments are one element and its properties,
// and an entry that is a second element carries its own "!"
// the way the publish sinks do.
func joinSource(elements []string) string {
	assert.Assert(len(elements) > 0, "a watching transport yields source elements")
	return strings.Join(elements, " ")
}
