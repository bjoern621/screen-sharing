package control

import "os"

// EnvInstance moves this backend to an endpoint of its own,
// so a build under development serves one shell while an installed build serves another.
// Unset is the address docs/ipc-api.md names, which every install answers on.
//
// The name lands in the endpoint's own leaf rather than in a directory above it:
// the Windows pipe has no directory, so one rule covers both platforms.
const EnvInstance = "MIRRORME_INSTANCE"

// instanceSuffix is what EnvInstance appends to the socket file name and the pipe name.
//
// The value travels verbatim: a repair here would bind an address the shell derives differently
// and cannot reach, so one carrying a path separator fails at the bind naming what it built.
func instanceSuffix() string {
	name := os.Getenv(EnvInstance)
	if name == "" {
		return ""
	}
	return "-" + name
}
