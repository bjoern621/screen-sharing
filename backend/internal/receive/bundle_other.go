//go:build !windows

package receive

// useBundledPlugins has nothing to do off Windows. GStreamer arrives as a dependency on every other
// channel rather than bundled (docs/packaging.md), so its plugins already sit on the path the
// library scans, and a build wanting a different set states that in the environment.
func useBundledPlugins() {}
