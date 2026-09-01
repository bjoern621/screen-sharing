package gstrun

import (
	"context"
	"time"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/padprobe"
	"bjoernblessin.de/screenshare/internal/pointer"
)

// Where a position comes from, per capture.
//
// Who can answer is the display server's property.
// X11 hands any client the pointer's position on request, so that backend polls it.
// A Wayland client cannot ask, and what knows is the cursor metadata PipeWire carries beside each
// frame, which only the process holding the capture reads.
// Both land in one hold, in the captured picture's own pixels, so nothing downstream of it knows
// which session it is on.

// pointerSources is how each capture element answers where the pointer is, keyed by factory.
//
// A table for the reason publish/cursor.go's is one: which backends answer at all is stated there,
// and a source per factory here is the same set read from the other side.
// A factory absent from the map is a capture that answers nothing,
// which the rules already refuse the metadata mode on.
var pointerSources = map[string]func(context.Context, gst.Element, *pointerHold){
	"ximagesrc":   x11Pointer,
	"pipewiresrc": portalPointer,
}

// watchPointer attaches the source this pipeline's capture answers through,
// and nil where its capture answers none.
//
// The source is found by shape and then by factory, as reportCaps finds the capture:
// a name would live both here and in whichever backend built the description.
//
// Attached before the pipeline plays, so no frame crosses the capture's pad unwatched.
func watchPointer(ctx context.Context, pipeline gst.Pipeline) *pointerHold {
	assert.IsNotNil(ctx, "a pointer source runs under a context")

	for v := range pipeline.IterateSources().Values() {
		el, ok := v.(gst.Element)
		if !ok {
			continue
		}
		f := el.GetFactory()
		if f == nil {
			continue
		}
		attach, answers := pointerSources[f.GetName()]
		if !answers {
			continue
		}
		pad := el.GetStaticPad("src")
		if pad == nil {
			continue
		}

		hold := &pointerHold{}
		watchPointerSize(pad, hold)
		attach(ctx, el, hold)
		return hold
	}
	return nil
}

// watchPointerSize follows the size of the picture the capture produces, off its caps.
//
// A probe rather than a read: caps arrive ahead of the frames they describe and again whenever
// the stream renegotiates, and a size read once would make a fraction of the picture before it.
func watchPointerSize(pad gst.Pad, hold *pointerHold) {
	assert.IsNotNil(hold, "a size probe writes to a hold")

	pad.AddProbe(gst.PadProbeTypeEventDownstream, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		event := padprobe.Event(info)
		if event == nil || event.GetType() != gst.EventCaps {
			return gst.PadProbeOK
		}
		caps := event.ParseCaps()
		if caps == nil || caps.GetSize() == 0 {
			return gst.PadProbeOK
		}
		s := caps.GetStructure(0)
		if s == nil {
			return gst.PadProbeOK
		}
		width, gotWidth := s.GetInt("width")
		height, gotHeight := s.GetInt("height")
		if gotWidth && gotHeight {
			hold.size(int(width), int(height))
		}
		return gst.PadProbeOK
	})
}

// x11Pointer polls the X server, XQueryPointer answering any client that asks with nothing
// to subscribe to.
//
// The reading is against the display's root and the capture is one crop out of it, so the crop's
// own origin comes off here.
// It is read off the element rather than enumerated:
// the crop the frames carry is the one in its properties,
// and an origin read fresh would place the pointer against a layout they were not captured under.
//
// A reader that will not answer ends the loop rather than writing nothing forever: a session
// with no X server has no position to have.
func x11Pointer(ctx context.Context, el gst.Element, hold *pointerHold) {
	assert.IsNotNil(hold, "an X11 pointer source writes to a hold")

	originX, originY := elementInt(el, "startx"), elementInt(el, "starty")
	go func() {
		reader, ok := pointer.NewX11()
		if !ok {
			return
		}
		defer reader.Close()

		ticker := time.NewTicker(pointer.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p, answered := reader.Read()
				if !answered {
					return
				}
				hold.take(p.X-originX, p.Y-originY, p.Visible)
			}
		}
	}()
}

// portalPointer takes the position off the frames, where the compositor puts it under the portal's
// metadata cursor mode.
//
// pipewiresrc carries it as a region of interest named "cursor" and attaches none for a pointer
// that is not over the captured region, so a frame without one is the pointer having left
// (cursormeta_linux.go).
//
// The probe sits on the capture's own pad, ahead of everything that scales or repeats a picture:
// the position is a fraction of what was captured, and imagefreeze downstream repeats a frame
// without repeating what it says about the pointer.
func portalPointer(_ context.Context, el gst.Element, hold *pointerHold) {
	assert.IsNotNil(hold, "a portal pointer source writes to a hold")

	pad := el.GetStaticPad("src")
	if pad == nil {
		return
	}
	pad.AddProbe(gst.PadProbeTypeBuffer, func(_ gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		buffer := padprobe.Buffer(info)
		if buffer == nil {
			return gst.PadProbeOK
		}
		x, y, over := cursorAt(buffer)
		hold.take(x, y, over)
		return gst.PadProbeOK
	})
}

// elementInt is one of an element's own integer properties, 0 where it states none.
// Every width an integer property is kept in reads,
// a figure read through a type assertion that stops matching going quietly to zero.
func elementInt(el gst.Element, name string) int {
	switch v := el.ObjectProperty(name).(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	default:
		return 0
	}
}
