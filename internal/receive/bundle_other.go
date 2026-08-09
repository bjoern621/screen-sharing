//go:build !windows

package receive

// useBundledPlugins does nothing off Windows. Every other channel installs
// GStreamer as a dependency rather than bundling it (docs/packaging.md), so the
// plugins are already on the path the library scans, and a build that wants
// another set says so in the environment.
func useBundledPlugins() {}
