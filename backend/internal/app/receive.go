package app

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/colour"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/text"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/wire"
)

// StartReceive opens a decode of one stream on one leg, inside this process.
// A decode already open with the tone mapping this call would build is success, not a second decode.
//
// StartWatch's counterpart, and the difference is where the frames end up.
// A watch spawns a player window this process does not draw in;
// a receive builds a GStreamer pipeline here, which the frame channel hands to the shell
// (docs/viewer-architecture.md).
//
// A decode, never a tile.
// Where the frames are drawn and beside which others is the shell's (docs/ipc-api.md).
//
// The leg is a parameter for the reason StartWatch's is: the setting is what a shell offers by
// default and the call is what was chosen.
// The render chain is not, being one answer for every decode on this machine.
//
// Tone mapping is a parameter for the opposite reason: two tiles on one stream may want different
// answers.
// It is a build choice rather than a property, so a decode running with the other answer is rebuilt.
func (a *App) StartReceive(streamName, transportName string, toneMap bool) error {
	assert.Assert(streamName != "", "a decode names the stream it receives")
	assert.Assert(transportName != "", "a decode names the leg it receives over", streamName)

	// Announced outside the lock, since ReceiveState takes procMu.
	// It runs on the refusals below too, announcing the set unchanged: every event carries a whole
	// state, so a duplicate is harmless and the alternative announces nothing on a repeat.
	defer a.emitReceiveState()

	ref := StreamRef{Name: streamName, Transport: transportName}

	// What asking for tone mapping builds, which on a machine with no rung is not what was asked.
	// Comparing the running decode against the request instead would tear the same pipeline down on
	// every call.
	wanted := receive.WillToneMap(toneMap)

	// The state is read before anything is validated, and that order is what makes a repeat safe.
	// A precondition moves under a decode that already is the state this call names (the relay reports
	// a format this leg stopped carrying), and a validation placed first would refuse the repeat on
	// behalf of a state it was never asked to establish.
	if a.receiving(ref, wanted) {
		logger.Debugf("'%s' over %s is already being received", streamName, transportName)
		return nil
	}

	// Everything that can refuse runs before anything is taken down.
	// The read above is what keeps a repeat safe, and this is what keeps a refusal from costing the
	// decode that was already running: each of the three below is an Umgebungsfehler that a working
	// tile can meet, a group service that went away for a moment being the ordinary one, and a
	// teardown placed first turns "this call could not be honoured" into "and the picture is gone".
	// There is nothing to roll back to once the pipeline is stopped, which is why the order carries
	// the guarantee rather than a recovery path.
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

	// A leg that has to ask the relay where its media is asks here, before anything is taken down,
	// which is what keeps a relay that did not answer from costing the decode already running.
	source, err := transport.ReceiveSource(transportName, s, streamName)
	if err != nil {
		return err
	}

	// A decode running with the other answer is not the state this call names, so it is taken down
	// and built again.
	// Again rather than adjusted: the rung is an element of the pipeline and there is no property to
	// write, so the tile goes dark for as long as one decode takes to open.
	if replaced := a.replacedReceiver(ref, wanted); replaced != nil {
		logger.Infof("rebuilding the decode of '%s' over %s %s tone mapping",
			streamName, transportName, withOrWithout(wanted))
		replaced.Stop()
	}

	a.procMu.Lock()
	defer a.procMu.Unlock()

	if _, present := a.receivers[ref]; present {
		// The same question under the lock: the read above is what keeps a repeat cheap, and this is
		// what keeps two starts racing for one pair from building two pipelines.
		return nil
	}

	stream := receive.Stream{
		Name:      streamName,
		Transport: transportName,
		Source:    joinSource(source),
	}
	open := receive.Open{Chain: s.Viewer.RenderChain, ToneMap: toneMap}
	receiver, err := receive.New(stream, open, receive.Events{
		// A first frame moves what the state reports: the chain that ran, the memory the pads
		// negotiated.
		OnLive: a.emitReceiveState,
		OnEnd: func(message string) {
			a.receiveEnded(ref, message)
		},
	})
	if err != nil {
		return err
	}

	a.receivers[ref] = receiver
	logger.Infof("receiving '%s' over %s", streamName, transportName)
	return nil
}

// receiving is whether a decode is open for the pair and was built with the tone mapping this call
// would build with.
//
// Read off the map rather than off what a caller believed it had started, which is what lets it
// stand in front of StartReceive's validation.
// Both halves are the state the call names, so a decode running with the other answer reads as
// absent.
func (a *App) receiving(ref StreamRef, toneMap bool) bool {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	receiver, present := a.receivers[ref]
	return present && receiver.ToneMap() == toneMap
}

// replacedReceiver takes the pair's decode out of the set where it was built with the other answer
// about tone mapping, and hands it back for the caller to stop.
//
// Out under the lock and stopped outside it, which is StopReceive's order:
// a teardown blocks on the pipeline reaching NULL and every other method touching the receivers
// would wait behind it.
func (a *App) replacedReceiver(ref StreamRef, toneMap bool) *receive.Receiver {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	receiver, present := a.receivers[ref]
	if !present || receiver.ToneMap() == toneMap {
		return nil
	}
	delete(a.receivers, ref)
	return receiver
}

func withOrWithout(on bool) string {
	if on {
		return "with"
	}
	return "without"
}

// StopReceive closes one running decode.
// A stream nothing is decoding is success, for the reason StopWatch takes a viewer that is already
// gone: the state the caller asked for is the one that holds.
func (a *App) StopReceive(streamName, transportName string) {
	assert.Assert(streamName != "", "a stop names the stream whose decode it closes")
	assert.Assert(transportName != "", "a stop names the leg the decode receives over", streamName)

	defer a.emitReceiveState()

	ref := StreamRef{Name: streamName, Transport: transportName}

	a.procMu.Lock()
	receiver, present := a.receivers[ref]
	delete(a.receivers, ref)
	a.procMu.Unlock()

	if !present {
		return
	}
	// Outside the lock: a teardown blocks on the pipeline reaching NULL, and every other method
	// touching the receivers would wait behind it.
	receiver.Stop()
	logger.Infof("stopped receiving '%s' over %s", streamName, transportName)
}

// SetReceiveAudio sets how loud one decode plays, and whether it plays at all.
//
// The loudness belongs to the decode and not to a window drawing it, because the audio branch is one
// element inside one pipeline: two windows on one decode share it, and a per-window volume would be
// two controls over one element (docs/viewer-architecture.md).
//
// Repeatable: the receiver holds what it was asked for and writes it onto the branch, so the same
// call on a decode already at that loudness leaves the same state behind, and one arriving before
// the decoder has exposed an audio pad is applied when it does rather than lost.
//
// A decode that does not exist is refused.
// That is not what a repeat produces: a stop takes the decode away, and a loudness for it afterwards
// names something absent rather than a state that already holds.
func (a *App) SetReceiveAudio(streamName, transportName string, volume float64, muted bool) error {
	assert.Assert(streamName != "", "a loudness names the stream it belongs to")
	assert.Assert(transportName != "", "a loudness names the leg the decode receives over", streamName)

	// Announced after the write: the loudness is part of what the receive state reports, and a shell
	// that did not make the change learns it there.
	defer a.emitReceiveState()

	a.procMu.Lock()
	receiver, present := a.receivers[StreamRef{Name: streamName, Transport: transportName}]
	a.procMu.Unlock()

	if !present {
		return fmt.Errorf("nothing is receiving '%s' over %s", streamName, transportName)
	}

	// Bounded rather than refused, so a slider that ran past its end is a value the backend brings
	// back rather than an error a reader has to understand.
	receiver.SetAudio(min(max(volume, 0), 1), muted)
	return nil
}

// AudioLevels is how loud every decode carrying audio is, at this instant.
//
// Read off the pipelines rather than accumulated, for the reason ReceiveState is.
// A decode with no audio branch, or one whose branch has posted no measurement yet, has no entry at
// all: absence and silence are different facts, and a floor invented here would erase the
// difference.
func (a *App) AudioLevels() []wire.AudioLevel {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	out := make([]wire.AudioLevel, 0, len(a.receivers))
	for ref, receiver := range a.receivers {
		peak, rms, ok := receiver.Level()
		if !ok {
			continue
		}
		out = append(out, wire.AudioLevel{
			Stream: wire.StreamRef{StreamName: ref.Name, Transport: ref.Transport},
			PeakDB: peak,
			RMSDB:  rms,
		})
	}
	return out
}

// receiveEnded drops a pipeline that ended on its own and says why.
//
// The pipeline has stopped itself by the time this runs, so nothing is torn down here.
// The exit says why that one ended and the state beside it says which decodes are left, the pair a
// viewer that closed on its own produces.
func (a *App) receiveEnded(ref StreamRef, message string) {
	a.procMu.Lock()
	_, present := a.receivers[ref]
	delete(a.receivers, ref)
	a.procMu.Unlock()

	if !present {
		// A stop the user asked for takes the receiver out first and the teardown then ends the bus
		// watch, so an exit announced here would report a decode ending that the shell knows it closed.
		return
	}
	logger.Warnf("receiving '%s' over %s ended: %s", ref.Name, ref.Transport, message)
	a.emit(wire.ReceiveExitEvent(wire.StreamRef{StreamName: ref.Name, Transport: ref.Transport},
		message, a.membership().failure()))
	a.emitReceiveState()
}

// ReceiveState is what every running decode turned out to be, read off the pipelines rather than
// remembered.
//
// Nothing here is cached, the rule the publish stats follow: a chain falls back at build time and
// the memory features settle when the pads negotiate, so a state assembled from what a caller
// believed it started would report the chain that was asked for rather than the one that ran.
func (a *App) ReceiveState() []wire.ReceiveStream {
	a.procMu.Lock()
	defer a.procMu.Unlock()

	// One read for every decode: what rolls an HDR stream down is the machine's answer and the same
	// for all of them, and a tile reads it beside the stream it is deciding about.
	// A missing element is named only where the offer cannot be taken, and stays empty both on a
	// machine that tone-maps and on one registering every factory whose rung failed on a property.
	// CanToneMap is what tells those two apart.
	offer := receive.ToneMapping()

	out := make([]wire.ReceiveStream, 0, len(a.receivers))
	for ref, receiver := range a.receivers {
		stats := receiver.Stats()
		volume, muted, hasAudio := receiver.Audio()
		live := stats.Frames > 0
		out = append(out, wire.ReceiveStream{
			Stream:         wire.StreamRef{StreamName: ref.Name, Transport: ref.Transport},
			Live:           live,
			Failure:        a.receiveFailure(ref.Name, live),
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

// receiveFailure is why a tile carries no picture, and nil while a decode is merely opening.
//
// Two answers and an absence.
// Membership comes first: a machine the group stopped honouring loses every stream at once, and
// naming the publisher instead would send a reader after somebody who never stopped publishing.
// The relay snapshot answers the rest, a stream it stopped carrying being one nothing is going to
// arrive on.
func (a *App) receiveFailure(streamName string, live bool) *screensharev1.Text {
	assert.Assert(streamName != "", "a decode names the stream it receives")

	if live {
		return nil
	}
	if lapsed := a.membership().failure(); lapsed != nil {
		return lapsed
	}
	if carried, known := a.relayCarries(streamName); known && !carried {
		return text.Of(screensharev1.TextCode_TEXT_CODE_STREAM_LEFT_THE_RELAY)
	}
	return nil
}

// emitReceiveState announces the running decodes.
//
// Read back through ReceiveState rather than handed a set, so what is announced is what a read would
// answer rather than what a caller believed it had just done.
// It takes procMu, so a caller holding that lock defers this rather than calling it in place.
func (a *App) emitReceiveState() {
	a.emit(wire.ReceiveStateEvent(a.ReceiveState()))
}

// joinSource is the transport's source elements as one launch-line fragment.
//
// The transport yields them as arguments, which is what a spawned gst-launch-1.0 takes, and this
// side parses a line, so the two forms meet here rather than the transport declaring both.
// A space is all that is between them: the arguments are one element and its properties, and an
// entry that is a second element carries its own "!" the way the publish sinks do.
func joinSource(elements []string) string {
	assert.Assert(len(elements) > 0, "a watching transport yields source elements")
	return strings.Join(elements, " ")
}
