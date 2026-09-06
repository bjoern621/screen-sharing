//go:build !windows

package discordrpc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// dial opens the socket a running Discord client serves.
//
// The client numbers its socket, one per instance running,
// so a second Discord on the same login answers on the next number up.
// The directory is the session's runtime one,
// and a sandboxed install places it under a prefix of its own:
// Flatpak under the application id, snap under the package name.
// macOS has no runtime directory and puts the socket in TMPDIR, which is the fallback below.
func dial() (net.Conn, error) {
	for _, dir := range socketDirs() {
		for i := range socketsPerHost {
			path := filepath.Join(dir, fmt.Sprintf("discord-ipc-%d", i))
			conn, err := net.DialTimeout("unix", path, dialTimeout)
			if err == nil {
				return conn, nil
			}
		}
	}
	return nil, errors.New(noClientRunning)
}

// socketDirs is where a Discord client places its socket, in the order they are tried.
func socketDirs() []string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.Getenv("TMPDIR")
	}
	if base == "" {
		base = "/tmp"
	}

	return []string{
		base,
		filepath.Join(base, "app", "com.discordapp.Discord"),
		filepath.Join(base, "snap.discord"),
	}
}
