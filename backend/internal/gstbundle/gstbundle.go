// Package gstbundle locates the GStreamer plugins a bundle ships beside the binary.
//
// A bundle carries its own GStreamer, built against a prefix that exists only on the build machine,
// so a registry scan reaches no plugin unless told where they went (docs/packaging.md, "Windows").
// The directory holds every element a publish or receive pipeline names, one transport's source or
// sink at a time, and a plugin left out of it fails as `no element "..."` the first time that leg
// starts rather than at build.
// An installation that is no bundle has no such directory and resolves against the installed
// GStreamer.
//
// Two processes need the answer: internal/receive sets the path on itself before initializing
// the library, and internal/publish hands it to the gst-launch-1.0 child it spawns.
// One directory spelled in both is the drift this package prevents.
package gstbundle

import (
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Dir is where a bundle keeps its plugins, beside the binary rather than under lib/,
// the layout scripts/bundle-windows.sh writes.
const Dir = "gstreamer-1.0"

// PathVar is the plugin path GStreamer scans on top of its built-in one, so prepending to it adds
// rather than replaces.
const PathVar = "GST_PLUGIN_PATH"

// PluginPath is the value PathVar takes for a process running against the bundle, false where this
// installation is not one.
//
// The bundle leads whatever the environment already names, so a run uses the GStreamer it shipped
// with while a value set on purpose still applies.
//
// An unresolvable binary path and a missing directory are both Umgebungsfehler and answer false
// rather than panicking: an ordinary installation takes the second branch every time.
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
