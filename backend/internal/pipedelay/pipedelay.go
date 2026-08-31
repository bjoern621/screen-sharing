// Package pipedelay measures how long a pipeline holds a frame.
//
// One frame's delay: pipeline running time when the frame reaches a pad, minus the running time
// the frame carries.
// A live source stamps a buffer as it captures or receives it, so that subtraction is the wall
// clock everything between source and pad spent on it: converting, encoding and parsing
// on a publish, depacketizing and decoding on a receive.
//
// Measured at a pad, never in a sink's render callback.
// A synchronizing sink holds each frame until its presentation time, so a reading taken after
// that wait reports the configured latency rather than the work done, and the difference between
// the two is the diagnosis: a pipeline whose work fills its latency window drops the next frame
// that runs long.
//
// Nothing here is a rate.
// A probe accumulates and the sampler divides two readings by the interval between them, the split
// every counter in this app is read under.
//
// The peak is neither an accumulator nor a rate.
// A mean over any interval hides the single frame that ran long, and that frame is what a sink's
// shed threshold is judged against, so the worst is kept beside the sum rather than derived
// from it.
package pipedelay

import (
	"sync/atomic"
	"time"

	"github.com/go-gst/go-glib/pkg/gobject/v2"
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/padprobe"
)

// Probe is one measuring point and what has passed it.
//
// The streaming thread writes every field and a sampler reads them, hence atomic throughout: read
// while frames still arrive, and no reader needs them consistent with each other.
//
// element supplies the clock and the base time a running time is taken against, both moving
// with the pipeline's state, so neither is cached.
type Probe struct {
	element gst.Element
	// segment converts a buffer's timestamp into a running time.
	// A copy, the carrying event not outliving the callback that saw it.
	// A pointer swap, it changing once per stream start against once per frame here.
	segment atomic.Pointer[gst.Segment]
	total   atomic.Uint64 // ns
	frames  atomic.Uint64
	peak    atomic.Uint64 // ns
}

// Reading is what a probe has accumulated since the pipeline started.
//
// Two counters rather than an average: the average worth reading spans the interval between two
// samples, and this side has no interval.
// Frames is what Total was summed over, so a sampler divides one delta by the other.
type Reading struct {
	Total  time.Duration
	Frames uint64
	// Peak is the worst any one frame cost since the pipeline started, and it never comes down.
	// A high-water mark answers what a mean cannot: whether anything ever crossed the threshold past
	// which a sink stops handing frames over.
	Peak time.Duration
}

// Watch measures at the named static pad of element.
//
// nil where the element has no such pad, read by a caller as a pipeline reporting no delay.
// An element growing its pads on request has none to attach to before it links, and a measurement
// is not something a pipeline's shape may fail over.
func Watch(element gst.Element, padName string) *Probe {
	assert.IsNotNil(element, "a probe measures at an element")
	assert.Assert(padName != "", "a probe names the pad it measures at")

	pad := element.GetStaticPad(padName)
	if pad == nil {
		return nil
	}

	// Two probes, not one mask: the info answers for the kind the callback was registered under and
	// asserts on any other, so one callback asking "buffer or event?" is a GStreamer critical per
	// frame rather than a branch.
	p := &Probe{element: element}
	pad.AddProbe(gst.PadProbeTypeBuffer, p.measure)
	pad.AddProbe(gst.PadProbeTypeEventDownstream, p.follow)
	return p
}

// Read is what has passed, the zero Reading for a probe nothing has measured.
// Safe on a nil probe, the pipeline carrying none.
func (p *Probe) Read() Reading {
	if p == nil {
		return Reading{}
	}
	return Reading{
		Total:  time.Duration(p.total.Load()),
		Frames: p.frames.Load(),
		Peak:   time.Duration(p.peak.Load()),
	}
}

// measure runs on the streaming thread, once per buffer.
//
// A frame whose delay cannot be read is passed and not counted: every frame ahead of the segment
// event, every buffer carrying no timestamp, and every frame of a pipeline without a clock.
// None of those is a measurement of zero, and a zero here would read as a pipeline that held
// nothing.
func (p *Probe) measure(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
	buffer := padprobe.Buffer(info)
	if buffer == nil {
		return gst.PadProbeOK
	}
	delay, ok := p.delayOf(buffer)
	if !ok {
		return gst.PadProbeOK
	}
	p.total.Add(uint64(delay))
	p.frames.Add(1)
	p.raisePeak(uint64(delay))
	return gst.PadProbeOK
}

// raisePeak lifts the high-water mark to ns, leaving it alone where this frame was not the worst.
//
// Compare-and-swap loop rather than load and store: two streaming threads reach one probe wherever
// a pipeline runs several branches through the watched pad.
// Costs one uncontended swap on the single writer every other pipeline here has.
func (p *Probe) raisePeak(ns uint64) {
	for {
		peak := p.peak.Load()
		if ns <= peak {
			return
		}
		if p.peak.CompareAndSwap(peak, ns) {
			return
		}
	}
}

// follow keeps the segment running times are taken against, arriving ahead of the frames it
// describes and again on every stream restart.
// Copied, the carrying event not outliving this call.
func (p *Probe) follow(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
	event := padprobe.Event(info)
	if event == nil || event.GetType() != gst.EventSegment {
		return gst.PadProbeOK
	}
	if segment := event.ParseSegment(); segment != nil {
		p.segment.Store(segment.Copy())
	}
	return gst.PadProbeOK
}

// delayOf is one buffer's delay, false where this pipeline cannot state it.
//
// A negative result is refused rather than clamped to zero: a frame stamped for a moment that has
// not arrived, produced by a source running ahead of the clock, would read as a frame that crossed
// the pipeline instantly.
func (p *Probe) delayOf(buffer *gst.Buffer) (time.Duration, bool) {
	segment := p.segment.Load()
	if segment == nil || segment.Format() != gst.FormatTime {
		return 0, false
	}
	pts := buffer.PTS()
	if pts == gst.ClockTimeNone {
		return 0, false
	}
	stamped := segment.ToRunningTime(gst.FormatTime, uint64(pts))
	if stamped == uint64(gst.ClockTimeNone) {
		return 0, false
	}

	now, ok := p.runningTime()
	if !ok || now < stamped {
		return 0, false
	}
	return time.Duration(now - stamped), true
}

// runningTime is the pipeline's own running time: clock reading minus base time.
// false before the pipeline has a clock and before it is scheduled, which is every frame crossing
// a pad during preroll.
func (p *Probe) runningTime() (uint64, bool) {
	clock := p.element.GetClock()
	if clock == nil {
		return 0, false
	}
	// Handed over with a reference, dropped from a finalizer, taken once per frame here
	// (internal/padprobe states the same cost on the buffers beside it).
	defer gobject.UnsafeObjectUnref(clock)

	now, base := clock.GetTime(), p.element.GetBaseTime()
	if now == gst.ClockTimeNone || base == gst.ClockTimeNone || now < base {
		return 0, false
	}
	return uint64(now - base), true
}
