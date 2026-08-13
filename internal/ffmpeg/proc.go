package ffmpeg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Stats is one encoder progress sample.
// Both publish engines emit it, each measuring it from what its own pipeline offers,
// so the meaning of a field is fixed here rather than by whichever engine filled it:
//
//   - Fps, CaptureFps and InstMbps are per-interval rates: the frames encoded,
//     the frames captured, and the bytes produced since the previous sample, over
//     the wall-clock interval between the two samples. A rate averaged over the
//     run instead would take minutes to follow a collapse.
//   - Speed and AvgMbps are cumulative over the run.
//   - Dup counts frames the encoder repeated to hold the output rate, which is
//     what rises when capture cannot keep up with it. Drop counts input frames
//     discarded because they arrived faster than the output rate, a different
//     event.
//   - Missing marks the figures this sample carries no measurement for.
//
// MarshalJSON is the wire format for the "publish:stats" event.
type Stats struct {
	Frame    int
	Fps      float64
	SizeKiB  float64
	TimeSec  float64
	Speed    float64
	Dup      int
	Drop     int
	InstMbps float64
	AvgMbps  float64
	// CaptureFps is how often the screen produced a new picture, as against Fps,
	// which is how often the encoder emitted one.
	// The two differ wherever a backend paces its output independently of its source:
	// the portal path repeats the newest damage frame at the configured rate, so its Fps equals the
	// target whatever the screen does, and this is the figure a starved capture or a static screen
	// shows up in.
	// A pipeline built without the rate probe marks it missing instead of reporting a zero rate.
	CaptureFps float64
	Missing    Missing
}

// Missing is the set of Stats figures a sample carries no measurement for.
// ffmpeg reports "N/A" until the first packet is muxed, a per-interval figure has no value on the
// first sample of a run, and an engine instruments only what its pipeline exposes;
// none of the three is the measured zero that marks a stalled encoder.
// The zero value marks nothing missing, so an engine that measures a figure leaves its flag alone.
type Missing struct {
	Fps        bool
	CaptureFps bool
	SizeKiB    bool
	TimeSec    bool
	Speed      bool
	InstMbps   bool
	AvgMbps    bool
}

// MarshalJSON writes every figure Missing marks as null, which is what keeps an unmeasured figure
// out of the UI's numbers.
// encoding/json has no per-field presence for a float64, hence the pointer-shaped wire struct.
func (s Stats) MarshalJSON() ([]byte, error) {
	measured := func(v float64, missing bool) *float64 {
		if missing {
			return nil
		}
		return &v
	}
	return json.Marshal(struct {
		Frame      int      `json:"frame"`
		Fps        *float64 `json:"fps"`
		CaptureFps *float64 `json:"captureFps"`
		SizeKiB    *float64 `json:"sizeKiB"`
		TimeSec    *float64 `json:"timeSec"`
		Speed      *float64 `json:"speed"`
		Dup        int      `json:"dup"`
		Drop       int      `json:"drop"`
		InstMbps   *float64 `json:"instMbps"`
		AvgMbps    *float64 `json:"avgMbps"`
	}{
		Frame:      s.Frame,
		Fps:        measured(s.Fps, s.Missing.Fps),
		CaptureFps: measured(s.CaptureFps, s.Missing.CaptureFps),
		SizeKiB:    measured(s.SizeKiB, s.Missing.SizeKiB),
		TimeSec:    measured(s.TimeSec, s.Missing.TimeSec),
		Speed:      measured(s.Speed, s.Missing.Speed),
		Dup:        s.Dup,
		Drop:       s.Drop,
		InstMbps:   measured(s.InstMbps, s.Missing.InstMbps),
		AvgMbps:    measured(s.AvgMbps, s.Missing.AvgMbps),
	})
}

// Proc is a running ffmpeg or ffplay child.
// Its methods are safe to call from multiple goroutines.
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

// Stop kills the child.
// The pending exit is reported to onExit with a nil error, since a requested stop is not a failure.
//
// It is idempotent: a child that has already gone is the state a stop asks for, so a second call
// returns having done nothing.
func (p *Proc) Stop() {
	assert.IsNotNil(p.cmd, "a running child was launched from a command")

	if !p.running.Load() {
		return
	}
	p.stopped.Store(true)
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}

// stdoutLineInitial and stdoutLineMax size the scanner behind onLine.
// A line is one message, and the roster the native grid answers grows with the stream count,
// so the limit is generous rather than tuned.
const (
	stdoutLineInitial = 64 * 1024
	stdoutLineMax     = 4 * 1024 * 1024
)

// Start launches exe with args and supervises it.
//
// hideWindow hides the child's console window on Windows (no effect elsewhere);
// it must be false for ffplay, whose video window would otherwise be hidden too.
// wantStdin opens a pipe to the child's stdin, exposed as Proc.Stdin; without it the child reads
// from the null device.
// tag names the run log.
// onStats, when non-nil, receives an encoder progress sample per ffmpeg -progress block.
// onLine, when non-nil, receives every line the child writes to stdout, for a child that talks back
// rather than only logging; both read the same pipe, so a caller passes one or the other.
// onExit fires once when the child exits, with a non-nil error only on an unexpected failure,
// the tail of stderr, and the path of the full run log.
func Start(
	exe string,
	args []string,
	hideWindow bool,
	wantStdin bool,
	tag string,
	extraEnv []string,
	onStats func(Stats),
	onLine func(string),
	onExit func(err error, stderrTail string, logPath string),
) (*Proc, error) {
	assert.Assert(onStats == nil || onLine == nil, "one reader of the child's stdout", tag)
	assert.Assert(exe != "", "a child is launched from a resolved binary", tag)
	assert.Assert(tag != "", "a child names the run log it writes")

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
		// Machine-readable progress on stdout, kept out of BuildPublishArgs so the command shown in the
		// UI stays the plain encoder line.
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
	if onStats != nil || onLine != nil {
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
	// Here rather than beside the reaping goroutine below: the assignment needs the pid to still be
	// this child's, which holding the process handle guarantees only until cmd.Wait releases it.
	KillOnAppExit(cmd)

	proc := &Proc{cmd: cmd, Stdin: stdin}
	proc.running.Store(true)
	assert.Assert(!wantStdin || proc.Stdin != nil, "a child asked for a stdin pipe carries one", tag)

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
	if onLine != nil {
		readers.Add(1)
		go func() {
			defer readers.Done()
			// Mirrored into the run log a line at a time, so what the child said is in the log the way it
			// would be without a reader on the pipe.
			sc := bufio.NewScanner(stdout)
			sc.Buffer(make([]byte, 0, stdoutLineInitial), stdoutLineMax)
			for sc.Scan() {
				fmt.Fprintln(logFile, sc.Text())
				onLine(sc.Text())
			}
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
