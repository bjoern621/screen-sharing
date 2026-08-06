// Package gstreamer decodes a stream with GStreamer: the transport's source
// fragment, then decode and render into a gtk4paintablesink.
//
// The render chains between the two are a table (chains.go), which is the only
// place a launch line is written and the only place that says where a chain holds
// its frames and what it claims about their colour. What a run then negotiated is
// read back off the pads (memory.go), so the overlay reports what happened rather
// than what was asked for.
//
// It is the receive-side counterpart of the publish pipeline in
// desktop/internal/publish/gstreamer.go, and registers itself as a player backend, so
// nothing above the seam names GStreamer.
package gstreamer

import (
	"sync"

	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstpbutils"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// Backend is the name this backend registers under.
const Backend = "gstreamer"

func init() {
	player.Register(Backend, New, Chains)
}

// Element names the launch line gives the elements the player reads back.
const (
	decodeName = "dec"
	fitName    = "fit"
	sinkName   = "sink"
)

// renderQueue is the buffer between the decode thread and the render thread,
// bounded in time rather than in buffers so the bound means the same thing at
// every resolution and frame rate.
//
// It must not leak. A leaking queue in front of a sink that syncs on the clock
// costs roughly half the frame rate: the sink holds each buffer until its
// presentation time, so the queue sits at its bound for most of every frame
// period, and each arrival then drops a frame that was going to be shown.
// Non-leaky, the queue backpressures instead, and frames that really are too
// late are dropped once, by the sink, which is the element that knows what late
// means. Measured against a 60 fps 1080p stream, a three-buffer leaky queue
// rendered 22 fps where the same chain without the leak rendered 46, the rate
// the source delivered.
const renderQueue = "queue max-size-buffers=0 max-size-bytes=0 max-size-time=100000000"

// initOnce initializes GStreamer with the first pipeline, so the binary has no
// init-order dependency between main and the players.
var initOnce sync.Once

// initGStreamer brings up the library. pbutils names the codecs the overlay
// shows, off the negotiated caps.
func initGStreamer() {
	initOnce.Do(func() {
		// Before Init: the plugin path is read during the registry scan Init
		// runs, and a path set afterwards is a path nothing rescans.
		useBundledPlugins()

		gst.Init()
		gstpbutils.PbUtilsInit()
	})
}
