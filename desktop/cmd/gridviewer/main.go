// Command gridviewer renders streams as a native GTK4 grid window.
//
// It decodes through decodebin backed by gst-libav, so it plays everything the
// gst-launch wall plays, including H.265 4:4:4 and RGB (RExt) streams no
// browser path decodes. Unlike the wall it composites in the GTK scene graph
// instead of a GStreamer compositor: each stream stays its own pipeline and
// widget, so tiles carry chrome and the layout can change without touching the
// pipelines.
//
// It is a separate binary because the Wails app links GTK3 through WebKitGTK,
// and GTK3 and GTK4 cannot share a process. The app spawns it like the wall
// and passes the stream list as one JSON argument (watch.GridConfig).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/screenshare/watch"
)

func main() {
	configJSON := flag.String("config", "", "watch.GridConfig as JSON")
	flag.Parse()

	var cfg watch.GridConfig
	if err := json.Unmarshal([]byte(*configJSON), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "bad -config: %v\n", err)
		os.Exit(2)
	}
	if len(cfg.Streams) == 0 {
		fmt.Fprintln(os.Stderr, "empty stream list")
		os.Exit(2)
	}

	gst.Init()

	// NonUnique: a second viewer must not activate inside the first one's
	// process; the app replaces the viewer by killing and respawning it.
	app := gtk.NewApplication("de.bjoernblessin.screenshare.gridviewer", gio.ApplicationNonUnique)

	var v *viewer
	app.ConnectActivate(func() {
		v = newViewer(app, cfg)
	})

	code := app.Run(os.Args[:1])
	if v != nil {
		v.stop()
	}
	os.Exit(code)
}
