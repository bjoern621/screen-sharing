// Command bench measures the native grid's render path: N pipelines from one
// relay stream, each into its own gtk4paintablesink shown in a GtkPicture, with
// the sink's rendered/dropped counters printed once a second.
//
// It exists to compare render chains against each other on real hardware; it is
// not part of the grid binary.
package main

/*
#cgo pkg-config: gobject-2.0
#include <stdlib.h>
#include <glib-object.h>

static gpointer grab_object_property(gpointer object, const char *name) {
	gpointer out = NULL;
	g_object_get(object, name, &out, NULL);
	return out;
}
*/
import "C"

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/go-gst/go-glib/pkg/gobject/v2"
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/screenshare-nativegrid/internal/player/gstreamer"
)

// shipped is the chain the grid actually plays. It is not spelled out here: the
// backend renders it, so a measurement of "shipped" cannot drift from what the
// tiles run.
const shipped = "shipped"

// benchQueue is the queue the shipped chain uses. The alternatives carry it too,
// so a comparison between two of them is a comparison of their conversions.
const benchQueue = "queue max-size-buffers=0 max-size-bytes=0 max-size-time=100000000"

// chains are the alternative render chains, appended to the source fragment. Each
// ends in a sink named "sink", which is where the counters are read.
var chains = map[string][]string{
	// The chain as it stood before the queue stopped leaking, so the cost of
	// leaking in front of a syncing sink stays measurable.
	"leaky": {
		"decodebin",
		"videoconvert n-threads=0",
		"video/x-raw,format=RGBA,colorimetry=sRGB",
		"queue max-size-buffers=3 leaky=downstream",
		"gtk4paintablesink name=sink",
	},
	// No conversion at all: the sink takes the decoder's YUV and GTK colour-manages.
	"raw": {
		"decodebin",
		benchQueue,
		"gtk4paintablesink name=sink",
	},
	// Convert on the GPU, hand the sink a GL texture.
	"gl": {
		"decodebin",
		"glupload",
		"glcolorconvert",
		"video/x-raw(memory:GLMemory),format=RGBA",
		benchQueue,
		"gtk4paintablesink name=sink",
	},
	// GL, plus a GPU downscale to tile size before the sink.
	"glscale": {
		"decodebin",
		"glupload",
		"glcolorconvert",
		"glcolorscale",
		"video/x-raw(memory:GLMemory),format=RGBA,width=640,height=360",
		benchQueue,
		"gtk4paintablesink name=sink",
	},
	// The VA postprocessor converts to RGBA on the GPU and hands the result back
	// as system memory, so only the conversion moves off the CPU.
	"vaconv": {
		"decodebin",
		"vapostproc",
		"video/x-raw,format=RGBA",
		benchQueue,
		"gtk4paintablesink name=sink",
	},
	// The VA postprocessor hands the sink a DMA-BUF the GTK renderer imports.
	"dmabuf": {
		"decodebin",
		"vapostproc",
		"video/x-raw(memory:DMABuf)",
		benchQueue,
		"gtk4paintablesink name=sink",
	},
	// CPU convert with a CPU downscale to a fixed tile size.
	// The shipped chain scales the same way, to whatever size its tile happens to have.
	"scale": {
		"decodebin",
		"videoscale n-threads=0",
		"video/x-raw,width=640,height=360",
		"videoconvert n-threads=0",
		"video/x-raw,format=RGBA,colorimetry=sRGB",
		benchQueue,
		"gtk4paintablesink name=sink",
	},
}

// fitName is the capsfilter the shipped chain scales into, which -fit drives from the widget's size.
// A chain that carries no such element is measured as it stands.
const fitName = "fit"

type tile struct {
	pipeline gst.Pipeline
	sink     gst.Element
	frames   atomic.Uint64
	// ticks counts GdkFrameClock frames on the widget, which is the ceiling any
	// render rate is measured against: an occluded or throttled window ticks
	// slower than the monitor.
	ticks atomic.Uint64
}

func main() {
	url := flag.String("url", "rtsp://127.0.0.1:8554/bench", "RTSP stream to receive")
	chain := flag.String("chain", shipped, "render chain: "+shipped+", "+strings.Join(keys(chains), ", "))
	count := flag.Int("streams", 4, "how many pipelines to run")
	secs := flag.Int("seconds", 20, "how long to run")
	sync := flag.Bool("sync", true, "sink syncs on the clock")
	qos := flag.Bool("qos", true, "sink sends QoS events upstream")
	lateness := flag.Int64("lateness", 5_000_000, "sink max-lateness in nanoseconds, -1 unlimited")
	fit := flag.Bool("fit", true, "bound the chain's scaler to the tile's size, where the chain has one")
	// The app sets both per watch leg, so they are flags here too: a measurement
	// taken with a different jitter buffer or RTP transport than the one in use
	// is a measurement of something else.
	latency := flag.Int("latency", 200, "rtspsrc jitter buffer in milliseconds")
	protocols := flag.String("protocols", "tcp", "RTP lower transport rtspsrc negotiates: tcp or udp")
	flag.Parse()

	source := fmt.Sprintf("rtspsrc location=%s protocols=%s latency=%d", *url, *protocols, *latency)
	var desc string
	switch elements, ok := chains[*chain]; {
	case *chain == shipped:
		desc = gstreamer.Describe(source, gstreamer.DefaultChain)
	case ok:
		desc = strings.Join(append([]string{source}, elements...), " ! ")
	default:
		fmt.Fprintf(os.Stderr, "unknown chain %q\n", *chain)
		os.Exit(2)
	}
	fmt.Printf("chain %s x%d sync=%t\n%s\n\n", *chain, *count, *sync, desc)

	gst.Init()

	app := gtk.NewApplication("de.bjoernblessin.NativeGridBench", gio.ApplicationNonUnique)
	app.ConnectActivate(func() {
		win := gtk.NewApplicationWindow(app)
		win.SetDefaultSize(1280, 720)
		grid := gtk.NewGrid()
		grid.SetRowHomogeneous(true)
		grid.SetColumnHomogeneous(true)
		win.SetChild(grid)

		cols := 1
		for cols*cols < *count {
			cols++
		}

		tiles := make([]*tile, 0, *count)
		for i := range *count {
			t, pic, err := start(desc, sinkOptions{
				sync: *sync, qos: *qos, lateness: *lateness, fit: *fit,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "stream %d: %v\n", i, err)
				os.Exit(1)
			}
			tiles = append(tiles, t)
			grid.Attach(pic, i%cols, i/cols, 1, 1)
		}

		win.SetVisible(true)
		go report(tiles, *secs, func() { coreglib.IdleAdd(func() { app.Quit() }) })
	})
	os.Exit(app.Run(os.Args[:1]))
}

// sinkOptions are the base-sink knobs the run varies.
type sinkOptions struct {
	sync     bool
	qos      bool
	lateness int64
	// fit writes the tile's pixel size into the chain's fit capsfilter, the way the grid keeps a
	// scaler on the size its tile draws.
	// Off, the same chain scales nothing and converts whole frames, which is the comparison.
	fit bool
}

// start builds one pipeline and the GtkPicture drawing its paintable.
func start(desc string, opts sinkOptions) (*tile, *gtk.Picture, error) {
	el, err := gst.ParseLaunch(desc)
	if err != nil {
		return nil, nil, err
	}
	pipeline, ok := el.(gst.Pipeline)
	if !ok {
		return nil, nil, fmt.Errorf("parse did not yield a pipeline")
	}
	sinkEl := pipeline.GetByName("sink")
	if sinkEl == nil {
		return nil, nil, fmt.Errorf("no sink in the pipeline")
	}
	sinkEl.SetObjectProperty("sync", opts.sync)
	sinkEl.SetObjectProperty("qos", opts.qos)
	sinkEl.SetObjectProperty("max-lateness", opts.lateness)

	var fitEl gst.Element
	if opts.fit {
		fitEl = pipeline.GetByName(fitName)
	}

	t := &tile{pipeline: pipeline, sink: sinkEl}
	paintable := paintableOf(sinkEl)
	paintable.Connect("invalidate-contents", func() { t.frames.Add(1) })

	pic := gtk.NewPicture()
	pic.SetPaintable(paintable)
	pic.SetContentFit(gtk.ContentFitContain)
	var lastW, lastH int
	pic.AddTickCallback(func(gtk.Widgetter, gdk.FrameClocker) bool {
		t.ticks.Add(1)
		if fitEl == nil {
			return true
		}
		// The tile's size has to arrive upstream as a caps bound.
		// The sink's own window-width and window-height do not carry it: the size they learn
		// leaves the sink as overlay-composition allocation metadata and never reaches the
		// caps, so the reconfigure event they can send renegotiates the source's size straight
		// back (measured on gtk4paintablesink 0.14.4: 1920x1080 in, 1920x1080 on the sink pad,
		// with reconfigure-on-window-resize=always and both sizes written every resize).
		scale := pic.ScaleFactor()
		w, h := pic.Width()*scale, pic.Height()*scale
		if w > 0 && h > 0 && (w != lastW || h != lastH) {
			lastW, lastH = w, h
			// The bound comes from the backend, so it carries the memory feature the chain
			// works in. Caps that name no feature pin the frames into system memory, which
			// on a device chain measures a download the chain itself does not do.
			fitEl.SetObjectProperty("caps",
				gst.CapsFromString(gstreamer.FitCaps(gstreamer.DefaultChain, w, h)))
		}
		return true
	})

	go func() {
		for msg := range pipeline.GetBus().Messages(context.Background()) {
			if msg.Type() == gst.MessageError {
				_, e := msg.ParseError()
				fmt.Fprintf(os.Stderr, "pipeline error: %v\n", e)
				return
			}
		}
	}()
	pipeline.SetState(gst.StatePlaying)
	return t, pic, nil
}

// report prints one line a second per tile, then a total, then quits.
func report(tiles []*tile, secs int, quit func()) {
	type prev struct{ frames, rendered, dropped, ticks uint64 }
	last := make([]prev, len(tiles))

	for tick := 1; tick <= secs; tick++ {
		time.Sleep(time.Second)
		var invalidations, rendered, dropped, ticks uint64
		for i, t := range tiles {
			f, k := t.frames.Load(), t.ticks.Load()
			r, d := counters(t.sink)
			invalidations += f - last[i].frames
			rendered += r - last[i].rendered
			dropped += d - last[i].dropped
			ticks += k - last[i].ticks
			last[i] = prev{f, r, d, k}
		}
		n := float64(len(tiles))
		fmt.Printf("t+%02ds  per-tile: %.1f drawn, %.1f sink-rendered, %.1f sink-dropped, %.1f frame-clock\n",
			tick, float64(invalidations)/n, float64(rendered)/n, float64(dropped)/n, float64(ticks)/n)
	}
	// The size the sink negotiated says whether a scaler upstream took the tile's
	// size or stayed on the source's.
	fmt.Printf("sink caps: %s\n", sinkCaps(tiles[0].sink))
	quit()
}

// sinkCaps renders what the sink negotiated on its input, and says so when it
// has negotiated nothing.
func sinkCaps(sink gst.Element) string {
	pad := sink.GetStaticPad("sink")
	if pad == nil {
		return "no sink pad"
	}
	caps := pad.GetCurrentCaps()
	if caps == nil || caps.GetSize() == 0 {
		return "not negotiated"
	}
	return caps.String()
}

// counters reads the sink's rendered and dropped totals.
func counters(sink gst.Element) (rendered, dropped uint64) {
	st, ok := sink.ObjectProperty("stats").(*gst.Structure)
	if !ok || st == nil {
		return 0, 0
	}
	r, _ := st.GetUint64("rendered")
	d, _ := st.GetUint64("dropped")
	return r, d
}

// paintableOf is the grid's own bridge between go-gst and gotk4.
func paintableOf(sink gst.Element) *gdk.Paintable {
	obj := gobject.UnsafeObjectToGlibNone(sink)
	name := C.CString("paintable")
	defer C.free(unsafe.Pointer(name))
	ptr := C.grab_object_property(C.gpointer(obj), name)
	return &gdk.Paintable{Object: coreglib.AssumeOwnership(unsafe.Pointer(ptr))}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
