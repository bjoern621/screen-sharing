package gstreamer

import (
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/logger"
)

// bundleDir is where a Windows bundle keeps its GStreamer plugins: beside the
// binary rather than under lib/, so the directory is the one thing the bundle
// layout has to get right (docs/packaging.md, "Windows").
const bundleDir = "gstreamer-1.0"

// pluginPathVar is the plugin path GStreamer scans in addition to its built-in
// one, which is what makes prepending to it additive rather than a replacement.
const pluginPathVar = "GST_PLUGIN_PATH"

// useBundledPlugins points GStreamer at the plugins shipped beside the binary.
//
// A Windows bundle carries its own GStreamer, built against an MSYS2 prefix that
// does not exist on the machine running it, so the registry scan reaches nothing
// unless it is told where the plugins went. An installation that is not a bundle
// has no such directory and is left to find its plugins the way it always does.
//
// The bundle goes in front of whatever the environment already names, so the
// grid runs against the GStreamer it was bundled with while a value set on
// purpose still applies.
func useBundledPlugins() {
	exe, err := os.Executable()
	if err != nil {
		logger.Warnf("cannot resolve the running binary, leaving %s alone: %v", pluginPathVar, err)
		return
	}

	dir := filepath.Join(filepath.Dir(exe), bundleDir)
	if _, err := os.Stat(dir); err != nil {
		logger.Debugf("no plugin bundle at %s, using the installed GStreamer: %v", dir, err)
		return
	}

	path := dir
	if set := os.Getenv(pluginPathVar); set != "" {
		path += string(os.PathListSeparator) + set
	}
	if err := os.Setenv(pluginPathVar, path); err != nil {
		logger.Warnf("cannot set %s to %s: %v", pluginPathVar, path, err)
		return
	}
	logger.Infof("GStreamer plugins from the bundle at %s", dir)
}
