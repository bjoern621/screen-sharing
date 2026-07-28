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
	name      string
	pipeline  gst.Pipeline
	sink      gst.Element
	convert   gst.Element // videoconvert, whose input caps are the decoded frames
	paintable *gdk.Paintable
	cancel    context.CancelFunc
	started   time.Time
	live      atomic.Bool
	frames    atomic.Uint64
	video     decodeTrack
	audio     decodeTrack

	mu           sync.Mutex
	volume       gst.Element // the audio branch's volume element, nil without audio
	audioConvert gst.Element // the audio branch's audioconvert, for the raw audio caps
	// stats are the transport elements statSources matched, in discovery order.
	stats []elementStats
}

// New parses and starts the pipeline for one stream.
func New(st roster.Stream, ev player.Events) (player.Player, error) {
	assert.Assert(st.Source != "", "a stream carries the source fragment to decode", st.Name)
	initGStreamer()

	desc := Describe(st.Source)
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
		pipeline:  pipeline,
		sink:      sink,
		convert:   pipeline.GetByName(convertName),
		paintable: paintableOf(sink),
		cancel:    cancel,
		started:   time.Now(),
	}
	assert.IsNotNil(r.paintable, "the sink hands out its paintable from construction", st.Name)

	r.watchFrames(ev.OnLive)
	r.watchDecodePads(ev.OnAudio)
	r.watchElements()

	go r.watchBus(ctx, ev.OnEnd)
	pipeline.SetState(gst.StatePlaying)
	logger.Infof("stream %q playing over %s", st.Name, st.Transport)
	return r, nil
}

// watchFrames counts the decoded frames and reports the first one. The paintable
// invalidates its contents once per frame: the first emission is the live signal,
// the count feeds the stats overlay's measured fps. The handler stays connected
// because disconnecting from inside the emission is not worth the bookkeeping.
func (r *receiver) watchFrames(onLive func()) {
	r.paintable.Connect("invalidate-contents", func() {
		r.frames.Add(1)
		if r.live.CompareAndSwap(false, true) {
			logger.Debugf("stream %q rendered its first frame", r.name)
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
func (r *receiver) watchBus(ctx context.Context, onEnd func(message string)) {
	defer r.cancel()
	for msg := range r.pipeline.GetBus().Messages(ctx) {
		var message string
		switch msg.Type() {
		case gst.MessageError:
			_, err := msg.ParseError()
			message = err.Error()
		case gst.MessageEOS:
			message = "stream ended"
		default:
			continue
		}
		logger.Warnf("stream %q ended: %s", r.name, message)
		r.pipeline.SetState(gst.StateNull)
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

// Stop tears the pipeline down. The bus watch is cancelled first, so the state
// change it would report as an error goes nowhere.
func (r *receiver) Stop() {
	r.cancel()
	r.pipeline.BlockSetState(gst.StateNull, gst.ClockTime(stopTimeout))
	logger.Debugf("stream %q pipeline stopped", r.name)
}
