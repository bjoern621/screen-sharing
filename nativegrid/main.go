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
// streams appear. A sidebar row can move its stream to another watch leg or retune
// the one it is on; the window asks the app for that on stdout and receives the
// answer as the next push. The sidebar's foot asks the same way about the app
// itself: its window to the front, its publish on or off. The same pipe carries
// which streams have a tile open, so the app can show what this window took.
// Without -config, built-in videotestsrc streams drive a standalone demo run.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare-nativegrid/internal/idle"
	"bjoernblessin.de/screenshare-nativegrid/internal/layout"
	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/player/gstreamer"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
	"bjoernblessin.de/screenshare-nativegrid/internal/session"
	"bjoernblessin.de/screenshare-nativegrid/internal/threadname"
	"bjoernblessin.de/screenshare-nativegrid/internal/ui/window"
)

// appID is the GTK application id, which the desktop shell keys the window on.
const appID = "de.bjoernblessin.ScreenshareNativeGrid"

// exitBadFlag is what a flag naming something that does not exist leaves behind,
// the status a shell reads as a wrong invocation.
const exitBadFlag = 2

func main() {
	// Before any element exists, because the exception it disarms is raised while
	// one is being built and a handler that arrives afterwards missed it.
	threadname.Ignore()

	configArg := flag.String("config", "", "stream list as JSON; empty runs the demo streams")
	backendArg := flag.String("player", gstreamer.Backend, "decode backend: "+strings.Join(player.Names(), ", "))
	flag.Parse()

	// A run without -config has no app on the other end of stdin and stdout.
	fromApp := *configArg != ""

	cfg := config(*configArg)
	factory, err := player.For(*backendArg)
	if err != nil {
		// The flag is typed by hand, so a name no backend registered under is the
		// run's input and not a bug here. The process stops on it, instead of
		// carrying a nil factory into the model one call later and failing there
		// under the name of a missing decoder.
		//
		// The reason goes to stderr rather than through the logger: logger.Errorf
		// exits with status 1, which would cost the caller the wrong-invocation
		// status, and a log level below WARN would drop the line the caller reads.
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(exitBadFlag)
	}

	// The UI loop is the one thread the model and the views run on. Player callbacks
	// arrive on pipeline threads and hop here; so do the relayouts a drag must not
	// perform from inside its own callback.
	dispatch := idle.Dispatch(func(f func()) { coreglib.IdleAdd(f) })
	// The chains are the backend's to answer for, and it answers off the element
	// registry of this machine, so the model asks rather than being handed a list. The
	// name is the one the factory came from, which the lookup above made sure exists.
	chains := func() []player.Chain { return player.Chains(*backendArg) }
	sess := session.New(cfg, factory, chains, layout.NewFileStore(), sender(fromApp), reporter(fromApp), runner(fromApp), dispatch)

	// NonUnique: a second grid must not activate inside the first one's process; the
	// app replaces the window by killing and respawning it.
	app := adw.NewApplication(appID, gio.ApplicationNonUnique)
	app.ConnectActivate(func() {
		window.New(app, sess, dispatch).SetVisible(true)
		// Only an app-driven run is pushed rosters; the demo's stdin is a terminal,
		// not a stream of configs.
		if fromApp {
			go roster.Pushes(os.Stdin, func(push roster.Config) {
				dispatch(func() {
					// The app state goes in first: both halves of the push are
					// notified separately, and a view redrawing on the roster
					// would otherwise draw this push's streams beside the app
					// state of the one before it.
					sess.SetApp(push.App)
					sess.SetRoster(push.Streams)
				})
			})
		}
	})

	// GTK sees only argv[0]; the flags above are not its business.
	code := app.Run(os.Args[:1])
	sess.Close()
	os.Exit(code)
}

// sender is where a watch-leg request goes: the app reads stdout, the demo run
// has nobody to answer one. The demo streams declare no watch legs to move
// between, so a discarded request is a request the sidebar cannot make.
func sender(fromApp bool) roster.Send {
	if !fromApp {
		return roster.Discard
	}
	return roster.Sender(os.Stdout)
}

// reporter is where the watch set goes: the app reads stdout and shows what this window took,
// the demo run has nobody to tell.
func reporter(fromApp bool) roster.Report {
	if !fromApp {
		return roster.DiscardReport
	}
	return roster.Reporter(os.Stdout)
}

// runner is where a command goes: the app runs it and pushes what it changed.
// The demo config carries no app state, so the sidebar draws no control that
// could send one.
func runner(fromApp bool) roster.Run {
	if !fromApp {
		return roster.DiscardCommand
	}
	return roster.Runner(os.Stdout)
}

// config is the roster the window opens on: the app's, or the demo one.
//
// The argument is the run's input, whether the app that spawns this process
// wrote it (desktop/watch.BuildGridConfig) or a person typed it, so a roster
// that does not parse ends the process the way an unknown -player does. The
// exit status and the stderr line are what the spawning app reads; a panic
// would bury both under this side's stack.
func config(raw string) roster.Config {
	if raw == "" {
		logger.Infof("no -config given, running the demo streams")
		return roster.DemoConfig()
	}
	cfg, err := roster.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad -config: %v\n", err)
		os.Exit(exitBadFlag)
	}
	return cfg
}
