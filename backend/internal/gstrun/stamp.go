package gstrun

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/framestamp"
	"bjoernblessin.de/screenshare/internal/padprobe"
	"bjoernblessin.de/screenshare/internal/pipedelay"
)

// Writing the wall clock into every encoded frame, so a viewer can time the path between the two
// machines.
//
// Measured at the same pad the publish delay is: what a stamp says is "the encoder was done with
// this picture now", which is exactly what that reading ends at, so the two stages meet rather than
// overlap.
//
// A frame is stamped where the codec has a unit for it and the buffers hold whole pictures, and
// passed untouched otherwise.
// Nothing here fails a run: an unstamped stream plays, and a viewer of one reports the path as
// unmeasured (internal/framestamp).

// linkWindow is the delivery window the publish leg settled on, as the stamp carries it.
//
// Held rather than read per frame: the sink answers once a second and the reporting tick is already
// asking it, so a second reader would be a property read per frame for a figure that changes at
// most once between ticks.
// Zero until the first tick, and zero for a leg stating no window, which a reader takes as unstated
// either way.
type linkWindow struct{ ms atomic.Uint32 }

func (w *linkWindow) take(ms *float64) {
	if ms == nil || *ms < 0 || *ms > math.MaxUint16 {
		w.ms.Store(0)
		return
	}
	w.ms.Store(uint32(*ms))
}

func (w *linkWindow) read() uint16 { return uint16(w.ms.Load()) }

// stampFrames writes the publishing machine's reading into each frame leaving the named element:
// the wall clock, and what this pipeline has measured of its own share of the path.
//
// The publishing stages are measured here and travel nowhere else, so without this a viewer of
// somebody else's stream has no way to them at all.
// They ride as the probe's own running totals rather than as an average, so the viewer divides them
// over its own sampling interval, which is how it reads every other counter (internal/pipedelay).
//
// Attached before the pipeline plays, like the delay probe, so no frame crosses the pad unstamped.
//
// Two probes and not one. How a stream is framed comes off the caps event, which arrives ahead of
// the frames it describes and again whenever the stream renegotiates, so the frames themselves cost
// a pointer read: caps taken per frame would be one wrapped object per frame for a collection to
// find, which is the cost internal/padprobe exists to keep off this path.
func stampFrames(pipeline gst.Pipeline, element string, probe *pipedelay.Probe, window *linkWindow) {
	assert.Assert(element != "", "a stamp names the element it is written at")
	assert.IsNotNil(window, "a stamp reads the leg's window from somewhere")

	el := pipeline.GetByName(element)
	if el == nil {
		return
	}
	pad := el.GetStaticPad("src")
	if pad == nil {
		return
	}

	var carriage atomic.Pointer[framestamp.Carriage]
	pad.AddProbe(gst.PadProbeTypeEventDownstream, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		event := padprobe.Event(info)
		if event == nil || event.GetType() != gst.EventCaps {
			return gst.PadProbeOK
		}
		if c, read := carriageOf(event.ParseCaps()); read {
			carriage.Store(&c)
			if _, carried := framestamp.Unit(c, framestamp.Stamp{At: time.Now()}); !carried {
				logger.Debugf("a %s stream framed in %q carries no stamp, so no viewer of it can time the way there",
					c.Media, c.Alignment)
			}
		}
		return gst.PadProbeOK
	})

	pad.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		c := carriage.Load()
		if c == nil {
			return gst.PadProbeOK
		}
		buffer := padprobe.Buffer(info)
		if buffer == nil {
			return gst.PadProbeOK
		}
		unit, carried := framestamp.Unit(*c, stampOf(probe, window))
		if !carried {
			return gst.PadProbeOK
		}
		insert(buffer, unit, *c)
		return gst.PadProbeOK
	})
}

// stampOf is what this frame carries out: the moment it left the encoder, and what this pipeline
// has spent on every frame so far.
//
// A probe this run carries none of reports no publishing stages, which is a stage nothing measured
// and never a stage that cost nothing.
// The running total is milliseconds where the probe counts nanoseconds, and it wraps at the width
// the stamp carries it in, which a viewer sees as one interval it cannot divide.
func stampOf(probe *pipedelay.Probe, window *linkWindow) framestamp.Stamp {
	s := framestamp.Stamp{At: time.Now(), LinkMs: window.read()}
	reading := probe.Read()
	if reading.Frames == 0 {
		return s
	}
	s.PublishMs = uint32(reading.Total / time.Millisecond)
	s.PublishFrames = uint32(reading.Frames)
	return s
}

// carriageOf is how a stream under these caps is framed, and false for caps describing nothing this
// can read.
func carriageOf(caps *gst.Caps) (framestamp.Carriage, bool) {
	if caps == nil || caps.GetSize() == 0 {
		return framestamp.Carriage{}, false
	}
	s := caps.GetStructure(0)
	if s == nil {
		return framestamp.Carriage{}, false
	}

	c := framestamp.Carriage{
		Media:     s.GetName(),
		Format:    s.GetString("stream-format"),
		Alignment: s.GetString("alignment"),
	}
	if size, ok := s.GetValue("nal-length-size").(int); ok {
		c.LengthSize = size
	}
	return c, true
}

// insert puts the unit into the frame where the codec's framing takes it, in place.
//
// In place because a probe here cannot hand back a different buffer: the binding exposes what the
// probe was given and no way to replace it, so what reaches the muxer is this buffer or nothing.
// A buffer whose memory is shared is left alone rather than written through, a write there reaching
// whatever else holds it.
//
// The bytes ahead of the insertion point are copied into the same block as the unit and cut off the
// front of the frame, which is what puts a block boundary where the unit goes without touching the
// picture: those bytes are the parameter sets, tens of bytes against a picture's thousands.
func insert(buffer *gst.Buffer, unit []byte, c framestamp.Carriage) {
	assert.IsNotNil(buffer, "a stamp is written into a buffer")
	assert.Assert(len(unit) > 0, "a written stamp has bytes")

	if !buffer.IsAllMemoryWritable() {
		return
	}
	read, mapped := buffer.Map(gst.MapRead)
	if !mapped {
		return
	}
	at := framestamp.Offset(c, read.Data())
	head := append([]byte{}, read.Data()[:at]...)
	read.Close()

	carrier := gst.NewBufferAllocate(nil, uint(len(head)+len(unit)), nil)
	if carrier == nil {
		return
	}
	write, mapped := carrier.Map(gst.MapWrite)
	if !mapped {
		return
	}
	copy(write.Data(), head)
	copy(write.Data()[len(head):], unit)
	write.Close()

	buffer.Resize(at, int(buffer.GetSize())-at)
	buffer.PrependMemory(carrier.GetAllMemory())
}
