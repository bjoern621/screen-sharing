package decode

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/gstbundle"
)

// Bringing the host up.
// Taking it down is shutdown.go.

// dialTimeout bounds the wait for a freshly spawned child to bind its socket.
//
// Generous rather than tight: the child loads the GStreamer registry before it listens, and
// a machine building that registry from scratch takes seconds over one that found it cached.
// A child that misses it is an Umgebungsfehler the caller surfaces.
const dialTimeout = 30 * time.Second

// dialInterval is how often the socket is tried while the child comes up.
const dialInterval = 20 * time.Millisecond

// process is the child and the channel its reaping closes.
// The goroutine that spawned it owns the Wait, so a shutdown watches for the exit through
// the channel rather than reaping it a second time.
type process struct {
	cmd    *exec.Cmd
	exited chan struct{}
}

// socketRoot is where the control socket is created.
//
// The session's runtime directory suits one: cleaned out when the session ends, and on a filesystem
// that never persists.
// The temporary directory stands in for a session with none.
func socketRoot() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return os.TempDir()
}

// ensure brings the host up where none runs.
func (c *Client) ensure() error {
	c.mu.Lock()
	closed, running := c.closed, c.ctrl != nil
	c.mu.Unlock()

	if closed {
		return errShutDown
	}
	if running {
		return nil
	}

	c.spawnMu.Lock()
	defer c.spawnMu.Unlock()

	// The question again under the spawn lock, two callers racing an absent host otherwise
	// spawning two.
	c.mu.Lock()
	closed, running = c.closed, c.ctrl != nil
	c.mu.Unlock()

	if closed {
		return errShutDown
	}
	if running {
		return nil
	}
	return c.spawn()
}

// spawn starts the child and dials it.
// The caller holds spawnMu.
func (c *Client) spawn() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate this executable to decode in: %w", err)
	}

	dir, err := os.MkdirTemp(socketRoot(), "screenshare-decode-")
	if err != nil {
		return err
	}
	socket := filepath.Join(dir, "host.sock")

	logFile, logPath, err := ffmpeg.NewRunLog("decode-host")
	if err != nil {
		os.RemoveAll(dir)
		return err
	}

	cmd := exec.Command(exe, Subcommand, socket)
	// The child is the process that plays the pipelines, so the plugin path belongs to it.
	if path, ok := gstbundle.PluginPath(); ok {
		cmd.Env = append(os.Environ(), gstbundle.PathVar+"="+path)
	}
	cmd.Stdout = logFile

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logFile.Close()
		os.RemoveAll(dir)
		return err
	}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		os.RemoveAll(dir)
		return fmt.Errorf("cannot start the decode host: %w", err)
	}
	// A host outliving the backend holds the GPU and the frame sockets with nothing driving it.
	ffmpeg.KillOnAppExit(cmd)

	proc := &process{cmd: cmd, exited: make(chan struct{})}
	go func() {
		defer close(proc.exited)
		// The copy drains before the wait, so the log holds everything the host said.
		io.Copy(logFile, stderr)
		cmd.Wait()
		logFile.Close()
		c.hostExited(dir, logPath)
	}()

	c.mu.Lock()
	c.dir, c.socket = dir, socket
	c.mu.Unlock()

	ctrl, err := c.dialWithin(connControl, dialTimeout)
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("the decode host did not answer: %w", err)
	}
	lifecycle, err := c.dial(connLifecycle)
	if err != nil {
		ctrl.Close()
		cmd.Process.Kill()
		return fmt.Errorf("the decode host did not answer: %w", err)
	}

	c.mu.Lock()
	c.proc, c.ctrl = proc, ctrl
	c.mu.Unlock()

	go c.readLifecycle(lifecycle)
	logger.Infof("decoding in a child process, logging to %s", logPath)
	return nil
}

// dial opens one connection of a kind to a host already listening.
func (c *Client) dial(kind connKind) (*conn, error) {
	c.mu.Lock()
	socket := c.socket
	c.mu.Unlock()

	if socket == "" {
		return nil, errNoHost
	}

	raw, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	conn := newConn(raw)
	if err := conn.send(kind); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// dialWithin tries the socket until the child binds it or the bound runs out.
func (c *Client) dialWithin(kind connKind, within time.Duration) (*conn, error) {
	deadline := time.Now().Add(within)
	for {
		conn, err := c.dial(kind)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(dialInterval)
	}
}
