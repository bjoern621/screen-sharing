// Package pipedelay measures how long a pipeline holds a frame.
//
// One frame's delay is the pipeline's running time when the frame reaches a pad, minus the running
// time the frame carries.
// A live source stamps a buffer at the moment it captures or receives it, so that subtraction is
// the wall clock everything between the source and the pad spent on it: converting, encoding and
// parsing on a publish, depacketizing and decoding on a receive.
//
// Measured at a pad and never in a sink's render callback.
// A synchronizing sink holds each frame until its presentation time, so a reading taken after that
// wait reports the latency the pipeline configured rather than the work it did, and the difference
// between the two is the whole diagnosis: a pipeline whose work fills its latency window drops the
// next frame that runs long.
//
// Nothing here is a rate.
// A probe accumulates and whoever samples it divides two readings by the interval between them,
// the split every counter in this app is read under.
package pipedelay

import (
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
)

// Probe is one measuring point and what has passed it.
//
// The streaming thread writes every field and a sampler reads them, which is what makes all of them
// atomic: a probe is read while frames are still arriving, and no reader needs them consistent with
// anything else.
//
// element supplies the clock and the base time a running time is taken against, both of which move
// with the pipeline's state, so neither is cached.
type Probe struct {
	element gst.Element
	// segment converts a buffer's timestamp into a running time.
	// A copy, the event carrying it not outliving the callback that saw it, and a pointer swap, it
	// changing once per stream start against once per frame here.
	segment atomic.Pointer[gst.Segment]
	total   atomic.Uint64 // ns
	frames  atomic.Uint64
}

// Reading is what a probe has accumulated since the pipeline started.
//
// Two counters rather than an average, because the average worth reading is the one over the
// interval between two samples and this side has no interval.
// Frames is what Total was summed over, so a sampler divides one delta by the other.
type Reading struct {
	Total  time.Duration
	Frames uint64
}

// Watch measures at the named static pad of element.
//
// nil where the element has no such pad, which a caller reads as a pipeline that reports no delay.
// An element growing its pads on request has none to attach to before it links, and a measurement is
// not something the shape of a pipeline may fail over.
func Watch(element gst.Element, padName string) *Probe {
	assert.IsNotNil(element, "a probe measures at an element")
	assert.Assert(padName != "", "a probe names the pad it measures at")

	pad := element.GetStaticPad(padName)
	if pad == nil {
		return nil
	}

	// Two probes and not one mask: the info a callback is handed answers for the kind it was
	// registered under and asserts on any other, so one callback asking "buffer or event?" is a
	// GStreamer critical per frame rather than a branch.
	p := &Probe{element: element}
	pad.AddProbe(gst.PadProbeTypeBuffer, p.measure)
	pad.AddProbe(gst.PadProbeTypeEventDownstream, p.follow)
	return p
}

// Read is what has passed, and the zero Reading for a probe nothing has measured yet.
// Safe on a nil probe, which is the pipeline that carries none.
func (p *Probe) Read() Reading {
	if p == nil {
		return Reading{}
	}
	return Reading{Total: time.Duration(p.total.Load()), Frames: p.frames.Load()}
}

// measure runs on the streaming thread, once per buffer.
//
// A frame whose delay cannot be read is passed and not counted: every frame ahead of the segment
// event, every buffer carrying no timestamp, and every frame of a pipeline that has no clock yet.
// None of those is a measurement of zero, and a zero here would read as a pipeline that held
// nothing.
func (p *Probe) measure(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
	buffer := info.GetBuffer()
	if buffer == nil {
		return gst.PadProbeOK
	}
	delay, ok := p.delayOf(buffer)
	if !ok {
		return gst.PadProbeOK
	}
	p.total.Add(uint64(delay))
	p.frames.Add(1)
	return gst.PadProbeOK
}

// follow keeps the segment the running times are taken against, which arrives ahead of the frames it
// describes and again whenever the stream restarts.
// Copied, because the event carrying it does not outlive this call.
func (p *Probe) follow(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
	event := info.GetEvent()
	if event == nil || event.GetType() != gst.EventSegment {
		return gst.PadProbeOK
	}
	if segment := event.ParseSegment(); segment != nil {
		p.segment.Store(segment.Copy())
	}
	return gst.PadProbeOK
}

// delayOf is one buffer's delay, and false where this pipeline cannot state it.
//
// A negative result is refused rather than clamped to zero.
// It is a frame stamped for a moment that has not arrived, which a source running ahead of the clock
// produces, and zero would report it as a frame that crossed the pipeline instantly.
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

// runningTime is the pipeline's own running time, which is the clock reading with the base time
// taken off it.
// false before the pipeline is given a clock and before it is scheduled, which is every frame that
// crosses a pad during preroll.
func (p *Probe) runningTime() (uint64, bool) {
	clock := p.element.GetClock()
	if clock == nil {
		return 0, false
	}
	now, base := clock.GetTime(), p.element.GetBaseTime()
	if now == gst.ClockTimeNone || base == gst.ClockTimeNone || now < base {
		return 0, false
	}
	return uint64(now - base), true
}
