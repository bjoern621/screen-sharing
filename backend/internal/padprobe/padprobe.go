// Package padprobe reads what a pad probe was handed and leaves no reference behind.
//
// The binding takes its own reference on a borrowed GstBuffer or GstEvent and drops it from a Go
// finalizer.
// A few wrapper bytes against a frame's megabytes never grow the heap, so a callback returning
// without an unref pins every frame it saw until something unrelated collects.
// Dropped here instead.
//
// Valid for the calling callback and nothing after it: the pipeline holds its own reference across
// the probe, and a value kept past the return names memory the pipeline has recycled.
package padprobe

import (
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
)

// Buffer is the frame crossing the pad, nil where the probe carries none.
func Buffer(info *gst.PadProbeInfo) *gst.Buffer {
	assert.IsNotNil(info, "a pad probe reads the info it was handed")

	buffer := info.GetBuffer()
	if buffer == nil {
		return nil
	}
	gst.UnsafeBufferUnref(buffer)
	return buffer
}

// Event is the event crossing the pad, nil where the probe carries none.
func Event(info *gst.PadProbeInfo) *gst.Event {
	assert.IsNotNil(info, "a pad probe reads the info it was handed")

	event := info.GetEvent()
	if event == nil {
		return nil
	}
	gst.UnsafeEventUnref(event)
	return event
}
