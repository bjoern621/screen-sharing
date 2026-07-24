package main

/*
#cgo pkg-config: gobject-2.0
#include <stdlib.h>
#include <glib-object.h>

// grab_object_property returns a new reference to an object-typed property.
static gpointer grab_object_property(gpointer object, const char *name) {
	gpointer out = NULL;
	g_object_get(object, name, &out, NULL);
	return out;
}
*/
import "C"

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/go-gst/go-glib/pkg/gobject/v2"
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/screenshare/watch"
)

// player is one stream's receive pipeline, decoding natively and ending in a
// gtk4paintablesink whose paintable the tile's GtkPicture draws.
type player struct {
	pipeline  gst.Pipeline
	paintable *gdk.Paintable
	cancel    context.CancelFunc
}

// newPlayer parses and starts the pipeline for one stream. onEnd fires from a
// background goroutine when the stream errors or ends; the pipeline is already
// stopped by then.
//
// videoconvert directly after decodebin keeps parse_launch's delayed linking
// off the audio pads, same as the wall: their caps never match, so only the
// video pad joins the branch. videoconvert negotiates an RGB(A) format with
// the sink, so 4:4:4 and RGB streams reach the GTK scene graph with full
// chroma; nothing on this path subsamples.
func newPlayer(st watch.GridStream, onEnd func(message string)) (*player, error) {
	desc := st.Source +
		" ! decodebin ! videoconvert" +
		" ! queue max-size-buffers=3 leaky=downstream" +
		" ! gtk4paintablesink name=sink"
	el, err := gst.ParseLaunch(desc)
	if err != nil {
		return nil, fmt.Errorf("stream %q: %w", st.Name, err)
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		return nil, fmt.Errorf("stream %q: parse did not yield a pipeline", st.Name)
	}
	sink := pipeline.GetByName("sink")
	if sink == nil {
		return nil, fmt.Errorf("stream %q: no gtk4paintablesink in the pipeline", st.Name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &player{pipeline: pipeline, paintable: paintableOf(sink), cancel: cancel}
	go p.watchBus(ctx, onEnd)
	pipeline.SetState(gst.StatePlaying)
	return p, nil
}

// watchBus reports the first fatal bus message and stops the pipeline: a dead
// receive pipeline has nothing to recover, and stopping it freezes the tile on
// its last frame under the error label.
func (p *player) watchBus(ctx context.Context, onEnd func(message string)) {
	defer p.cancel()
	for msg := range p.pipeline.GetBus().Messages(ctx) {
		switch msg.Type() {
		case gst.MessageError:
			_, err := msg.ParseError()
			onEnd(err.Error())
			p.pipeline.SetState(gst.StateNull)
			return
		case gst.MessageEOS:
			onEnd("stream ended")
			p.pipeline.SetState(gst.StateNull)
			return
		}
	}
}

func (p *player) stop() {
	p.cancel()
	p.pipeline.BlockSetState(gst.StateNull, gst.ClockTime(time.Second))
}

// paintableOf bridges the two binding worlds: go-gst wraps the sink element,
// gotk4 must wrap the paintable the GtkPicture draws. Both wrap the same C
// GObject, so the property crosses as a raw pointer. g_object_get returns a
// new reference, which AssumeOwnership hands to gotk4's lifetime management;
// the paintable itself is never wrapped by go-glib, so the two runtimes never
// manage the same object.
func paintableOf(sink gst.Element) *gdk.Paintable {
	obj := gobject.UnsafeObjectToGlibNone(sink)
	name := C.CString("paintable")
	defer C.free(unsafe.Pointer(name))
	ptr := C.grab_object_property(C.gpointer(obj), name)
	return &gdk.Paintable{Object: coreglib.AssumeOwnership(unsafe.Pointer(ptr))}
}
