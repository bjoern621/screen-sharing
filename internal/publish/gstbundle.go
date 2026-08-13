package publish

import (
	"os"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gstbundle"
)

// FindGstExe locates the GStreamer launcher the measuring pipelines in this package are run by:
// the test streams' and the encode probe's.
//
// It is one function rather than a lookup per caller because they spawn the same binary,
// and a bundle is only reachable to a caller that resolves it the way FindExe does.
// A bare name handed to exec.Command searches PATH alone, so the copy a Windows bundle ships beside
// the app - the one built against the plugins beside it - would be passed over for whatever the
// machine happens to carry, or for nothing at all.
//
// A publish is not among them.
// It runs in a process of this app's own (FindGstRunner), where the measuring runs stay on the
// launcher: what they need is a pipeline played and a count read off it, which is exactly what
// gst-launch does and all it does.
func FindGstExe() (string, error) {
	return ffmpeg.FindExe(GstExe)
}

// GstSubcommand is the argument this application answers to when it is spawned to play a publish
// pipeline, and the first argument of every such child (cmd/backend).
const GstSubcommand = "gst-publish"

// FindGstRunner is the executable a publish pipeline is played in: this one.
//
// Spawning the binary that is already running is what keeps the pipeline in a process of its own
// without adding a second artifact to build, sign, ship and find at runtime.
// The isolation is the same one gst-launch gave - a driver that faults takes the child and leaves
// the backend, the control socket and every viewer - and what it buys over the launcher is a
// pipeline this application can read from and, in time, talk to (internal/gstrun).
func FindGstRunner() (string, error) {
	return os.Executable()
}

// GstChildEnv is what a spawned GStreamer child is given on top of this process's environment,
// empty where there is nothing to add.
//
// Where the plugins are is gstbundle's answer, because internal/receive asks the same question of
// the same installation.
// What differs is who is told: this app sets nothing on itself here, since a publish pipeline links
// no GStreamer and the variable belongs to the child that does.
func GstChildEnv() []string {
	path, ok := gstbundle.PluginPath()
	if !ok {
		return nil
	}
	return []string{gstbundle.PathVar + "=" + path}
}
