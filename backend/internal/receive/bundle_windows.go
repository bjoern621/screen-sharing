package receive

import (
	"os"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/gstbundle"
)

// useBundledPlugins aims this process's GStreamer and GLib at what ships beside the binary:
// the plugins, and the GIO module every rtsps:// leg takes its TLS from.
//
// The variables go on the running process and not on a child,
// which separates it from publish.GstChildEnv:
// this package links the library, so the registry scan and the module scan that must reach the
// bundle are its own.
// gstbundle.Env is the table both read, so what this process is told is what a child is told.
func useBundledPlugins() {
	for _, entry := range gstbundle.Env() {
		variable, value, ok := strings.Cut(entry, "=")
		assert.Assert(ok && variable != "", "a bundle variable is spelled VAR=value", entry)

		if err := os.Setenv(variable, value); err != nil {
			logger.Warnf("cannot set %s to %s: %v", variable, value, err)
			continue
		}
		logger.Infof("%s from the bundle: %s", variable, value)
	}
}
