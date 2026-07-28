// Package gstreamer decodes a stream with GStreamer: the transport's source
// fragment, then decode and render into a gtk4paintablesink.
//
// It is the receive-side counterpart of the publish pipeline in
// desktop/publish/gstreamer.go, and registers itself as a player backend, so
// nothing above the seam names GStreamer.
package gstreamer

import (
	"strings"
	"sync"

	"github.com/go-gst/go-gst/pkg/gst"
	"github.com/go-gst/go-gst/pkg/gstpbutils"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// Backend is the name this backend registers under.
const Backend = "gstreamer"

func init() {
	player.Register(Backend, New)
}

// Element names the launch line gives the elements the player reads back.
const (
	decodeName  = "dec"
	convertName = "conv"
	sinkName    = "sink"
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

// renderChain is what a stream's source fragment is completed with:
//
//	<source> ! decodebin ! videoconvert ! RGBA/sRGB ! queue ! gtk4paintablesink
//
// decodebin picks the depayloader/demuxer and decoder from the stream's caps,
// backed by gst-libav, so the grid decodes everything a native ffplay/mpv window
// decodes, HEVC 4:4:4 and RGB included.
//
// videoconvert directly after decodebin keeps parse-launch's delayed linking off
// the audio pads: their caps never match, so only the video pad joins the branch.
// An audio pad instead gets its own branch, built when the pad appears
// (audio.go), so a video-only stream carries no idle audio elements.
//
// The capsfilter pins videoconvert's output to sRGB RGBA. Without it the sink
// also accepts raw YUV, videoconvert passes the decoded frames through, and GTK
// color-manages the texture itself: gtk4paintablesink maps an unknown transfer
// function to BT.709 for YUV, so GTK linearizes sRGB-encoded screen content with
// the BT.709 EOTF and lifts every shadow, a visibly washed-out picture on dark
// content. Pinned to RGBA, videoconvert applies matrix and range only
// (gamma-mode defaults to none) and tags the result sRGB, the same
// interpretation ffplay uses. 4:4:4 and RGB streams keep full chroma; nothing on
// this path subsamples.
var renderChain = []string{
	"decodebin name=" + decodeName,
	"videoconvert name=" + convertName + " n-threads=0",
	"video/x-raw,format=RGBA,colorimetry=sRGB",
	renderQueue,
	"gtk4paintablesink name=" + sinkName,
}

// Describe renders the launch line one stream's source fragment is played
// through. It is exported so a measurement runs the line this backend actually
// plays rather than a copy of it.
func Describe(source string) string {
	return strings.Join(append([]string{source}, renderChain...), " ! ")
}

// initOnce initializes GStreamer with the first pipeline, so the binary has no
// init-order dependency between main and the players.
var initOnce sync.Once

// initGStreamer brings up the library. pbutils names the codecs the overlay
// shows, off the negotiated caps.
func initGStreamer() {
	initOnce.Do(func() {
		gst.Init()
		gstpbutils.PbUtilsInit()
	})
}
