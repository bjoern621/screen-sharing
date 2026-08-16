package receive

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-glib/pkg/gobject/v2"
	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstapp"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/pipedelay"
)

// stopTimeout bounds one attempt at taking a pipeline to NULL, and stopAttempts is how many an
// unwilling pipeline is given.
//
// The bound keeps a source that will not let go from holding up the stream that replaces it or
// the backend's own shutdown.
// It is not a way out: a pipeline still running at process exit is torn down by the operating
// system rather than by GStreamer, and on Windows that means threads killed wherever they stand,
// including inside the display driver.
// A process left with a thread wedged in a driver call never finishes exiting, and everything it
// owns, the control pipe among it, stays owned by a process nothing can kill.
// So the bound is generous enough for a decoder shutting down under load to reach NULL inside it,
// and a pipeline that misses it twice is reported rather than walked past.
//
// Two attempts of the same length rather than one of twice the length: the first covers the
// ordinary slow case of a decoder draining, and the second separates "slower than expected" from
// "not coming back" in the log.
const (
	stopTimeout  = 3 * time.Second
	stopAttempts = 2
)

// renderSize is a width and a height in one word, width in the high half.
//
// A type with a pack and an unpack rather than the shift spelled per site: every packed size is
// the same uint64 as every other, so a site that unpacks the halves the other way round is a
// mistake the compiler cannot catch.
// One word rather than two fields is what lets a size be swapped and compared in one atomic
// operation.
type renderSize uint64

func packSize(width, height int) renderSize {
	return renderSize(uint64(uint32(width))<<32 | uint64(uint32(height)))
}

func (s renderSize) unpack() (width, height int) {
	return int(uint32(s >> 32)), int(uint32(s))
}

// Stream is one stream a receive pipeline is opened for.
//
// Source is the transport's own launch-line fragment, the elements up to the encoded stream
// (transport.GstSource); everything after it is this package's (chains.go).
// The protocol's knowledge stays in the transport entry and the decode knowledge here.
type Stream struct {
	// Name is what the relay knows the stream by, and what this package's log lines
	// report it under.
	Name string
	// Transport is the watch leg the source fragment was built for, carried so a log
	// line can name it. Nothing here branches on the protocol.
	Transport string
	Source    string
	// Raw is whether Source hands over pictures rather than a bitstream, which takes the
	// decoder out of the line and the audio branch with it.
	// A monitor read off this machine's screen is the case that exists: nothing encoded
	// those frames and nothing carried them, so there is no format to autoplug a decoder
	// for and no second track to expose.
	Raw bool
}

// Open carries the render choices that belong to the viewer rather than to the stream.
//
// A struct so a choice can be added without every caller learning about it, and its zero value is
// what a caller with no choice to make passes.
type Open struct {
	// Chain names the render chain the source fragment is completed with, one of the
	// names Chains reports.
	// Empty asks for the default, and so does a name this machine cannot run.
	Chain string
	// ToneMap asks for an HDR stream to be rolled down into the range a standard display
	// shows (tonemap.go).
	// A machine with no rung builds the decode without one and reports that it did, the
	// same fallback Chain makes.
	//
	// Asked here rather than turned on by what the stream turns out to carry, because a
	// pipeline is built before anything has negotiated: what a decode is asked for is
	// knowable at that moment and what it will receive is not.
	// An SDR stream through the rung is a filter with nothing to convert.
	ToneMap bool
}

// Events are one receiver's lifecycle callbacks.
// They fire on pipeline threads rather than on the caller's, so a callback touching state of the
// caller's own guards it itself.
type Events struct {
	// OnLive fires once, on the first decoded frame out of the sink. Not a transport
	// coming up.
	OnLive func()
	// OnAudio fires once, when the stream turns out to carry audio and the branch is
	// playing.
	OnAudio func()
	// OnEnd fires once on a fatal pipeline error or end of stream, with a message for a
	// reader.
	// The pipeline is already stopped: a dead receive pipeline has nothing to recover.
	OnEnd func(message string)
}

// Receiver is the running receive pipeline of one stream.
type Receiver struct {
	name string
	// chain is the row the pipeline was built from: the receive state reports what a
	// stream renders through, and the size bound and the memory check are written from
	// it.
	chain chain
	// toneMap is whether the pipeline was built with the rung that rolls an HDR stream
	// down, which is not always what was asked for: a machine with no rung builds without
	// one.
	// The receive state reports it, and a caller compares a new request against it.
	toneMap  bool
	pipeline gst.Pipeline
	sink     gstapp.AppSink
	// delay measures the work between the leg's source stamping a frame and the sink
	// taking it, at the sink's pad rather than where the samples arrive: the sink holds
	// each frame until its presentation time, so a reading taken in the sample handler
	// reports the configured latency and never the work.
	delay *pipedelay.Probe
	fit   gst.Element // capsfilter bounding what the chain's scaler produces
	// stopMu serialises the teardown, which two sides can reach at once: a stop, and the bus watch on
	// a pipeline that ended by itself.
	// Apart from mu, which guards the fields a teardown clears and is taken inside it.
	stopMu  sync.Mutex
	cancel  context.CancelFunc
	started time.Time
	live    atomic.Bool
	frames  atomic.Uint64
	// renderSize is the size SetRenderSize last wrote, width packed over height.
	// A caller repeating a size it already reported renegotiates nothing.
	renderSize atomic.Uint64
	video      decodeTrack
	audio      decodeTrack

	// peakDB and rmsDB are the level element's last measurement as bit patterns, and
	// hasLevel whether one has been posted at all.
	// Atomic rather than under mu: the bus thread writes them at the metering rate, every
	// level tick reads them, and neither reader needs them consistent with anything else
	// the receiver holds.
	peakDB   atomic.Uint64
	rmsDB    atomic.Uint64
	hasLevel atomic.Bool

	mu           sync.Mutex
	volume       gst.Element // audio branch's volume element, nil without audio
	audioConvert gst.Element // audio branch's audioconvert, read for the raw audio caps
	// wantVolume and wantMuted are what SetAudio last asked for, held whether or not the
	// branch exists.
	// A decode that has not exposed an audio pad yet is the ordinary case for a volume
	// arriving early, and dropping it there would make the effect's result depend on when
	// it was sent.
	wantVolume float64
	wantMuted  bool
	// stats are the transport's elements statSources matched, in the order they turned up.
	stats []elementStats
	// subs are the consumers frames are handed to, each with a pool of its own
	// (export.go).
	// None is the ordinary state of a decode nothing is drawing, and the pipeline runs
	// the same either way.
	subs []*Subscription
}

// New parses and starts one stream's pipeline, on the render chain resolve settles on.
func New(st Stream, open Open, ev Events) (*Receiver, error) {
	assert.Assert(st.Name != "", "a stream carries the name it is known by")
	assert.Assert(st.Source != "", "a stream carries the source fragment to decode", st.Name)
	initGStreamer()

	c := resolve(open.Chain)
	rung := toneMapFor(st.Name, open.ToneMap)
	open.ToneMap = rung.declared()
	desc := c.launch(st, rung)
	logger.Debugf("stream %q pipeline: %s", st.Name, desc)

	el, err := gst.ParseLaunch(desc)
	if err != nil {
		return nil, fmt.Errorf("stream %q: %w", st.Name, err)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		return nil, fmt.Errorf("stream %q: parse did not yield a pipeline", st.Name)
	}
	// One type assertion for two failures: a pipeline that grew no sink at all yields a
	// nil element, which is not an appsink either.
	sink, ok := pipeline.GetByName(sinkName).(gstapp.AppSink)
	if !ok {
		return nil, fmt.Errorf("stream %q: no appsink named %s in the pipeline", st.Name, sinkName)
	}
	// A shader holds the newlines its preprocessor directives need and quotes the launch
	// parser reads as syntax, so a rung's own conversion is set as a property rather than
	// written into the line.
	// The element it goes into is one this package's fragment named, so its absence is
	// this package's own bug.
	if rung.shader != "" {
		shader := pipeline.GetByName(toneMapName)
		assert.Assert(shader != nil, "a rung that carries a shader builds the element it is written into", rung.name, toneMapName)
		shader.SetObjectProperty("fragment", rung.shader)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &Receiver{
		name:     st.Name,
		chain:    c,
		toneMap:  open.ToneMap,
		pipeline: pipeline,
		sink:     sink,
		delay:    pipedelay.Watch(sink, "sink"),
		fit:      pipeline.GetByName(fitName),
		cancel:   cancel,
		started:  time.Now(),
		// A decode plays at the loudness it arrived with until something asks for
		// another. Any other default alters a stream nobody has said anything about.
		wantVolume: 1,
	}
	// A chain's elements and its fit caps come from one row, so a mismatch is that row's
	// two halves having drifted apart rather than anything a stream can cause.
	assert.Assert((r.fit != nil) == (c.fitCaps != ""),
		"a chain names the filter it bounds its size with", c.name, st.Name)

	r.watchSamples(ev.OnLive)
	if !st.Raw {
		r.watchDecodePads(ev.OnAudio)
		r.watchSourcePads(ev.OnAudio)
	}
	r.watchElements()

	go r.watchBus(ctx, ev.OnEnd)
	pipeline.SetState(gst.StatePlaying)
	// The chain that runs is not always the one asked for, since a machine that cannot
	// run it falls back, so this line names what ran.
	logger.Infof("stream %q receiving over %s through the %s render chain", st.Name, st.Transport, c.name)
	return r, nil
}

// watchSamples takes every frame off the sink and reports the first.
// The count feeds the measured fps a reader derives and the stall sweep a session runs, so both
// hold only while a pull means a frame.
//
// The sink emits on its streaming thread and holds the buffer until the handler returns, which is
// the backpressure renderSink's one-buffer bound is written for: pulling here and returning is
// what lets the next frame through.
//
// The sample then goes to every consumer that has subscribed (export.go), each copying it into a
// slot of its own pool on the GPU and naming that slot to the shell.
// None of them blocks: a consumer with no free slot drops the frame, so a window that is busy
// costs frames and never costs the pipeline.
// A decode nothing is drawing has no consumers and the sample is released here, which is the
// ordinary state of a stream received and not yet watched.
//
// The handler stays connected: disconnecting from inside the emission is not worth the
// bookkeeping.
func (r *Receiver) watchSamples(onLive func()) {
	r.sink.ConnectNewSample(func(sink gstapp.AppSink) gst.FlowReturn {
		// A sink with nothing to hand over was flushed on its way out of PLAYING.
		// The bus watch reports the end; this side stops counting.
		sample := sink.PullSample()
		if sample == nil {
			return gst.FlowEOS
		}
		// A pulled sample is owned here, and the binding otherwise drops it from a
		// finalizer whenever the collector next runs.
		// Every sample pins one of the decoder's textures, so a pool the collector has
		// not got around to freeing is a decoder that cannot allocate the next frame.
		// Unreffed here, the sample lives exactly as long as the handover.
		defer gst.UnsafeSampleUnref(sample)
		r.frames.Add(1)
		r.fanOut(sample)
		if r.live.CompareAndSwap(false, true) {
			logger.Debugf("stream %q decoded its first frame", r.name)
			// A frame out of the sink proves every pad from the decoder to the sink
			// negotiated, which is what makes the memory the chain asked for
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
// An audio pad never matches the video branch, so pad-added is the detection point and the branch
// is built there, while the pipeline plays.
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

// watchSourcePads decodes the tracks the launch line has no room for.
//
// A source that carries each track in a stream of its own hands out a pad per track, and the line
// holds one decoder: rtspsrc offers RTP video and RTP audio, the delayed link places the picture,
// and the audio pad is left with nowhere to go.
// A source that hands over one muxed pad, which is every other transport here, exposes nothing
// for this to do.
//
// The source is the element nothing feeds, which is what makes it reachable without the transport
// naming it: a fragment states what it opens and where from, not what its element is called.
func (r *Receiver) watchSourcePads(onAudio func()) {
	sources := 0
	for v := range r.pipeline.IterateSources().Values() {
		src, ok := v.(gst.Element)
		if !ok {
			continue
		}
		sources++
		src.Connect("pad-added", func(_ gst.Element, pad gst.Pad) {
			r.onSourcePad(pad, onAudio)
		})
	}

	assert.Assert(sources == 1, "a receive pipeline decodes from one source", r.name, sources)
}

// watchElements classifies the elements the pipeline holds and the ones it grows.
//
// Most of what a reader is shown comes from elements the launch line does not name: decodebin
// builds its decoders inside nested bins, rtspsrc its jitterbuffers.
// The signal catches those as they appear and the walk catches what parse-launch already built,
// the transport's source among them.
// Connecting before the walk means an element added in between is seen twice, which onElement
// absorbs.
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

// watchBus reports the first fatal bus message and stops the pipeline, in the element's own
// wording.
// The debug string GStreamer sends with it names the element and the source line it failed on,
// which is a log line rather than something to put in front of a reader.
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
			// The level element posts here, at the metering rate.
			// It is the one message on this bus that is not about the pipeline ending,
			// so it is read on the watch already running rather than on a second one.
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
		// The consumers are told on the call they are blocked reading.
		// ReceiveExit carries the same fact to every shell on the event stream, and
		// neither substitutes for the other: a consumer waiting on frames learns nothing
		// from a stream it is not the one reading.
		r.endSubs(message)
		if onEnd != nil {
			onEnd(message)
		}
		// After the report rather than before it, the teardown being the slow half and the news being
		// what a reader is waiting on.
		// Here and not left to a stop that may never come: a pipeline that ended on its own is one
		// whose receiver the caller drops where it stands (app.previewEnded), so this is the last frame
		// holding it.
		r.teardown()
		return
	}
}

// ToneMap answers what the pipeline was built with rather than what was asked for, which is what
// makes it the value a caller compares a new request against.
// A machine with no rung builds without one, and a caller comparing against its own ask would
// rebuild the same pipeline forever.
func (r *Receiver) ToneMap() bool { return r.toneMap }

// SetRenderSize bounds the branch to the pixels the tile drawing the stream shows.
//
// A tile is routinely far smaller than the stream it plays: a film strip 199 pixels wide against
// a 1920-wide desktop capture.
// Unbounded, every frame is converted at the source's resolution and the GPU throws most of those
// pixels away again at draw time.
//
// The bound is a maximum and not a size.
// The chain's scaler fixates inside the range and corrects the pixel aspect ratio to hold the
// display aspect ratio, so the picture needs no borders and a tile larger than its stream
// negotiates the stream's own size rather than an upscale nobody asked for.
//
// The caps come from the chain, feature and all: caps naming no memory feature pin the frames
// into system memory, so a bare video/x-raw written here downloads every frame of a device chain
// from the moment a tile first reports its size.
// A chain that names no filter to write them into renders at the size the source sends.
//
// Writing the filter renegotiates the branch, so only a size that changed is written.
// A zero width or height is a tile the shell has not measured yet, and leaves the pipeline where
// it is.
func (r *Receiver) SetRenderSize(width, height int) {
	r.mu.Lock()
	fit := r.fit
	r.mu.Unlock()
	if width <= 0 || height <= 0 || fit == nil {
		return
	}
	size := uint64(packSize(width, height))
	if r.renderSize.Swap(size) == size {
		return
	}
	caps := r.chain.fit(width, height)
	assert.Assert(caps != "", "a chain that names a fit filter bounds it with caps", r.chain.name, r.name)
	fit.SetObjectProperty("caps", gst.CapsFromString(caps))
	logger.Debugf("stream %q renders at most %dx%d, bounded by %s", r.name, width, height, caps)
}

// Stop tears the pipeline down, from whichever thread the caller is on.
// The bus watch is cancelled first, so the state change it would report as an error goes nowhere.
//
// No main-context guard is needed: an appsink owns nothing outside the pipeline, so the way down
// belongs to the thread that asks for it.
// Whether the pipeline reached NULL is the return value, because the caller that matters is the
// one shutting the process down: it is the only place that can decide what to do about a decoder
// still running, and it cannot decide it from a call that says nothing.
func (r *Receiver) Stop() bool {
	r.cancel()
	// The consumers go before the pipeline does: each frees its pool as it ends, and a
	// pool freed after the device it was allocated on is torn down is a free against a
	// device that is gone.
	r.endSubs("the stream was closed")

	return r.teardown()
}

// teardown takes the pipeline to NULL and hands it back, once, whichever side asks first: a stop, or
// the bus watch on a pipeline that ended on its own.
//
// The whole of it runs under one lock, so the pipeline cannot be handed back between the read that
// takes it and the state change that uses it, which would be a state change against memory that has
// been freed.
// A receiver that has already handed its pipeline back is the state a stop names, so it succeeds and
// does nothing, as StopReceive does on a decode nobody opened.
func (r *Receiver) teardown() bool {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()

	pipeline := r.heldPipeline()
	if pipeline == nil {
		return true
	}

	started := time.Now()
	for attempt := 1; attempt <= stopAttempts; attempt++ {
		// A state change that runs out of time comes back ASYNC and leaves the pipeline
		// running, which is the case a discarded result would read as SUCCESS: the
		// threads are still in the decoder, and the next thing this process does is
		// exit.
		switch ret := pipeline.BlockSetState(gst.StateNull, gst.ClockTime(stopTimeout)); ret {
		case gst.StateChangeSuccess:
			r.release()
			logger.Debugf("stream %q pipeline stopped after %s", r.name, since(started))
			return true
		case gst.StateChangeFailure:
			// A refusal is a decision already made rather than a change taking too
			// long, so waiting again waits for nothing.
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

// heldPipeline is the pipeline while this receiver holds one, and nil once it has been handed back.
//
// Copied under the lock rather than read where it is used: a stop can land between a nil check and
// the call after it, and every reader outside the build reaches the pipeline through here.
func (r *Receiver) heldPipeline() gst.Pipeline {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pipeline
}

// release hands the pipeline back, which a pipeline at NULL is not: the state change stops the
// threads and frees nothing.
//
// The binding unrefs a wrapper when Go collects it, and Go sees the wrapper rather than the decoder
// contexts, buffer pools and device surfaces behind it, so a process opening a pipeline per stream
// climbs by what each one holds and never comes down. Measured at six megabytes, five threads and
// eight descriptors a stream, none of it returned by a stop.
//
// Once per receiver, after the pipeline is at NULL and never before: a running pipeline handed back
// is a decoder still on the device with nothing left pointing at it.
// The element handles go with it. Each holds a ref of its own, so dropping them is what lets the
// children go when the bin releases them, and a method reaching one afterwards would be reaching
// into a stream that has stopped.
func (r *Receiver) release() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pipeline == nil {
		return
	}
	r.volume, r.audioConvert, r.stats = nil, nil, nil
	r.sink, r.fit = nil, nil

	gobject.UnsafeObjectUnref(r.pipeline)
	r.pipeline = nil
}

// since rounds an elapsed time to the millisecond.
// These lines carry seconds of teardown, and the nanoseconds under them say nothing about slow
// versus stuck.
func since(started time.Time) time.Duration {
	return time.Since(started).Round(time.Millisecond)
}
