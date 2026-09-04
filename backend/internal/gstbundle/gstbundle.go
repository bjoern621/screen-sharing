// Package gstbundle locates what a bundle ships beside the binary for the GStreamer it carries:
// the plugins, and the GIO module that gives GLib its TLS.
//
// A bundle carries its own GStreamer, built against a prefix that exists only on the build machine,
// so a registry scan reaches no plugin unless told where they went (docs/packaging.md, "Windows").
// The plugin directory holds every element a publish or receive pipeline names, one transport's
// source or sink at a time, and a plugin left out of it fails as `no element "..."` the first time
// that leg starts rather than at build.
// The module directory holds glib-networking's gnutls module, and GLib carries no TLS without it:
// every rtsps:// leg then fails at the connect as "Failed to connect. (Generic error)", naming TLS
// nowhere.
// An installation that is no bundle has neither directory and resolves against the installed
// GStreamer and GLib.
//
// Three processes need the answer: internal/receive sets the variables on itself before
// initializing the library, internal/decode hands them to the host it spawns, and internal/publish
// to every gst-launch-1.0 and gst-inspect-1.0 child.
// One directory spelled in each is the drift this package prevents, so Env is the whole table and
// every consumer reads it rather than a variable of its own.
package gstbundle

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Dir is where a bundle keeps its plugins, beside the binary rather than under lib/,
// the layout packaging/windows/bundle-runtime.sh writes.
const Dir = "gstreamer-1.0"

// PathVar is the plugin path GStreamer scans on top of its built-in one, so prepending to it adds
// rather than replaces.
const PathVar = "GST_PLUGIN_PATH"

// ModuleDir is where a bundle keeps the GIO TLS module, beside the binary as the plugins are.
//
// Not the lib/gio/modules GIO would scan unaided: on Windows GIO derives that directory from where
// its own DLL sits, and strips a trailing "bin" or "lib" from that path first, so the same layout
// loads under one directory name and silently does not under another, build/bin among them.
// A directory GIO finds only by being told keeps one mechanism, ModuleVar, on every layout.
const ModuleDir = "gio-modules"

// ModuleVar names the directories GIO loads modules from on top of its built-in one,
// so prepending to it adds rather than replaces, as PathVar does.
// The Nix package prefixes the same variable for the same module (packaging/nix/package.nix).
const ModuleVar = "GIO_EXTRA_MODULES"

// PluginPath is the value PathVar takes for a process running against the bundle, false where this
// installation is not one.
//
// The bundle leads whatever the environment already names, so a run uses the GStreamer it shipped
// with while a value set on purpose still applies.
//
// An unresolvable binary path and a missing directory are both Umgebungsfehler and answer false
// rather than panicking: an ordinary installation takes the second branch every time.
func PluginPath() (string, bool) {
	return bundled(Dir, PathVar)
}

// ModulePath is the value ModuleVar takes for a process running against the bundle, false where
// this installation ships no module directory, on PluginPath's terms.
func ModulePath() (string, bool) {
	return bundled(ModuleDir, ModuleVar)
}

// Env is every variable a process running against the bundle carries, as VAR=value, empty for an
// installation that is no bundle.
// The one table each consumer reads: this process through os.Setenv, a child through its command's
// environment.
func Env() []string {
	var env []string
	if path, ok := PluginPath(); ok {
		env = append(env, PathVar+"="+path)
	}
	if path, ok := ModulePath(); ok {
		env = append(env, ModuleVar+"="+path)
	}
	return env
}

// bundled answers the value variable takes for the directory named beside the binary, false where
// the binary cannot be resolved or the directory is not there.
func bundled(name, variable string) (string, bool) {
	assert.Assert(name != "", "a bundle directory has a name")
	assert.Assert(variable != "", "a bundle directory is named to a process through a variable")

	self, err := os.Executable()
	if err != nil {
		logger.Warnf("cannot resolve the running binary, leaving %s alone: %v", variable, err)
		return "", false
	}

	dir := filepath.Join(filepath.Dir(self), name)
	if _, err := os.Stat(dir); err != nil {
		logger.Debugf("no bundle directory at %s, leaving %s to the installation: %v", dir, variable, err)
		return "", false
	}

	assert.Assert(filepath.IsAbs(dir), "a bundle directory is an absolute path", dir)
	return leading(dir, os.Getenv(variable)), true
}

// leading is set with dir leading it, or set as it is where dir is already on it.
//
// Idempotent, because the answer travels: internal/receive writes it onto this process,
// internal/decode hands it to a child that asks again against an environment already carrying it,
// and a child of that child asks once more.
// Without the check each level put the directory on a second time, and a log named the bundle
// three times over.
// Entries are compared as written, the directory being spelled by one os.Executable each time.
func leading(dir, set string) string {
	assert.Assert(dir != "", "a bundle's list leads with its own directory")

	if set == "" {
		return dir
	}
	if slices.Contains(strings.Split(set, string(os.PathListSeparator)), dir) {
		return set
	}
	return dir + string(os.PathListSeparator) + set
}
