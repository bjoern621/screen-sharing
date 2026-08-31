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

// The endpoint and modes docs/ipc-api.md names for Linux and macOS.
//
// The contract version sits in the file name, as the Windows pipe carries it in its own:
// a v2 is a second socket, so two backends on different majors run side by side,
// and a shell opening the wrong one fails to connect rather than being turned away at Hello.
const (
	socketDirName  = "screenshare"
	socketFileName = "control-v1.sock"

	socketDirMode  fs.FileMode = 0o700
	socketFileMode fs.FileMode = 0o600
)

// staleDialTimeout bounds the connect separating a live listener from a socket a crashed process
// left behind.
//
// Short because both answers are local and immediate:
// a live listener accepts at once, an abandoned socket refuses at once.
// Anything slower is a machine under load,
// and waiting on it would hold up startup for a case that is not the one being tested.
const staleDialTimeout = 250 * time.Millisecond

// Listen opens the Unix socket this backend serves the control contract on.
// Every failure is the environment's and travels as an error,
// ErrAddressInUse wrapped where the address is another backend's.
func Listen() (net.Listener, error) {
	path, err := socketPath()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, socketDirMode); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", dir, err)
	}
	// MkdirAll leaves an existing directory's mode alone, so the mode is set rather than assumed.
	// The directory keeps another user out between the bind below and the chmod after it,
	// and a directory already there with a wider mode would leave that instant open.
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

	// The bind creates the file with the process umask applied to 0777: 0755 under a default umask,
	// wider under a permissive one.
	// The chmod makes the file itself say who may connect,
	// the 0700 directory above covering the moment before.
	if err := os.Chmod(path, socketFileMode); err != nil {
		listener.Close()
		return nil, fmt.Errorf("cannot restrict %s to this user: %w", path, err)
	}

	// A socket file outlives the process that bound it, so the unlink is set rather than inherited:
	// the default turns on how the listener was created, and this has to hold either way.
	// A clean shutdown then leaves nothing for the next start to recognise as stale.
	unix, ok := listener.(*net.UnixListener)
	assert.Assert(ok, "listening on a unix socket yields a unix listener")
	unix.SetUnlinkOnClose(true)

	return unix, nil
}

// Endpoint names the address this platform serves on,
// for the backend's log and for a shell's "the backend is not running" message.
//
// A path that cannot be resolved comes back as the reason, never as an empty string:
// the value is only ever read in a sentence a person reads,
// where an empty address reads as an address of nothing rather than as the absence of one.
func Endpoint() string {
	path, err := socketPath()
	if err != nil {
		return "(no control socket path: " + err.Error() + ")"
	}
	return path
}

// socketPath places the socket in the runtime directory,
// falling back to the user's config directory where there is none.
//
// XDG_RUNTIME_DIR is per user, mode 0700 already, and cleared when the session ends,
// so a socket a crash left there does not survive a logout.
// It is unset on macOS and on any login that never had a session bus, hence the fallback.
// The config directory survives reboots and gets backed up,
// so clearStale is required rather than a nicety.
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

// clearStale removes a socket a dead process left behind,
// and refuses to touch one another backend is listening on.
//
// Looking does not tell the two apart: a bound socket and an abandoned one are the same directory
// entry, the entry outliving the process and nothing unlinking it when that process is killed.
// Connecting does, a live listener accepting and an abandoned socket refusing at once.
// It runs before the bind because net.Listen on an existing path fails rather than replacing it.
//
// Anything at the path that is not a socket is left alone: not this service's file,
// and a start that deleted it would destroy data to make room for itself.
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
		return fmt.Errorf("%w: %s", ErrAddressInUse, path)
	}

	logger.Warnf("control: removing the socket a previous run left at %s", path)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cannot remove the stale socket %s: %w", path, err)
	}
	return nil
}
