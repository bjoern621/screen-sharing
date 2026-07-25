package publish

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bjoernblessin.de/screenshare/ffmpeg"
)

// superviseConfig launches one child process for an engine that is not ffmpeg.
// extraFiles are inherited by the child starting at fd 3, in order; the portal
// PipeWire remote fd is passed this way. onCleanup runs after the child exits,
// releasing engine-owned resources (the portal session).
// parseStdout, when set, consumes the child's stdout, which is where an engine
// prints its progress; the stream is teed into the run log on the way, so the
// log holds everything the child said either way.
type superviseConfig struct {
	exe         string
	args        []string
	tag         string
	extraFiles  []*os.File
	parseStdout func(io.Reader)
	onExit      func(err error, stderrTail string, logPath string)
	onCleanup   func()
}

// supervise starts and watches the child, mirroring ffmpeg.Start's behaviour for
// engines that speak no ffmpeg -progress stream: stderr is teed to a per-run log
// and a bounded tail, and onExit fires once with the tail and log path.
func supervise(cfg superviseConfig) (Handle, error) {
	logDir, err := ffmpeg.LogDir()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", sanitize(cfg.tag), time.Now().Format("20060102-150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("cannot create run log: %w", err)
	}
	fmt.Fprintf(logFile, "%s %s\n\n", cfg.exe, strings.Join(cfg.args, " "))

	// The stderr copier and the stdout tee both write the one log, so the writes
	// are serialized to keep either from landing inside the other's line.
	log := &syncWriter{w: logFile}

	cmd := exec.Command(cfg.exe, cfg.args...)
	cmd.ExtraFiles = cfg.extraFiles

	tail := &tailBuffer{max: 4096}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logFile.Close()
		return nil, err
	}

	var stdout io.ReadCloser
	if cfg.parseStdout != nil {
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			logFile.Close()
			return nil, err
		}
	} else {
		cmd.Stdout = logFile
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("cannot start %s: %w", cfg.exe, err)
	}

	proc := &child{cmd: cmd}
	proc.running.Store(true)

	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		io.Copy(io.MultiWriter(log, tail), stderr)
	}()
	if cfg.parseStdout != nil {
		readers.Add(1)
		go func() {
			defer readers.Done()
			cfg.parseStdout(io.TeeReader(stdout, log))
		}()
	}

	go func() {
		readers.Wait()
		waitErr := cmd.Wait()
		proc.running.Store(false)
		logFile.Close()
		if cfg.onCleanup != nil {
			cfg.onCleanup()
		}

		var reportErr error
		if !proc.stopped.Load() && waitErr != nil {
			reportErr = fmt.Errorf("%s exited: %w", cfg.tag, waitErr)
		}
		if cfg.onExit != nil {
			cfg.onExit(reportErr, strings.TrimSpace(tail.String()), logPath)
		}
	}()

	return proc, nil
}

// child is a supervised non-ffmpeg process. Its methods are safe to call from
// multiple goroutines.
type child struct {
	cmd     *exec.Cmd
	running atomic.Bool
	stopped atomic.Bool
}

func (c *child) Running() bool { return c.running.Load() }

func (c *child) Stop() {
	if !c.running.Load() {
		return
	}
	c.stopped.Store(true)
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
}

// tailBuffer keeps the last max bytes written to it, so the end of stderr can be
// surfaced in an exit message without holding the whole log in memory.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// syncWriter serializes concurrent writes onto one underlying writer.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// sanitize makes tag safe to use in a filename.
func sanitize(tag string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, tag)
}
