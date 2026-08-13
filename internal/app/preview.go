package app

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/receive"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The local preview: this machine decoding the stream it is sending, without the relay being party
// to it (docs/viewer-architecture.md, "What the broadcast preview draws").
//
// It belongs to the publish and to nothing else.
// The publish child is what produces the copy - a second sink on a loopback port,
// off the same encoder the relay leg is fed from - so the port is allocated as part of launching
// that child and the pipeline reading it goes up and down with it.
// There is no effect on the contract that opens a preview, and there is nothing for one to name:
// PublishState.live is singular, so "the preview" is a complete identity for as long as it exists
// and no identity at all otherwise.
//
// It is deliberately not a receiver in the map beside the relay decodes.
// Those are keyed by WatchKey, a stream and the protocol it crossed the relay over,
// and this stream crossed no relay: putting it there would need a transport name for a leg no
// transport carries, and every consumer of that table would then read a protocol that does not
// exist.
// What it gets instead is a field of its own and a place on the publish state.

// previewLeg is what the log calls the carriage between the child and this process.
// It is a description and not a registry name: no transport entry carries this leg,
// which is the whole reason the preview is modelled apart from the relay decodes.
const previewLeg = "loopback RTP"

// previewRun is the local preview in force: the port the child was told to copy to and the pipeline
// decoding what arrives there.
//
// It is replaced whole rather than mutated, for the reason publishRun is: the callbacks of a
// pipeline that has been superseded still fire, and one of them reporting against the preview that
// replaced it would say the running one had ended.
type previewRun struct {
	port     int
	receiver *receive.Receiver
}

// startPreviewLocked brings up the preview for a publish about to launch and returns the leg the
// child is told to copy to.
// procMu is held by the caller.
//
// Idempotent, and the guard is a read of what is running rather than a flag:
// a preview that is already up is already the state this asks for, and building a second one would
// bind a second port for a child that will only ever be told about one.
//
// Every failure here is an Umgebungsfehler that costs the preview and not the stream.
// A format with no local carriage, a port the kernel would not hand out, a pipeline that would not
// start - each of them leaves the publish to go ahead with no second sink,
// and the state then reports no preview, which is what the broadcast screen says rather than
// drawing a picture that would never arrive.
func (a *App) startPreviewLocked(s settings.Settings) publish.PreviewLeg {
	if a.preview != nil {
		return publish.PreviewLeg{Port: a.preview.port}
	}

	format, carried := publish.PreviewCarried(s.Publish.Codec)
	if !carried {
		logger.Warnf("no local preview for '%s': %s produces %s, which has no local carriage",
			s.Publish.Name, s.Publish.Codec, format)
		return publish.PreviewLeg{}
	}

	// The receiver binds the port before the child is told the number, so the child's first packet has
	// somewhere to land.
	// Nothing is lost if the order slips - a datagram sent at a port nothing holds is discarded and
	// the next one is not - but a receiver that came up second would be one whose first seconds are
	// missing for no reason anybody could see.
	port, err := publish.AllocatePreviewPort()
	if err != nil {
		logger.Warnf("no local preview for '%s': %v", s.Publish.Name, err)
		return publish.PreviewLeg{}
	}
	source, err := publish.PreviewSource(s.Publish.Codec, port)
	if err != nil {
		logger.Warnf("no local preview for '%s': %v", s.Publish.Name, err)
		return publish.PreviewLeg{}
	}

	run := &previewRun{port: port}
	receiver, err := receive.New(receive.Stream{
		Name:      s.Publish.Name,
		Transport: previewLeg,
		Source:    source,
	}, receive.Open{Chain: s.Viewer.RenderChain}, receive.Events{
		// A first frame changes what the state reports - the chain that ran, the memory the pads
		// negotiated - so it is announced like every other change.
		// The publish state is what carries it, because the preview is part of the publish.
		OnLive: a.emitPublishState,
		OnEnd: func(message string) {
			a.previewEnded(run, message)
		},
	})
	if err != nil {
		logger.Warnf("no local preview for '%s': %v", s.Publish.Name, err)
		return publish.PreviewLeg{}
	}
	run.receiver = receiver

	a.preview = run
	logger.Infof("previewing '%s' locally on 127.0.0.1:%d", s.Publish.Name, port)
	return publish.PreviewLeg{Port: port}
}

// stopPreviewLocked takes the preview down.
// procMu is held by the caller.
//
// A preview that is not running is not a failure, for the reason StopReceive takes a decode that is
// already closed: a stop names the state the caller wants and that state already holds.
//
// The receiver is stopped after it has been taken out of the field, so a teardown that blocks on
// the pipeline reaching NULL is one nothing else is waiting behind, and the exit it may report on
// the way finds no preview to end.
func (a *App) stopPreviewLocked() {
	run := a.preview
	if run == nil {
		return
	}
	a.preview = nil

	run.receiver.Stop()
	logger.Infof("stopped previewing on 127.0.0.1:%d", run.port)
}

// previewEnded drops a preview pipeline that ended on its own and says why.
//
// Nothing is torn down here and nothing is retried: the pipeline has already stopped itself,
// and what feeds it is a child this process is separately supervising.
// A preview that ends is a picture that goes away, which the publish state says by carrying no
// preview - the stream itself is untouched, and that is the whole point of the leg being a copy.
func (a *App) previewEnded(run *previewRun, message string) {
	assert.IsNotNil(run, "an exit belongs to the preview that produced it")

	a.procMu.Lock()
	if a.preview != run {
		// The exit reports a preview the app has already moved off: a stop was asked for,
		// or the publish behind it was replaced.
		// Either way the preview it names is gone and the one that is running is not this.
		a.procMu.Unlock()
		return
	}
	a.preview = nil
	a.procMu.Unlock()

	logger.Warnf("the local preview on 127.0.0.1:%d ended: %s", run.port, message)
	a.emitPublishState()
}

// previewSnapshotLocked is what the running preview turned out to be, read off the pipeline rather
// than remembered, and nil while none is running.
// procMu is held by the caller.
//
// Nothing here is cached, which is the rule ReceiveState follows for the same reason:
// a chain falls back at build time and the memory features settle when the pads negotiate,
// so a state assembled from what a caller believed it started would report the chain that was asked
// for rather than the one that ran.
func (a *App) previewSnapshotLocked() *wire.PreviewSnapshot {
	if a.preview == nil {
		return nil
	}

	stats := a.preview.receiver.Stats()
	return &wire.PreviewSnapshot{
		Port:         a.preview.port,
		Live:         stats.Frames > 0,
		Chain:        stats.Chain,
		DecodeMemory: stats.DecodeMemory,
		RenderMemory: stats.RenderMemory,
		Decoder:      stats.Decoder,
		Hardware:     stats.Hardware,
	}
}

// SubscribePreviewFrames opens one consumer's view of the local preview's frames.
//
// It opens no pipeline, for the reason SubscribeFrames opens no decode: what brings the preview up
// is the publish, and a subscription that started one would be the frame channel deciding that a
// stream should be published.
// Nothing publishing is therefore a refusal rather than a wait, and a shell reads the publish state
// to know whether to ask at all rather than asking to find out.
func (a *App) SubscribePreviewFrames() (*receive.Subscription, error) {
	a.procMu.Lock()
	run := a.preview
	a.procMu.Unlock()

	if run == nil {
		return nil, fmt.Errorf("nothing is publishing with a local preview")
	}
	return run.receiver.Subscribe(), nil
}
