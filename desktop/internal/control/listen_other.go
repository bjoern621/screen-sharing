//go:build !windows

package control

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// The endpoint docs/ipc-api.md names for Linux and macOS, and the modes it names with
// it.
//
// The contract version is in the file name for the reason the Windows pipe carries it
// in its own: a v2 is a second socket, so two backends on different majors can run side
// by side and a shell that opens the wrong one fails to connect rather than connecting
// and being turned away at Hello.
const (
	socketDirName  = "screenshare"
	socketFileName = "control-v1.sock"

	socketDirMode  fs.FileMode = 0o700
	socketFileMode fs.FileMode = 0o600
)

// staleDialTimeout bounds the one connect that separates a socket somebody is
// listening on from one a crashed process left behind.
//
// It is short because both answers are local and immediate: a live listener accepts at
// once and an abandoned socket refuses at once. Anything slower is a machine under
// load, and waiting longer for it would hold up the backend's startup for a case that
// is not the one being tested.
const staleDialTimeout = 250 * time.Millisecond

// Listen opens the Unix socket this backend serves the control contract on.
func Listen() (net.Listener, error) {
	path, err := socketPath()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, socketDirMode); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", dir, err)
	}
	// MkdirAll leaves an existing directory's mode alone, so the mode is set rather than
	// assumed. The directory is what actually keeps another user out during the instant
	// between the bind below and the chmod after it, and a directory left by an older
	// build with a wider mode would leave that instant open.
	if err := os.Chmod(dir, socketDirMode); err != nil {
		return nil, fmt.Errorf("cannot restrict %s to this user: %w", dir, err)
	}

	if err := clearStale(path); err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", path, err)
	}

	// The socket file is created with the process umask applied to 0777, which on a
	// default umask is 0755 and on a permissive one is wider. This is what makes the file
	// itself say who may connect; the 0700 directory above is what covers the moment
	// before it.
	if err := os.Chmod(path, socketFileMode); err != nil {
		listener.Close()
		return nil, fmt.Errorf("cannot restrict %s to this user: %w", path, err)
	}

	// A socket file outlives the process that bound it, so the unlink is stated rather
	// than inherited. The default depends on how the listener was created, and what is
	// wanted here is unconditional: a clean shutdown should leave nothing behind for the
	// next start to have to recognise as stale.
	unix, ok := listener.(*net.UnixListener)
	assert.Assert(ok, "listening on a unix socket yields a unix listener")
	unix.SetUnlinkOnClose(true)

	return unix, nil
}

// Endpoint names the address this platform serves on, for the backend's log and for a
// shell's "the backend is not running" message.
//
// A path that cannot be resolved comes back as the reason rather than as an empty
// string. The one use for this value is a sentence a person reads, and an empty address
// reads as an address of nothing rather than as the absence of one.
func Endpoint() string {
	path, err := socketPath()
	if err != nil {
		return "(no control socket path: " + err.Error() + ")"
	}
	return path
}

// socketPath places the socket in the runtime directory, falling back to the user's
// config directory where there is none.
//
// XDG_RUNTIME_DIR is the right home for it: it is per user, mode 0700 already, and it
// is cleared when the session ends, so a socket left there by a crash does not survive
// a logout. It is also unset on macOS and on any login that never had a session bus,
// which is what the fallback is for. The config directory is the wrong kind of place
// for a socket - it survives reboots and it gets backed up - and that is exactly why
// clearStale exists rather than being a nicety.
func socketPath() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("cannot place the control socket: XDG_RUNTIME_DIR is unset and there is no config directory: %w", err)
		}
		dir = config
	}

	return filepath.Join(dir, socketDirName, socketFileName), nil
}

// clearStale removes a socket a dead process left behind, and refuses to touch one
// another backend is still listening on.
//
// The two are not distinguishable by looking. A bound socket and an abandoned one are
// the same directory entry, because the entry outlives the process and nothing unlinks
// it when that process is killed. Connecting is what tells them apart - a live listener
// accepts, an abandoned socket refuses at once - so that is what is done, and it is
// done before the bind because net.Listen on an existing path fails rather than
// replacing it.
//
// Anything at the path that is not a socket is left alone. It is not this service's
// file whatever it is, and a start that deleted it would be a start that destroys data
// to make room for itself.
func clearStale(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot examine %s: %w", path, err)
	}
	if info.Mode()&fs.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket, so the control service has nowhere to listen", path)
	}

	conn, err := net.DialTimeout("unix", path, staleDialTimeout)
	if err == nil {
		conn.Close()
		return fmt.Errorf("another backend is already listening on %s", path)
	}

	logger.Warnf("control: removing the socket a previous run left at %s", path)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cannot remove the stale socket %s: %w", path, err)
	}
	return nil
}
