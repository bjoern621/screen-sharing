package publish

import (
	"os"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gstbundle"
)

// FindGstExe locates the GStreamer launcher the measuring pipelines here spawn: the test streams'
// and the encode probe's.
//
// A bundled launcher is only reachable through FindExe's resolution.
// A bare name handed to exec.Command searches PATH alone, which passes over the copy a Windows
// bundle ships beside the app, the one built against the plugins beside it.
//
// A publish is not among them.
// It plays in a process of this app's own (FindGstRunner), where a measuring run wants a pipeline
// played and a count read off it, all gst-launch does.
func FindGstExe() (string, error) {
	return ffmpeg.FindExe(GstExe)
}

// GstSubcommand is the first argument of every child spawned to play a publish pipeline, and what
// this binary dispatches on when it sees it (cmd/backend).
const GstSubcommand = "gst-publish"

// FindGstRunner returns the executable a publish pipeline plays in: this one.
//
// Spawning the binary that is already running keeps the pipeline in a process of its own without
// a second artifact to build, sign, ship and find at runtime.
// The isolation is the launcher's: a driver that faults takes the child and leaves the backend,
// the control socket and every viewer.
// What it adds is a pipeline this application can read from and talk to (internal/gstrun).
func FindGstRunner() (string, error) {
	return os.Executable()
}

// GstChildEnv is what a spawned GStreamer child gets on top of this process's environment,
// empty where there is nothing to add.
//
// gstbundle answers where the plugins are, internal/receive asking that of the same installation.
// Nothing is set on this process: a publish pipeline links no GStreamer, so the variable belongs
// to the child that does.
func GstChildEnv() []string {
	path, ok := gstbundle.PluginPath()
	if !ok {
		return nil
	}
	return []string{gstbundle.PathVar + "=" + path}
}
