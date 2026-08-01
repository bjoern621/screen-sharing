package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// stopTimeout bounds the teardown of one pipeline, so a source that will not
// let go cannot hold the window's close.
const stopTimeout = time.Second

// receiver is one stream's pipeline behind the player.Player contract.
type receiver struct {
	name string
	// chain is the row the pipeline was built from, kept because what a stream
	// renders through is part of what the overlay reports and because the size
	// bound and the memory check are written from it.
	chain     chain
	pipeline  gst.Pipeline
	sink      gst.Element
	fit       gst.Element // the capsfilter that bounds what the chain's scaler produces
	paintable *gdk.Paintable
	cancel    context.CancelFunc
	started   time.Time
	live      atomic.Bool
	frames    atomic.Uint64
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
func New(st roster.Stream, open player.Open, ev player.Events) (player.Player, error) {
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
	sink := pipeline.GetByName(sinkName)
	if sink == nil {
		return nil, fmt.Errorf("stream %q: no gtk4paintablesink in the pipeline", st.Name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &receiver{
		name:      st.Name,
		chain:     c,
		pipeline:  pipeline,
		sink:      sink,
		fit:       pipeline.GetByName(fitName),
		paintable: paintableOf(sink),
		cancel:    cancel,
		started:   time.Now(),
	}
	assert.IsNotNil(r.paintable, "the sink hands out its paintable from construction", st.Name)
	// A chain writes a size bound exactly where it names a filter to write it into.
	// The two come from one row, so a mismatch is that row's elements and its fit caps
	// having drifted apart rather than anything a stream can cause.
	assert.Assert((r.fit != nil) == (c.fitCaps != ""),
		"a chain names the filter it bounds its size with", c.name, st.Name)

	r.watchFrames(ev.OnLive)
	r.watchDecodePads(ev.OnAudio)
	r.watchElements()

	go r.watchBus(ctx, ev.OnEnd)
	pipeline.SetState(gst.StatePlaying)
	// The chain is named here rather than only where one is chosen, because the
	// chain that plays is not always the one that was asked for: a machine that
	// cannot run it falls back, and this is the line that says what ran.
	logger.Infof("stream %q playing over %s through the %s render chain", st.Name, st.Transport, c.name)
	return r, nil
}

// watchFrames counts the frames that reach the surface and reports the first one.
// The first emission counted is the live signal, and the count feeds the stats overlay's measured
// fps and the session's stall sweep, so both are only true while an emission means a frame.
//
// gtk4paintablesink invalidates the paintable's contents for two different reasons: once for each
// frame it hands over, and once more when it drops its texture on the way to StateNull.
// Counted alike, a pipeline that died before decoding anything reported a first frame, which took
// its tile out of loading and refilled a reconnect budget that then never ran out.
//
// The paintable tells them apart on its own: it reports the size of the image it holds, and it
// clears that image before invalidating on a flush, so an emission over a zero size is the texture
// going away rather than a frame arriving.
// The sink's own rendered counter answers the same question, but it is written on the streaming
// thread while this handler runs on the UI loop, so an emission cannot be ordered against it: the
// first frame's invalidation can arrive before the count that would confirm it.
//
// The handler stays connected because disconnecting from inside the emission is not worth the
// bookkeeping.
func (r *receiver) watchFrames(onLive func()) {
	r.paintable.Connect("invalidate-contents", func() {
		if r.paintable.IntrinsicWidth() <= 0 || r.paintable.IntrinsicHeight() <= 0 {
			return
		}
		r.frames.Add(1)
		if r.live.CompareAndSwap(false, true) {
			logger.Debugf("stream %q rendered its first frame", r.name)
			// A frame on the surface is the proof that every pad from the decoder to
			// the sink negotiated, which is what makes the memory the chain asked for
			// comparable to the memory it got.
			r.verifyMemory()
			if onLive != nil {
				onLive()
			}
		}
	})
}

// watchDecodePads grows the audio branch when decodebin exposes an audio pad.
// Audio pads never match the video branch, so pad-added is the audio detection
// point; the branch is built there, while the pipeline plays.
func (r *receiver) watchDecodePads(onAudio func()) {
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
// Most of what the overlay reports comes from elements the launch line does not
// name: decodebin builds its decoders inside nested bins, rtspsrc its
// jitterbuffers. The signal catches those as they appear, the walk catches the
// elements parse-launch already built, the transport's source among them.
// Connecting before the walk means an element added in between is seen twice,
// which onElement absorbs.
func (r *receiver) watchElements() {
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
// The tile shows what the element reported, in its own wording.
// The debug string GStreamer sends with it names the element and the source line it failed on, which
// is a log line rather than something to put on a tile.
func (r *receiver) watchBus(ctx context.Context, onEnd func(message string)) {
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
		withOwnMainContext(func() { r.pipeline.SetState(gst.StateNull) })
		if onEnd != nil {
			onEnd(message)
		}
		return
	}
}

func (r *receiver) Paintable() *gdk.Paintable { return r.paintable }

func (r *receiver) SetVolume(v float64) {
	assert.Assert(v >= 0 && v <= 1, "volume is a fraction", v)
	r.setAudioProperty("volume", v)
}

func (r *receiver) SetMuted(muted bool) {
	r.setAudioProperty("mute", muted)
}

// SetRenderSize bounds the branch to the pixels the widget drawing the paintable shows.
// It implements player.RenderSizer.
//
// A tile is routinely far smaller than the stream it plays: the spotlight's film strip is 199 pixels
// wide against a 1920-wide desktop capture.
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
// A zero width or height is a widget GTK has not allocated yet and leaves the pipeline where it is.
func (r *receiver) SetRenderSize(width, height int) {
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
func (r *receiver) setAudioProperty(name string, value any) {
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
func (r *receiver) Stop() {
	r.cancel()
	withOwnMainContext(func() {
		r.pipeline.BlockSetState(gst.StateNull, gst.ClockTime(stopTimeout))
	})
	logger.Debugf("stream %q pipeline stopped", r.name)
}
