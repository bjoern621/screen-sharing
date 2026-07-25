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
//
// The short leaky queue decouples decode from render: when a burst outruns the
// compositor the newest frames win, instead of the sink building a backlog of
// stale ones.
var renderChain = []string{
	"decodebin name=" + decodeName,
	"videoconvert name=" + convertName + " n-threads=0",
	"video/x-raw,format=RGBA,colorimetry=sRGB",
	"queue max-size-buffers=3 leaky=downstream",
	"gtk4paintablesink name=" + sinkName,
}

// describe renders the launch line for one stream's source fragment.
func describe(source string) string {
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
