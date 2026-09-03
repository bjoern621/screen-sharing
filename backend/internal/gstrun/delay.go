package gstrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/padprobe"
	"bjoernblessin.de/screenshare/internal/pipedelay"
)

// Reporting what the publish leg costs a frame.
//
// The parent cannot measure any of it.
// A delay is a subtraction against the pipeline's own clock and the transport's own link counters,
// and both live in this process, where the pipeline does.
// What crosses is the reading and never a verdict: how the stages add up is the parent's, where
// the receiving side's stages are known too.
//
// The line goes on standard output beside the caps and the pointer, under a prefix of its own, and
// the parent's reader skips a line it does not recognise.

// DelayPrefix leads the line one delay reading is reported on, as JSON.
//
// JSON rather than the whitespace fields a position takes, half of this reading being absent
// on a leg that measures no link: presence is the message, and a field spelled as a number would
// report an unmeasured window as a window of nothing.
const DelayPrefix = "screenshare-delay "

// DelayFlag names the element a frame's delay is measured at, as an argument after the subcommand
// (cmd/backend).
// A run given none measures none, which is every run that reports no progress either: the delay
// rides with the meter rather than with the pipeline.
const DelayFlag = "--delay="

// ShedFlag names the queue that drops what the encoder could not take, counted at both its ends.
// Reported beside the delay because the two are one reading: a leg short of the capture rate costs
// frames here and would otherwise cost delay, so what was dropped is what the delay did not grow
// by.
const ShedFlag = "--shed="

// delayInterval is how often a reading is written, the cadence progressreport prints at and
// the cadence the backend samples a decode at.
const delayInterval = time.Second

// Delay is one reading of what this pipeline costs a frame.
//
// Transit and Frames are cumulative, so the parent divides two readings by the interval between
// them rather than trusting this side's cadence, the split every other counter here is read under.
type Delay struct {
	// TransitNs is the wall clock between the capture stamping a frame and the measured element
	// passing it on, summed over Frames frames: converting, encoding and parsing.
	TransitNs uint64 `json:"transitNs"`
	Frames    uint64 `json:"frames"`
	// Dropped is what the shed threw away since the pipeline started, absent on a run carrying no
	// shed.
	// Cumulative like the pair above, so the parent divides two readings by the interval between them.
	Dropped *uint64 `json:"dropped,omitempty"`
}

// watchDelay attaches the probe one reading is taken off, and nil where this pipeline cannot carry
// one.
//
// The element is the parent's to name, since the parent built the pipeline: measuring at whatever
// happens to be last would measure a tap's own sink on a run that carries one.
// An element the pipeline does not hold, or one whose pad grows on request, measures nothing rather
// than ending the run, an unmeasured publish being a publish.
//
// Attached before the pipeline plays and never from the reporting goroutine, so the probe
// is on the pad ahead of the first frame across it.
func watchDelay(pipeline gst.Pipeline, element string) *pipedelay.Probe {
	assert.Assert(element != "", "a delay measurement names the element it is taken at")

	el := pipeline.GetByName(element)
	if el == nil {
		return nil
	}
	return pipedelay.Watch(el, "src")
}

// reportDelay writes one reading per tick until the context ends, on its own goroutine.
// shed is nil on a pipeline carrying none, which reports no drop rather than a drop of nothing.
func reportDelay(ctx context.Context, probe *pipedelay.Probe, shed *shedCount, out io.Writer) {
	assert.IsNotNil(ctx, "a delay report runs under a context")
	assert.IsNotNil(out, "a delay report is written to a writer")
	assert.IsNotNil(probe, "a delay report reads a probe")

	ticker := time.NewTicker(delayInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		reading := probe.Read()
		delay := Delay{TransitNs: uint64(reading.Total), Frames: reading.Frames}
		if dropped, counted := shed.Read(); counted {
			delay.Dropped = &dropped
		}
		writeDelay(out, delay)
	}
}

// shedCount is what one queue took in, what it handed on, and how deep it is, the three together
// being what it dropped.
//
// A queue keeps no drop counter of its own, so the pair at its ends is what there is, and the depth
// is what separates a frame still on its way from one thrown away.
// Written by the streaming threads and read by the reporting one, hence atomic.
// The depth is a property read on the reporting thread alone.
type shedCount struct {
	in  atomic.Uint64
	out atomic.Uint64
	// dropped is the highest figure any reading reached, making a total that only counts up out
	// of three readings taken one after another.
	dropped atomic.Uint64
	// level is how many buffers the queue holds at this moment, nil where nothing states one.
	// A function rather than the element, so what a reading does with a depth can be stated without
	// a queue that happens to be holding one.
	level func() uint64
}

// watchShed counts at both ends of the named queue, and nil where the pipeline holds no such
// element or it grows its pads on request.
// An uncounted shed is a shed, so nothing here fails a run.
func watchShed(pipeline gst.Pipeline, element string) *shedCount {
	assert.Assert(element != "", "a shed count names the queue it is taken at")

	el := pipeline.GetByName(element)
	if el == nil {
		return nil
	}
	sink, src := el.GetStaticPad("sink"), el.GetStaticPad("src")
	if sink == nil || src == nil {
		return nil
	}

	c := &shedCount{level: queueLevel(el)}
	sink.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		if padprobe.Buffer(info) != nil {
			c.in.Add(1)
		}
		return gst.PadProbeOK
	})
	src.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		if padprobe.Buffer(info) != nil {
			c.out.Add(1)
		}
		return gst.PadProbeOK
	})
	return c
}

// Read is what the shed has dropped, and false for a pipeline carrying none.
// Safe on a nil count, that pipeline.
//
// What went in less what came out less what is in the queue at this moment.
// The queue's depth is not the same one frame at every reading, so the pair alone counts every
// frame in flight as dropped and uncounts it once it leaves, a total that goes down: a readout
// counting backwards in front of whoever is watching it.
//
// The three readings are taken one after another, so a frame crossing between two of them lands
// on one side or the other, and the high-water mark keeps that from moving the figure backwards.
// A dropped frame is never undropped.
func (c *shedCount) Read() (uint64, bool) {
	if c == nil {
		return 0, false
	}
	in, out, held := c.in.Load(), c.out.Load(), c.held()
	dropped := uint64(0)
	if in > out+held {
		dropped = in - out - held
	}

	for {
		seen := c.dropped.Load()
		if dropped <= seen {
			return seen, true
		}
		if c.dropped.CompareAndSwap(seen, dropped) {
			return dropped, true
		}
	}
}

// held is how many buffers the queue is holding at this moment, and zero where nothing states one.
func (c *shedCount) held() uint64 {
	if c.level == nil {
		return 0
	}
	return c.level()
}

// queueLevel reads a queue's own depth, in buffers.
//
// The property is a guint, which the binding answers as uint32.
// The other widths are here because a figure read through a type assertion that stops matching goes
// quietly to zero, and a zero depth counts every frame in flight as dropped.
// An element that is no queue answers something else and reads as no depth, which is what it has.
func queueLevel(el gst.Element) func() uint64 {
	return func() uint64 {
		switch level := el.ObjectProperty("current-level-buffers").(type) {
		case uint32:
			return uint64(level)
		case uint64:
			return level
		case int32:
			return uint64(max(level, 0))
		case int64:
			return uint64(max(level, 0))
		default:
			return 0
		}
	}
}

// writeDelay renders one reading onto the child's output.
// Beside ParseDelay so the two spellings are one, and separate from the loop so a test can state
// that they are.
func writeDelay(out io.Writer, delay Delay) {
	assert.IsNotNil(out, "a delay reading is written to a writer")

	// A struct of numbers, so a failure here is this file having grown a field encoding/json cannot
	// carry rather than anything a run did.
	line, err := json.Marshal(delay)
	assert.IsNil(err, "a delay reading is a struct of numbers and encodes", err)

	fmt.Fprintf(out, "%s%s\n", DelayPrefix, line)
}

// ParseDelay reads one reported reading back, false for a line that is not one.
//
// Beside the writer so the two spellings are one, as ParsePointer is.
// A line that does not parse answers false rather than asserting: the reader is pointed
// at a child's whole standard output, where an unrelated line is the ordinary case.
func ParseDelay(line string) (Delay, bool) {
	rest, ok := strings.CutPrefix(line, DelayPrefix)
	if !ok {
		return Delay{}, false
	}
	var delay Delay
	if err := json.Unmarshal([]byte(rest), &delay); err != nil {
		return Delay{}, false
	}
	return delay, true
}
