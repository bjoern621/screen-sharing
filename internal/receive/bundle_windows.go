package receive

import (
	"os"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/gstbundle"
)

// useBundledPlugins aims this process's GStreamer at the plugins shipped beside the binary.
//
// The variable goes on the running process and not on a child, which is what separates it from
// publish.GstChildEnv: this package links the library, so the registry scan that must reach the
// bundle is its own.
func useBundledPlugins() {
	path, ok := gstbundle.PluginPath()
	if !ok {
		return
	}
	if err := os.Setenv(gstbundle.PathVar, path); err != nil {
		logger.Warnf("cannot set %s to %s: %v", gstbundle.PathVar, path, err)
		return
	}
	logger.Infof("GStreamer plugins from the bundle at %s", path)
}
