// Command screenshare-nativegrid is the native stream grid: an Adwaita window with
// a retractable stream sidebar and one video tile per watched stream.
//
// Checking a sidebar row watches that stream: the row's status dot becomes a
// spinner, the tile shows the loading skeleton, and both go live with the first
// decoded frame. A failed pipeline shows the message on the tile with a retry
// button. Dragging a tile reorders the grid with a live preview, and dragging a
// sidebar row does the same from the list side. Hovering a tile fades in the web
// grid's media controls: mute and volume once the stream carries audio, the stats
// overlay, spotlight, disconnect.
//
// The window is one model and two views of it: internal/session decides what is
// watched, in what order, and which stream is spotlit, and remembers it across runs;
// internal/ui draws it. Decoding sits behind internal/player, so the grid names no
// media framework. Styling follows docs/design-language.md, with the web app's Tabler
// icons.
//
// The stream list arrives as -config JSON from the app and stays current through
// roster pushes on stdin, so the window can open on an idle relay and fill up as
// streams appear. Without -config, built-in videotestsrc streams drive a standalone
// demo run.
package main

import (
	"flag"
	"os"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/player/gstreamer"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/window"
)

// appID is the GTK application id, which the desktop shell keys the window on.
const appID = "de.bjoernblessin.ScreenshareNativeGrid"

func main() {
	configArg := flag.String("config", "", "stream list as JSON; empty runs the demo streams")
	backendArg := flag.String("player", gstreamer.Backend, "decode backend: "+strings.Join(player.Names(), ", "))
	flag.Parse()

	cfg := config(*configArg)
	factory, err := player.For(*backendArg)
	if err != nil {
		logger.Errorf("%v", err)
	}

	// The UI loop is the one thread the model and the views run on. Player callbacks
	// arrive on pipeline threads and hop here; so do the relayouts a drag must not
	// perform from inside its own callback.
	dispatch := idle.Dispatch(func(f func()) { coreglib.IdleAdd(f) })
	sess := session.New(cfg.Streams, factory, layout.NewFileStore(), dispatch)

	// NonUnique: a second grid must not activate inside the first one's process; the
	// app replaces the window by killing and respawning it.
	app := adw.NewApplication(appID, gio.ApplicationNonUnique)
	app.ConnectActivate(func() {
		window.New(app, sess, dispatch).SetVisible(true)
		// Only an app-driven run is pushed rosters; the demo's stdin is a terminal,
		// not a stream of configs.
		if *configArg != "" {
			go roster.Pushes(os.Stdin, func(streams []roster.Stream) {
				dispatch(func() { sess.SetRoster(streams) })
			})
		}
	})

	// GTK sees only argv[0]; the flags above are not its business.
	code := app.Run(os.Args[:1])
	sess.Close()
	os.Exit(code)
}

// config is the roster the window opens on: the app's, or the demo one.
func config(raw string) roster.Config {
	if raw == "" {
		logger.Infof("no -config given, running the demo streams")
		return roster.DemoConfig()
	}
	cfg, err := roster.Parse(raw)
	if err != nil {
		logger.Errorf("%v", err)
		assert.Never()
	}
	return cfg
}
