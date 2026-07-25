package ffmpeg

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
)

// Stats is one encoder progress sample, parsed from ffmpeg's -progress output.
// The JSON tags are the wire format for the "publish:stats" event.
type Stats struct {
	Frame    int     `json:"frame"`
	Fps      float64 `json:"fps"`
	SizeKiB  float64 `json:"sizeKiB"`
	TimeSec  float64 `json:"timeSec"`
	Speed    float64 `json:"speed"`
	Drop     int     `json:"drop"`
	InstMbps float64 `json:"instMbps"` // derived from Δsize/Δtime between samples
	AvgMbps  float64 `json:"avgMbps"`  // ffmpeg's cumulative bitrate field
}

// Proc is a running ffmpeg or ffplay child. Its methods are safe to call from
// multiple goroutines.
type Proc struct {
	cmd     *exec.Cmd
	running atomic.Bool
	stopped atomic.Bool // set by Stop so the natural exit is not reported as an error
	// Stdin is the child's stdin pipe, nil unless Start was told to open one.
	// Writes from concurrent goroutines need external coordination.
	Stdin io.WriteCloser
}

// Running reports whether the child is still alive.
func (p *Proc) Running() bool { return p.running.Load() }

// Stop kills the child. The pending exit is reported to onExit with a nil
// error, since a requested stop is not a failure.
func (p *Proc) Stop() {
	if !p.running.Load() {
		return
	}
	p.stopped.Store(true)
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}

// Start launches exe with args and supervises it.
//
// hideWindow hides the child's console window on Windows (no effect elsewhere);
// it must be false for ffplay, whose video window would otherwise be hidden too.
// wantStdin opens a pipe to the child's stdin, exposed as Proc.Stdin; without
// it the child reads from the null device.
// tag names the run log. onStats, when non-nil, receives an encoder progress
// sample per ffmpeg -progress block. onExit fires once when the child exits,
// with a non-nil error only on an unexpected failure, the tail of stderr, and
// the path of the full run log.
func Start(
	exe string,
	args []string,
	hideWindow bool,
	wantStdin bool,
	tag string,
	extraEnv []string,
	onStats func(Stats),
	onExit func(err error, stderrTail string, logPath string),
) (*Proc, error) {
	logDir, err := LogDir()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", sanitizeTag(tag), time.Now().Format("20060102-150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("cannot create run log: %w", err)
	}

	full := args
	if onStats != nil {
		// Machine-readable progress on stdout, kept out of BuildPublishArgs so
		// the command shown in the UI stays the plain encoder line.
		full = append(append([]string{}, args...), "-progress", "pipe:1")
	}
	fmt.Fprintf(logFile, "%s %s\n\n", exe, strings.Join(full, " "))

	tail := &tailBuffer{max: 4096}
	cmd := exec.Command(exe, full...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	setHidden(cmd, hideWindow)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logFile.Close()
		return nil, err
	}

	var stdout io.ReadCloser
	if onStats != nil {
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			logFile.Close()
			return nil, err
		}
	} else {
		cmd.Stdout = logFile
	}

	var stdin io.WriteCloser
	if wantStdin {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			logFile.Close()
			return nil, err
		}
	}

	err = cmd.Start()
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("cannot start %s: %w", exe, err)
	}

	proc := &Proc{cmd: cmd, Stdin: stdin}
	proc.running.Store(true)

	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		io.Copy(io.MultiWriter(logFile, tail), stderr)
	}()
	if onStats != nil {
		readers.Add(1)
		go func() {
			defer readers.Done()
			parseProgress(stdout, onStats)
		}()
	}

	go func() {
		readers.Wait() // all pipe output consumed before reaping the child
		waitErr := cmd.Wait()
		proc.running.Store(false)
		logFile.Close()

		var reportErr error
		if !proc.stopped.Load() && waitErr != nil {
			reportErr = fmt.Errorf("%s exited: %w", tag, waitErr)
		}
		if onExit != nil {
			onExit(reportErr, strings.TrimSpace(tail.String()), logPath)
		}
	}()

	return proc, nil
}
