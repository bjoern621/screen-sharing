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

// stopTimeout bounds one attempt at taking a pipeline to NULL, and stopAttempts is
// how many of those an unwilling pipeline is given.
//
// The bound exists so a source that will not let go cannot hold up the stream that
// replaces it or the backend's own shutdown. What it is not is a way out: a pipeline
// still running when this process exits is torn down by the operating system instead
// of by GStreamer, and on Windows that means threads killed wherever they stand,
// including inside the display driver. A process left with a thread wedged in a
// driver call never finishes exiting, and everything it owns - the control pipe among
// it - stays owned by a process nothing can kill. So the bound is generous enough
// that a decoder shutting down under load reaches NULL inside it, and a pipeline that
// misses it twice is reported rather than silently walked past.
//
// Two attempts of the same length rather than one of twice the length: the ordinary
// slow case is a decoder draining, which the first wait covers, and the second is
// what separates "slower than expected" from "not coming back" in the log.
const (
	stopTimeout  = 3 * time.Second
	stopAttempts = 2
)

// renderSize is a width and a height carried as one word, width in the high half.
//
// It is a type with a pack and an unpack rather than the shift spelled at each site,
// because the packing is one fact and three sites knowing it are three chances to unpack
// the halves the other way round - which the compiler cannot catch, since every packed
// size is the same uint64 as every other. One word rather than two fields is what lets a
// size be swapped and compared in one atomic operation.
type renderSize uint64

func packSize(width, height int) renderSize {
	return renderSize(uint64(uint32(width))<<32 | uint64(uint32(height)))
}

func (s renderSize) unpack() (width, height int) {
	return int(uint32(s >> 32)), int(uint32(s))
}

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

	// peakDB and rmsDB are the last measurement the level element posted, as bit
	// patterns, and hasLevel whether one has been posted at all. Atomic rather than
	// under mu because they are written from the bus thread at the metering rate and
	// read by every level tick, and neither reader needs them consistent with anything
	// else the receiver holds.
	peakDB   atomic.Uint64
	rmsDB    atomic.Uint64
	hasLevel atomic.Bool

	mu           sync.Mutex
	volume       gst.Element // the audio branch's volume element, nil without audio
	audioConvert gst.Element // the audio branch's audioconvert, for the raw audio caps
	// wantVolume and wantMuted are the loudness SetAudio last asked for, held whether
	// or not the branch exists. A decode that has not exposed an audio pad yet is the
	// ordinary case for a volume arriving early, and dropping it there would make the
	// effect's result depend on when it was sent.
	wantVolume float64
	wantMuted  bool
	// stats are the transport elements statSources matched, in discovery order.
	stats []elementStats
	// subs are the consumers the frames are handed to, each with its own pool
	// (export.go). Zero of them is the ordinary state of a decode nothing is drawing
	// yet, and the pipeline runs the same either way.
	subs []*Subscription
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
		// A decode plays at the loudness it arrived with until something asks for
		// another. Unchanged is the only default that does not silently alter a stream
		// nobody has said anything about.
		wantVolume: 1,
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
// The sample is then handed to every consumer that has subscribed (export.go), each
// of which copies it into a slot of its own pool on the GPU and names that slot to
// the shell. None of them blocks: a consumer with no free slot drops the frame, so a
// window that is busy costs frames and never costs the pipeline. A decode nothing is
// drawing has no consumers and the sample is simply released here, which is the
// ordinary state of a stream that is being received and not yet watched.
//
// The handler stays connected because disconnecting from inside the emission is
// not worth the bookkeeping.
func (r *Receiver) watchSamples(onLive func()) {
	r.sink.ConnectNewSample(func(sink gstapp.AppSink) gst.FlowReturn {
		// A sink with nothing to hand over is one that was flushed on its way out of
		// PLAYING. The bus watch is what reports the end; this side stops counting.
		sample := sink.PullSample()
		if sample == nil {
			return gst.FlowEOS
		}
		// A pulled sample is owned here, and the binding otherwise drops it from a
		// finalizer whenever the collector next runs. That is not a lifetime a decoder
		// can be held to: every sample pins one of its textures, and a pool the
		// collector has not got around to freeing is a decoder that cannot allocate the
		// next frame. Released here, the sample lives exactly as long as the handover.
		defer gst.UnsafeSampleUnref(sample)
		r.frames.Add(1)
		r.fanOut(sample)
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
		case gst.MessageElement:
			// The level element posts here, at the metering rate. It is the one message
			// on this bus that is not about the pipeline ending, which is why it is read
			// on the watch that was already running rather than on a second one.
			if msg.HasName(levelMessage) {
				r.onLevelMessage(msg)
			}
			continue
		default:
			continue
		}
		logger.Warnf("stream %q ended: %s", r.name, message)
		if debug != "" {
			logger.Debugf("stream %q pipeline error: %s", r.name, debug)
		}
		r.pipeline.SetState(gst.StateNull)
		// The consumers are told on the calls they are blocked reading. The same fact
		// reaches every shell as ReceiveExit on the event stream, and neither is a
		// substitute for the other: a consumer waiting on frames learns nothing from
		// an event it is not the one reading.
		r.endSubs(message)
		if onEnd != nil {
			onEnd(message)
		}
		return
	}
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
	size := uint64(packSize(width, height))
	if r.renderSize.Swap(size) == size {
		return
	}
	caps := r.chain.fit(width, height)
	assert.Assert(caps != "", "a chain that names a fit filter bounds it with caps", r.chain.name, r.name)
	r.fit.SetObjectProperty("caps", gst.CapsFromString(caps))
	logger.Debugf("stream %q renders at most %dx%d, bounded by %s", r.name, width, height, caps)
}

// Stop tears the pipeline down, from whichever thread the caller is on. The bus
// watch is cancelled first, so the state change it would report as an error goes
// nowhere.
//
// It needs no main-context guard, which the native grid's teardown did: there the
// sink was gtk4paintablesink, whose way out of PAUSED flushes a GTK object built on
// the UI loop and panics when another thread touches it. An appsink owns nothing
// outside the pipeline, so the way down belongs to the thread that asks for it.
// Whether the pipeline actually reached NULL is the return value, because the caller
// that matters is the one shutting the process down: it is the only place that can
// decide what to do about a decoder that is still running, and it cannot decide it
// from a call that says nothing.
func (r *Receiver) Stop() bool {
	r.cancel()
	// The consumers go before the pipeline does. Each frees its pool as it ends, and a
	// pool freed after the device it was allocated on is torn down is a free against a
	// device that is gone.
	r.endSubs("the stream was closed")

	started := time.Now()
	for attempt := 1; attempt <= stopAttempts; attempt++ {
		// The result is read rather than discarded. A state change that runs out of time
		// comes back as ASYNC and leaves the pipeline running, which looks from here
		// exactly like the SUCCESS it is not: the threads are still in the decoder, and
		// the next thing this process does is exit.
		switch ret := r.pipeline.BlockSetState(gst.StateNull, gst.ClockTime(stopTimeout)); ret {
		case gst.StateChangeSuccess:
			logger.Debugf("stream %q pipeline stopped after %s", r.name, since(started))
			return true
		case gst.StateChangeFailure:
			// The pipeline refused the change outright rather than taking too long, so
			// waiting again is waiting for a decision that has already been made.
			logger.Warnf("stream %q pipeline refused to stop after %s", r.name, since(started))
			return false
		default:
			logger.Warnf("stream %q pipeline has not stopped after %s, waiting again", r.name, since(started))
		}
	}

	logger.Warnf("stream %q pipeline is still running after %s and is being left that way; "+
		"a process that exits with a decoder still in the display driver can be impossible to kill afterwards",
		r.name, since(started))
	return false
}

// since is an elapsed time in the form these lines are read in. The figures they
// carry are seconds of teardown, and a reader deciding whether a decoder is slow or
// stuck has no use for the nanoseconds under them.
func since(started time.Time) time.Duration {
	return time.Since(started).Round(time.Millisecond)
}
