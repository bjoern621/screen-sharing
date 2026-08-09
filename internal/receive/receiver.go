package receive

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstapp"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// stopTimeout bounds the teardown of one pipeline, so a source that will not let
// go cannot hold up the stream that replaces it or the backend's own shutdown.
const stopTimeout = time.Second

// Stream is one stream a receive pipeline is opened for.
//
// Source is the transport's own launch-line fragment, the elements up to the
// encoded stream (transport.GstSource). Everything after it is this package's
// (chains.go), which is what keeps the protocol's knowledge in the transport entry
// and the decode knowledge here.
type Stream struct {
	// Name is what the stream is known by on the relay, and what every line this
	// package logs is reported under.
	Name string
	// Transport is the watch leg the source fragment was built for. It is carried
	// so a log line can name it; nothing here branches on the protocol.
	Transport string
	Source    string
}

// Open is what a receiver is opened with besides the stream itself: the render
// choices that belong to the viewer rather than to the stream.
//
// It is a struct so a choice can be added without every caller learning about it,
// and its zero value is what a caller with no choice to make passes.
type Open struct {
	// Chain names the render chain the stream's source fragment is completed with,
	// one of the names Chains reports. Empty asks for the default, and so does a
	// name this machine cannot run.
	Chain string
}

// Events are one receiver's lifecycle callbacks. They fire on pipeline threads
// rather than on the caller's, so a callback that touches state of the caller's
// own does its own guarding.
type Events struct {
	// OnLive fires once, when the first decoded frame leaves the sink. Its meaning
	// is a frame out of the pipeline, not a transport coming up.
	OnLive func()
	// OnAudio fires once, when the stream turns out to carry audio and the audio
	// branch is playing.
	OnAudio func()
	// OnEnd fires once on a fatal pipeline error or end of stream, with a
	// human-readable message. The pipeline is already stopped: a dead receive
	// pipeline has nothing to recover.
	OnEnd func(message string)
}

// Receiver is one stream's running receive pipeline.
type Receiver struct {
	name string
	// chain is the row the pipeline was built from, kept because what a stream
	// renders through is part of what the receive state reports and because the
	// size bound and the memory check are written from it.
	chain    chain
	pipeline gst.Pipeline
	sink     gstapp.AppSink
	fit      gst.Element // the capsfilter that bounds what the chain's scaler produces
	cancel   context.CancelFunc
	started  time.Time
	live     atomic.Bool
	frames   atomic.Uint64
	// renderSize is the size SetRenderSize last wrote, width packed over height.
	// A caller that reports the size it reported before renegotiates nothing.
	renderSize atomic.Uint64
	video      decodeTrack
	audio      decodeTrack

	mu           sync.Mutex
	volume       gst.Element // the audio branch's volume element, nil without audio
	audioConvert gst.Element // the audio branch's audioconvert, for the raw audio caps
	// stats are the transport elements statSources matched, in discovery order.
	stats []elementStats
}

// New parses and starts the pipeline for one stream, on the render chain the
// caller asked for.
func New(st Stream, open Open, ev Events) (*Receiver, error) {
	assert.Assert(st.Name != "", "a stream carries the name it is known by")
	assert.Assert(st.Source != "", "a stream carries the source fragment to decode", st.Name)
	initGStreamer()

	c := resolve(open.Chain)
	desc := c.launch(st.Source)
	logger.Debugf("stream %q pipeline: %s", st.Name, desc)

	el, err := gst.ParseLaunch(desc)
	if err != nil {
		return nil, fmt.Errorf("stream %q: %w", st.Name, err)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		return nil, fmt.Errorf("stream %q: parse did not yield a pipeline", st.Name)
	}
	// One assertion for two failures: a pipeline that grew no sink at all yields a
	// nil element, which is not an appsink either.
	sink, ok := pipeline.GetByName(sinkName).(gstapp.AppSink)
	if !ok {
		return nil, fmt.Errorf("stream %q: no appsink named %s in the pipeline", st.Name, sinkName)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &Receiver{
		name:     st.Name,
		chain:    c,
		pipeline: pipeline,
		sink:     sink,
		fit:      pipeline.GetByName(fitName),
		cancel:   cancel,
		started:  time.Now(),
	}
	// A chain writes a size bound exactly where it names a filter to write it into.
	// The two come from one row, so a mismatch is that row's elements and its fit caps
	// having drifted apart rather than anything a stream can cause.
	assert.Assert((r.fit != nil) == (c.fitCaps != ""),
		"a chain names the filter it bounds its size with", c.name, st.Name)

	r.watchSamples(ev.OnLive)
	r.watchDecodePads(ev.OnAudio)
	r.watchElements()

	go r.watchBus(ctx, ev.OnEnd)
	pipeline.SetState(gst.StatePlaying)
	// The chain is named here rather than only where one is chosen, because the
	// chain that runs is not always the one that was asked for: a machine that
	// cannot run it falls back, and this is the line that says what ran.
	logger.Infof("stream %q receiving over %s through the %s render chain", st.Name, st.Transport, c.name)
	return r, nil
}

// watchSamples takes every frame off the sink and reports the first one. The count
// feeds the measured fps a reader derives and the stall sweep a session runs, so
// both are only true while a pull means a frame.
//
// The sink emits on its streaming thread and holds the buffer until the handler
// returns, which is the backpressure renderSink's one-buffer bound is written for:
// pulling here and returning is what lets the next frame through.
//
// The sample itself goes nowhere yet. Exporting it as a GPU handle is the frame
// channel's job and the frame channel does not exist (docs/viewer-architecture.md,
// "The frame channel"), so the pipeline decodes, the counters and the negotiated
// caps are real, and the frames are released here for as long as there is nowhere
// to send them.
//
// The handler stays connected because disconnecting from inside the emission is
// not worth the bookkeeping.
func (r *Receiver) watchSamples(onLive func()) {
	r.sink.ConnectNewSample(func(sink gstapp.AppSink) gst.FlowReturn {
		// A sink with nothing to hand over is one that was flushed on its way out of
		// PLAYING. The bus watch is what reports the end; this side stops counting.
		if sink.PullSample() == nil {
			return gst.FlowEOS
		}
		r.frames.Add(1)
		if r.live.CompareAndSwap(false, true) {
			logger.Debugf("stream %q decoded its first frame", r.name)
			// A frame out of the sink is the proof that every pad from the decoder to
			// the sink negotiated, which is what makes the memory the chain asked for
			// comparable to the memory it got.
			r.verifyMemory()
			if onLive != nil {
				onLive()
			}
		}
		return gst.FlowOK
	})
}

// watchDecodePads grows the audio branch when decodebin exposes an audio pad.
// Audio pads never match the video branch, so pad-added is the audio detection
// point; the branch is built there, while the pipeline plays.
func (r *Receiver) watchDecodePads(onAudio func()) {
	dec := r.pipeline.GetByName(decodeName)
	if dec == nil {
		logger.Warnf("stream %q has no %s element, audio stays off", r.name, decodeName)
		return
	}
	dec.Connect("pad-added", func(_ gst.Element, pad gst.Pad) {
		r.onDecodePad(pad, onAudio)
	})
}

// watchElements classifies the elements the pipeline holds and the ones it grows.
// Most of what a reader is shown comes from elements the launch line does not
// name: decodebin builds its decoders inside nested bins, rtspsrc its
// jitterbuffers. The signal catches those as they appear, the walk catches the
// elements parse-launch already built, the transport's source among them.
// Connecting before the walk means an element added in between is seen twice,
// which onElement absorbs.
func (r *Receiver) watchElements() {
	r.pipeline.Connect("deep-element-added", func(_ gst.Bin, _ gst.Bin, child gst.Element) {
		r.onElement(child)
	})
	for v := range r.pipeline.IterateRecurse().Values() {
		if e, ok := v.(gst.Element); ok {
			r.onElement(e)
		}
	}
}

// watchBus reports the first fatal bus message and stops the pipeline.
// A reader shows what the element reported, in its own wording.
// The debug string GStreamer sends with it names the element and the source line it failed on, which
// is a log line rather than something to put in front of a user.
func (r *Receiver) watchBus(ctx context.Context, onEnd func(message string)) {
	defer r.cancel()
	for msg := range r.pipeline.GetBus().Messages(ctx) {
		var message, debug string
		switch msg.Type() {
		case gst.MessageError:
			d, err := msg.ParseError()
			message, debug = err.Error(), d
		case gst.MessageEOS:
			message = "stream ended"
		default:
			continue
		}
		logger.Warnf("stream %q ended: %s", r.name, message)
		if debug != "" {
			logger.Debugf("stream %q pipeline error: %s", r.name, debug)
		}
		r.pipeline.SetState(gst.StateNull)
		if onEnd != nil {
			onEnd(message)
		}
		return
	}
}

// SetVolume sets the audio branch's volume, 0 to 1. A no-op until OnAudio fired.
func (r *Receiver) SetVolume(v float64) {
	assert.Assert(v >= 0 && v <= 1, "volume is a fraction", v)
	r.setAudioProperty("volume", v)
}

// SetMuted mutes or unmutes the audio branch. A no-op until OnAudio fired.
func (r *Receiver) SetMuted(muted bool) {
	r.setAudioProperty("mute", muted)
}

// SetRenderSize bounds the branch to the pixels the tile drawing the stream shows.
//
// A tile is routinely far smaller than the stream it plays: a film strip 199 pixels wide against a
// 1920-wide desktop capture.
// Unbounded, every frame is converted at the source's resolution and the GPU throws most of those
// pixels away again at draw time.
//
// The bound is a maximum, not a size.
// The chain's scaler fixates inside the range and corrects the pixel aspect ratio to hold the display
// aspect ratio, so the picture needs no borders and a tile larger than its stream negotiates the
// stream's own size rather than an upscale nobody asked for.
//
// The caps come from the chain, feature and all, because caps that name no memory feature pin the
// frames into system memory: on a device chain, writing a bare video/x-raw here is a download of
// every frame from the moment a tile first reports its size.
// A chain that names no filter to write them into renders at the size the source sends.
//
// Writing the filter renegotiates the branch, so only a size that changed is written.
// A zero width or height is a tile whose size the shell has not measured yet and leaves the pipeline
// where it is.
func (r *Receiver) SetRenderSize(width, height int) {
	if width <= 0 || height <= 0 || r.fit == nil {
		return
	}
	size := uint64(uint32(width))<<32 | uint64(uint32(height))
	if r.renderSize.Swap(size) == size {
		return
	}
	caps := r.chain.fit(width, height)
	assert.Assert(caps != "", "a chain that names a fit filter bounds it with caps", r.chain.name, r.name)
	r.fit.SetObjectProperty("caps", gst.CapsFromString(caps))
	logger.Debugf("stream %q renders at most %dx%d, bounded by %s", r.name, width, height, caps)
}

// setAudioProperty writes one property of the audio branch's volume element, and
// does nothing while the stream has no audio branch.
func (r *Receiver) setAudioProperty(name string, value any) {
	r.mu.Lock()
	vol := r.volume
	r.mu.Unlock()
	if vol == nil {
		return
	}
	vol.SetObjectProperty(name, value)
}

// Stop tears the pipeline down, from whichever thread the caller is on. The bus
// watch is cancelled first, so the state change it would report as an error goes
// nowhere.
//
// It needs no main-context guard, which the native grid's teardown did: there the
// sink was gtk4paintablesink, whose way out of PAUSED flushes a GTK object built on
// the UI loop and panics when another thread touches it. An appsink owns nothing
// outside the pipeline, so the way down belongs to the thread that asks for it.
func (r *Receiver) Stop() {
	r.cancel()
	r.pipeline.BlockSetState(gst.StateNull, gst.ClockTime(stopTimeout))
	logger.Debugf("stream %q pipeline stopped", r.name)
}
