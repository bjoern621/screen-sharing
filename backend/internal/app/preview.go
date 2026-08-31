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

// The local preview: this machine decoding the stream it sends, the relay no party to it
// (docs/viewer-architecture.md, "What the broadcast preview draws").
//
// It belongs to the publish and to nothing else.
// The publish child produces the copy,
// a second sink on a loopback port off the encoder that feeds the relay leg,
// so the port is allocated as part of launching that child,
// and the pipeline reading it goes up and down with it.
// No effect on the contract opens one, and none could name it:
// PublishState.live is singular,
// so "the preview" is a complete identity while one exists and no identity otherwise.
//
// A field of its own rather than a receiver beside the relay decodes,
// which are keyed by StreamRef, a stream and the protocol it crossed the relay over.
// This stream crossed no relay,
// so an entry there would need a transport name for a leg no transport carries,
// and every consumer of that table would read a protocol that does not exist.

// previewLeg is what the log calls the carriage between the child and this process.
// A description, not a transport registry name: no transport entry carries this leg.
const previewLeg = "loopback RTP"

// previewRun is the local preview in force:
// the port the child was told to copy to, and the pipeline decoding what arrives there.
//
// Replaced whole rather than mutated, as publishRun is:
// a superseded pipeline's callbacks still fire,
// and one reporting against the preview that replaced it would say the running one had ended.
type previewRun struct {
	port     int
	receiver *receive.Receiver
}

// startPreviewLocked brings the preview up for a launching publish,
// and returns the leg the child is told to copy to.
// procMu is held by the caller.
//
// Idempotent, guarded by a read of what is running rather than by a flag:
// a preview already up is the state asked for,
// and a second would bind a second port for a child told about one.
//
// Every failure here is an Umgebungsfehler costing the preview and not the stream:
// a format with no local carriage, a port the kernel would not hand out,
// a pipeline that would not start.
// The publish goes ahead with no second sink and the state reports no preview,
// which the broadcast screen says rather than drawing a picture that would never arrive.
func (a *App) startPreviewLocked(s settings.Settings) publish.PreviewLeg {
	if a.preview != nil {
		return publish.PreviewLeg{Port: a.preview.port}
	}

	format, carried := publish.PreviewCarried(s.Publish.Codec())
	if !carried {
		logger.Warnf("no local preview for '%s': %s produces %s, which has no local carriage",
			s.Publish.Name, s.Publish.Codec(), format)
		return publish.PreviewLeg{}
	}

	// The receiver binds the port before the child is told the number,
	// so the child's first packet has somewhere to land.
	// A slipped order loses nothing permanent,
	// a datagram sent at a port nothing holds is dropped and the next one is not,
	// but the preview's first seconds go missing for no visible reason.
	port, err := publish.AllocatePreviewPort()
	if err != nil {
		logger.Warnf("no local preview for '%s': %v", s.Publish.Name, err)
		return publish.PreviewLeg{}
	}
	source, err := publish.PreviewSource(s.Publish.Codec(), port)
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
		// The first frame settles what the state reports: the chain that ran,
		// the memory the pads negotiated.
		// It rides the publish state, the preview being part of the publish.
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

// stopPreviewLocked takes the preview down, and succeeds where none is running:
// the stop names a state that already holds, as StopReceive does on a closed decode.
// procMu is held by the caller.
//
// The field is cleared before the receiver is stopped,
// so an exit reported during the teardown finds no preview to end.
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
// Nothing is torn down and nothing is retried:
// the pipeline stopped itself, and what feeds it is a child this process supervises separately.
// The publish state then carries no preview and the stream is untouched, the leg being a copy.
func (a *App) previewEnded(run *previewRun, message string) {
	assert.IsNotNil(run, "an exit belongs to the preview that produced it")

	a.procMu.Lock()
	if a.preview != run {
		// The exit names a preview the app has already moved off:
		// a stop was asked for, or the publish behind it was replaced.
		a.procMu.Unlock()
		return
	}
	a.preview = nil
	a.procMu.Unlock()

	logger.Warnf("the local preview on 127.0.0.1:%d ended: %s", run.port, message)
	a.emitPublishState()
}

// previewSnapshotLocked is what the running preview turned out to be, read off the pipeline,
// and nil while none runs.
// procMu is held by the caller.
//
// Nothing is cached, the rule ReceiveState follows:
// a chain falls back at build time and the memory features settle when the pads negotiate,
// so a state assembled from what a caller believed it started would report the chain asked for,
// not the one that ran.
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
// It opens no pipeline, as SubscribeFrames opens no decode:
// the publish brings the preview up,
// and a subscription starting one would be the frame channel deciding a stream should be published.
// Nothing publishing is a refusal rather than a wait,
// and the publish state is what a shell reads to know whether to ask.
func (a *App) SubscribePreviewFrames() (*receive.Subscription, error) {
	a.procMu.Lock()
	run := a.preview
	a.procMu.Unlock()

	if run == nil {
		return nil, fmt.Errorf("nothing is publishing with a local preview")
	}
	return run.receiver.Subscribe(), nil
}
