package publish

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
)

// superviseConfig launches one child process for an engine that is not ffmpeg.
// extraFiles are inherited by the child starting at fd 3, in order: the portal PipeWire remote fd
// travels this way, and nothing does on Windows, which inherits no descriptors.
// onCleanup runs after the child exits, releasing engine-owned resources like the portal session.
// parseStdout, when set, consumes the child's stdout, where an engine prints its progress.
// That stream is teed into the run log on the way,
// so the log holds everything the child said either way.
// env adds to this process's environment rather than replacing it, so a child keeps everything the
// app was started with (GstChildEnv fills it).
// redact hides the run's secrets in whatever is written to the log, and nil hides nothing.
// The command line is the first thing the log carries and it spells the relay token and the SRT
// passphrase out in full, so a log offered to whoever is helping carries both out with it
// (transport.Redact builds the function).
type superviseConfig struct {
	exe         string
	env         []string
	args        []string
	tag         string
	extraFiles  []*os.File
	redact      func(string) string
	parseStdout func(io.Reader)
	onExit      func(err error, stderrTail string, logPath string)
	onCleanup   func()
}

// supervise starts and watches the child, mirroring ffmpeg.Start for engines that speak no ffmpeg
// -progress stream: stderr is teed to a per-run log and a bounded tail, and onExit fires once with
// the tail and the log path.
// A log directory, a pipe or a child the machine will not give is an environment failure and comes
// back as an error; the asserts below state this package's own contract.
func supervise(cfg superviseConfig) (Handle, error) {
	assert.Assert(cfg.exe != "", "a supervised child names the executable to run", cfg.tag)
	assert.Assert(cfg.tag != "", "a supervised child names its run log", cfg.exe)
	// The descriptors reach the child by position,
	// so a closed slot shifts every later fd and lands the pipeline on the wrong one.
	for i, f := range cfg.extraFiles {
		assert.IsNotNil(f, "an inherited descriptor is an open file", cfg.tag, i)
	}
	// Windows inherits none at all: os/exec supports ExtraFiles on Unix alone, and a child handed one
	// there does not start, failing with "fork/exec <exe>: not supported by windows" rather than
	// anything naming a descriptor.
	// Asserting here states which caller was wrong, where the exec error names only the launcher.
	assert.Assert(runtime.GOOS != "windows" || len(cfg.extraFiles) == 0,
		"a Windows child is passed no descriptors to inherit", cfg.tag, len(cfg.extraFiles))

	logDir, err := ffmpeg.LogDir()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", sanitize(cfg.tag), time.Now().Format("20060102-150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("cannot create run log: %w", err)
	}
	commandLine := fmt.Sprintf("%s %s", cfg.exe, strings.Join(cfg.args, " "))
	if cfg.redact != nil {
		commandLine = cfg.redact(commandLine)
	}
	fmt.Fprintf(logFile, "%s\n\n", commandLine)

	// The stderr copier and the stdout tee write the one log, so the writes are serialized to keep
	// either from landing inside the other's line.
	log := &syncWriter{w: logFile}

	cmd := exec.Command(cfg.exe, cfg.args...)
	cmd.ExtraFiles = cfg.extraFiles
	if len(cfg.env) > 0 {
		cmd.Env = append(os.Environ(), cfg.env...)
	}

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
	// Registered before the goroutine that waits on the child, for the reason KillOnAppExit documents.
	// A GStreamer pipeline orphans exactly like an ffmpeg one.
	ffmpeg.KillOnAppExit(cmd)

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

	// Both readers drain before the wait, so the tail and the log hold everything the child wrote by
	// the time onExit carries them.
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

// child is a supervised non-ffmpeg process, and its methods are safe to call from any goroutine.
// stopped is what keeps a kill this process asked for out of the exit error.
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

// tailBuffer keeps the last max bytes written to it, so an exit message can carry the end of stderr
// without the whole log being held in memory.
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

// syncWriter serializes concurrent writes onto one writer.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// sanitize makes tag safe to spell in a filename.
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
