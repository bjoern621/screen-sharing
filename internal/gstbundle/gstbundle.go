// Package gstbundle locates the GStreamer plugins a bundle ships beside the binary.
//
// A bundle carries its own GStreamer, built against a prefix that exists on no machine but the one
// that built it, so a registry scan reaches no plugin unless it is told where they went
// (docs/packaging.md, "Windows").
// An installation that is not a bundle has no such directory and finds its plugins the way it
// always does.
//
// Two callers need that answer for two different processes: internal/receive links GStreamer and
// sets the path on itself before it initializes the library, and internal/publish spawns
// gst-launch-1.0 and hands the path to the child.
// One directory spelled in both is the drift this package exists to prevent.
package gstbundle

import (
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Dir is where a bundle keeps its plugins: beside the binary rather than under lib/,
// which is the layout scripts/bundle-windows.sh writes.
const Dir = "gstreamer-1.0"

// PathVar is the plugin path GStreamer scans in addition to its built-in one,
// which is what makes prepending to it additive rather than a replacement.
const PathVar = "GST_PLUGIN_PATH"

// PluginPath is the value PathVar takes for a process running against the bundle,
// and false where this installation is not one.
//
// The bundle goes in front of whatever the environment already names, so a run uses the GStreamer
// it shipped with while a value set on purpose still applies.
//
// Neither failure is this app's: a binary whose own path cannot be resolved and a directory that is
// not there are both Umgebungsfehler, and both answer false rather than panicking,
// because an ordinary installation takes the second branch every time.
func PluginPath() (string, bool) {
	self, err := os.Executable()
	if err != nil {
		logger.Warnf("cannot resolve the running binary, leaving %s alone: %v", PathVar, err)
		return "", false
	}

	dir := filepath.Join(filepath.Dir(self), Dir)
	if _, err := os.Stat(dir); err != nil {
		logger.Debugf("no plugin bundle at %s, using the installed GStreamer: %v", dir, err)
		return "", false
	}

	path := dir
	if set := os.Getenv(PathVar); set != "" {
		path += string(os.PathListSeparator) + set
	}

	assert.Assert(filepath.IsAbs(dir), "a bundle's plugin directory is an absolute path", dir)
	return path, true
}
