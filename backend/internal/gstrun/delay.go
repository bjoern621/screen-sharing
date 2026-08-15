package gstrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/pipedelay"
)

// Reporting what the publish leg costs a frame.
//
// The parent cannot measure any of it.
// A delay is a subtraction against the pipeline's own clock and the transport's own link counters,
// and both live in this process, where the pipeline does.
// What crosses is the reading and never a verdict: how the stages add up is the parent's, which is
// where the receiving side's stages are known too.
//
// The line goes on standard output beside the caps and the pointer, under a prefix of its own, and
// the parent's reader skips a line it does not recognise.

// DelayPrefix leads the line one delay reading is reported on, as JSON.
//
// JSON rather than the whitespace fields a position takes, because half of this reading is absent on
// a leg that measures no link: presence is the message, and a field spelled as a number would report
// an unmeasured window as a window of nothing.
const DelayPrefix = "screenshare-delay "

// DelayFlag names the element a frame's delay is measured at, as an argument after the subcommand
// (cmd/backend).
// A run given none measures none, which is every run that reports no progress either: the delay
// rides with the meter rather than with the pipeline.
const DelayFlag = "--delay="

// delayInterval is how often a reading is written, the cadence progressreport prints at and the
// cadence the backend samples a decode at.
const delayInterval = time.Second

// Delay is one reading of what this pipeline costs a frame.
//
// Transit and Frames are cumulative, so the parent divides two readings by the interval between them
// rather than trusting this side's cadence, which is what every other counter here is read under.
// The link figures are the transport's own and are absent on a leg that keeps none: an RTSP or WHIP
// sink states no window and times no round trip.
type Delay struct {
	// TransitNs is the wall clock between the capture stamping a frame and the measured element
	// passing it on, summed over Frames frames: converting, encoding and parsing.
	TransitNs uint64 `json:"transitNs"`
	Frames    uint64 `json:"frames"`
	// LinkMs is the delivery window the publish leg settled on with the relay, the delay every packet
	// is held for so a lost one has room to be sent again.
	LinkMs *float64 `json:"linkMs,omitempty"`
	// RttMs is the round trip to the relay on this leg, which is what says whether LinkMs has room for
	// a retransmission at all.
	RttMs *float64 `json:"rttMs,omitempty"`
}

// linkSource is the sink factory whose stats structure states a leg's own delivery delay and round
// trip, and the two fields it states them under.
//
// A table because which counter means what is the element's knowledge, the shape internal/receive
// reads its transport counters through.
// Only the sinks listed here are asked at all: reading a property off an element that has none is a
// GLib warning on a pipeline that is working, and a publish carries sinks that count bytes and draw
// pictures beside the one on the wire.
type linkSource struct {
	window string
	rtt    string
}

var linkSources = map[string]linkSource{
	"srtsink": {window: "negotiated-latency-ms", rtt: "rtt-ms"},
}

// watchDelay attaches the probe one reading is taken off, and nil where this pipeline cannot carry
// one.
//
// The element is the parent's to name, since the parent built the pipeline: measuring at whatever
// happens to be last would measure a tap's own sink on a run that carries one.
// An element the pipeline does not hold, or one whose pad grows on request, measures nothing rather
// than ending the run, an unmeasured publish being a publish.
//
// Attached before the pipeline plays and never from the reporting goroutine, so the probe is on the
// pad ahead of the first frame across it.
func watchDelay(pipeline gst.Pipeline, element string) *pipedelay.Probe {
	assert.Assert(element != "", "a delay measurement names the element it is taken at")

	el := pipeline.GetByName(element)
	if el == nil {
		return nil
	}
	return pipedelay.Watch(el, "src")
}

// reportDelay writes one reading per tick until the context ends, on its own goroutine.
func reportDelay(ctx context.Context, pipeline gst.Pipeline, probe *pipedelay.Probe, out io.Writer) {
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
		delay.LinkMs, delay.RttMs = linkOf(pipeline)
		writeDelay(out, delay)
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

// linkOf is the delivery window and the round trip the publish leg reports, both absent where this
// pipeline holds no sink that keeps them.
func linkOf(pipeline gst.Pipeline) (window, rtt *float64) {
	for v := range pipeline.IterateSinks().Values() {
		el, ok := v.(gst.Element)
		if !ok {
			continue
		}
		f := el.GetFactory()
		if f == nil {
			continue
		}
		source, keeps := linkSources[f.GetName()]
		if !keeps {
			continue
		}
		stats, _ := el.ObjectProperty("stats").(*gst.Structure)
		if stats == nil {
			continue
		}
		return statField(stats, source.window), statField(stats, source.rtt)
	}
	return nil, nil
}

// statField is one figure off an element's stats structure, nil where the element keeps no such
// field.
func statField(stats *gst.Structure, key string) *float64 {
	if !stats.HasField(key) {
		return nil
	}
	value, ok := numberOf(stats.GetValue(key))
	if !ok {
		return nil
	}
	return &value
}

// numberOf reads one stats field as a number, false for a type no link counter is kept in.
func numberOf(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

// ParseDelay reads one reported reading back, false for a line that is not one.
//
// Beside the writer so the two spellings are one, as ParsePointer is.
// A line that does not parse answers false rather than asserting: the reader is pointed at a child's
// whole standard output, where an unrelated line is the ordinary case.
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
