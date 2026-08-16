// Package receive decodes one stream for the shell's tile grid: the transport's source
// fragment, then decode and convert into an appsink the frame channel reads
// (docs/viewer-architecture.md, "The receive package").
//
// The render chains between the two are a table (chains.go), the only place a launch line is
// written and the only place stating where a chain holds its frames and what it claims about
// their colour.
// What a run negotiated is read back off the pads (memory.go), so the receive state reports what
// happened rather than what was asked for.
//
// It is the receive-side counterpart of the publish pipeline in internal/publish/gstreamer.go,
// and differs in where the process boundary falls: a publish pipeline is a gst-launch-1.0 child,
// and this side links GStreamer, because a decoded frame has to stay in this process's address
// space to be exported as a GPU handle.
package receive

import (
	"sync"

	"github.com/go-gst/go-gst/pkg/gst"
)

// The names the launch line gives the elements the receiver reads back.
const (
	decodeName = "dec"
	fitName    = "fit"
	sinkName   = "sink"
)

// renderQueue buffers between the decode thread and the sink's thread, bounded in time rather
// than in buffers so the bound means the same at every resolution and frame rate.
//
// It must not leak.
// The sink holds each buffer until its presentation time, so a leaky queue sits at its bound for
// most of every frame period and each arrival drops a frame that was going to be shown: on a
// 60 fps 1080p stream, a three-buffer leaky queue rendered 22 fps where the same chain without
// the leak rendered 46, the rate the source delivered.
// Non-leaky, the queue backpressures and what is really late is dropped once, by the sink, the
// element that knows what late means.
const renderQueue = "queue max-size-buffers=0 max-size-bytes=0 max-size-time=100000000"

// renderSink is the sink every chain ends in: frames leave the pipeline here rather than being
// drawn.
//
// emit-signals hands each sample over as it arrives, and the one-buffer bound backpressures
// rather than discards, for the reason renderQueue does not leak: nothing before the sink knows
// what late means.
// appsink's own drop property stays off with it, that one discarding the newest arrival on a full
// queue rather than the frame whose moment has passed.
//
// sync is stated rather than inherited, because renderQueue's bound is written against a sink
// that holds each buffer until its presentation time.
//
// qos is what keeps a chain short of the rate it is sent from falling behind, and appsink inherits
// it off.
// The sink measures each frame against its moment and sends the overrun upstream, where the decoder
// discards what is already late before the render chain converts it, so shedding costs a decode
// rather than a decode and a conversion.
// Without it nothing on this leg discards anything: the queue backpressures, the transport's flow
// control turns the shortfall into a backlog, and the lag grows for as long as the stream runs.
//
// max-lateness stays at the inherited -1, which hands on every frame however late it is.
// A cutoff at the sink is not a second line of defence, because discarding there drains nothing:
// the backlog sits upstream of it, so a pipeline with qos off falls behind at the same rate whether
// or not the sink is throwing frames away.
// With qos on it only takes frames the proportion loop had already decided were worth drawing, a
// 90 fps stream into a chain holding a third of it drawing 430 frames against 541 over the same
// interval and reaching the same point in the stream.
const renderSink = "appsink name=" + sinkName +
	" emit-signals=true max-buffers=1 sync=true qos=true"

// initOnce ties initialization to the first pipeline, so the backend's start carries no
// init-order dependency on the first stream it receives.
var initOnce sync.Once

// initGStreamer brings up the library and pbutils, which names the codecs the receive state
// reports off the negotiated caps.
func initGStreamer() {
	initOnce.Do(func() {
		// Before Init: the registry scan Init runs reads the plugin path, and a path
		// set afterwards is one nothing rescans.
		// The thread-name handler goes first for the same reason: the scan starts
		// threads, and each one names itself.
		ignoreThreadNameExceptions()
		useBundledPlugins()

		gst.Init()
		initPbUtils()
	})
}
