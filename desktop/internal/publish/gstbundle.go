package publish

import (
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
)

// gstBundleDir is where a bundle keeps its GStreamer plugins: beside the binary
// rather than under lib/, the layout scripts/bundle-windows.sh writes and
// docs/packaging.md describes. The native grid reads the same directory for
// itself (nativegrid/internal/player/gstreamer/bundle_windows.go); this is the
// spelling for the children this app spawns.
const gstBundleDir = "gstreamer-1.0"

// gstPluginPathVar is the plugin path GStreamer scans in addition to its built-in
// one, which is what makes prepending to it additive rather than a replacement.
const gstPluginPathVar = "GST_PLUGIN_PATH"

// FindGstExe locates the GStreamer launcher every pipeline in this package is run
// by: the publish engine's, the test streams' and the encode probe's.
//
// It is one function rather than a lookup per caller because the three spawn the
// same binary, and a bundle is only reachable to a caller that resolves it the way
// FindExe does. A bare name handed to exec.Command searches PATH alone, so the
// copy a Windows bundle ships beside the app - the one built against the plugins
// beside it - would be passed over for whatever the machine happens to carry, or
// for nothing at all.
func FindGstExe() (string, error) {
	return ffmpeg.FindExe(GstExe)
}

// GstChildEnv is what a spawned GStreamer child is given on top of this process's
// environment, empty where there is nothing to add.
//
// A bundle carries its own GStreamer, built against a prefix that exists on no
// machine but the one that built it, so a child's registry scan reaches no plugin
// unless it is told where they went. An installation that is not a bundle has no
// such directory and is left to find its plugins the way it always does.
//
// The bundle goes in front of whatever the environment already names, so a child
// runs against the GStreamer it shipped with while a value set on purpose still
// applies. The app sets nothing on itself here: it links no GStreamer, and the
// variable belongs to the child that does.
func GstChildEnv() []string {
	self, err := os.Executable()
	if err != nil {
		logger.Warnf("cannot resolve the running binary, leaving %s to the GStreamer child: %v", gstPluginPathVar, err)
		return nil
	}

	dir := filepath.Join(filepath.Dir(self), gstBundleDir)
	if _, err := os.Stat(dir); err != nil {
		logger.Debugf("no plugin bundle at %s, the GStreamer child uses the installed plugins: %v", dir, err)
		return nil
	}

	path := dir
	if set := os.Getenv(gstPluginPathVar); set != "" {
		path += string(os.PathListSeparator) + set
	}
	return []string{gstPluginPathVar + "=" + path}
}
