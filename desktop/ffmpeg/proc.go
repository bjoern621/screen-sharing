package ffmpeg

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
// tag names the run log. onStats, when non-nil, receives an encoder progress
// sample per ffmpeg -progress block. onExit fires once when the child exits,
// with a non-nil error only on an unexpected failure, the tail of stderr, and
// the path of the full run log.
func Start(
	exe string,
	args []string,
	hideWindow bool,
	tag string,
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

	err = cmd.Start()
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("cannot start %s: %w", exe, err)
	}

	proc := &Proc{cmd: cmd}
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

// parseProgress reads ffmpeg's -progress key=value stream and emits one Stats
// per block (blocks end with a "progress=" line). InstMbps is derived from the
// change in total_size and out_time between consecutive blocks.
func parseProgress(r io.Reader, onStats func(Stats)) {
	scanner := bufio.NewScanner(r)
	cur := map[string]string{}
	var prevBytes, prevTime float64
	havePrev := false

	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		if key != "progress" {
			cur[key] = value
			continue
		}

		bytesNow := parseFloat(cur["total_size"])
		timeNow := parseFloat(cur["out_time_us"]) / 1_000_000

		stats := Stats{
			Frame:   int(parseFloat(cur["frame"])),
			Fps:     parseFloat(cur["fps"]),
			SizeKiB: bytesNow / 1024,
			TimeSec: timeNow,
			Speed:   parseFloat(strings.TrimSuffix(cur["speed"], "x")),
			Drop:    int(parseFloat(cur["drop_frames"])),
			AvgMbps: parseFloat(strings.TrimSuffix(cur["bitrate"], "kbits/s")) / 1000,
		}
		if havePrev && timeNow > prevTime {
			stats.InstMbps = (bytesNow - prevBytes) * 8 / (timeNow - prevTime) / 1_000_000
		}
		prevBytes, prevTime, havePrev = bytesNow, timeNow, true

		onStats(stats)
		cur = map[string]string{}
	}
}

// parseFloat returns the float value of s, or 0 for "N/A" and unparseable input.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// sanitizeTag makes tag safe to use in a filename (watch tags carry the stream
// name, which is user-controlled).
func sanitizeTag(tag string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, tag)
}

// tailBuffer keeps the last max bytes written to it, used to surface the end of
// stderr in an exit message without holding the whole log in memory.
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
