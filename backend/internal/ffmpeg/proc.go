package ffmpeg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"bjoernblessin.de/go-utils/util/assert"
)

// Stats is one encoder progress sample.
// Both publish engines emit it off their own pipelines, so a field means what is stated here rather
// than what filled it:
//
//   - Fps, CaptureFps and InstMbps are per-interval rates: frames encoded, frames
//     captured and bytes produced since the previous sample, over the wall-clock
//     interval between the two. An average over the run takes minutes to follow a
//     collapse.
//   - Speed and AvgMbps are cumulative over the run.
//   - Dup counts frames the encoder repeated to hold the output rate, which is
//     what rises when capture falls behind it. Drop counts input frames discarded
//     for arriving faster than the output rate, the opposite event.
//   - Missing marks the figures this sample carries no measurement for.
//
// MarshalJSON is the wire format of the "publish:stats" event.
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
	// CaptureFps is how often the screen produced a picture, against Fps, how often the encoder
	// emitted one.
	// The two part wherever a backend paces its output independently of its source:
	// the portal path repeats the newest damage frame at the configured rate, so its Fps holds at the
	// target whatever the screen does, and a starved capture or a still screen shows up here alone.
	// A pipeline built with no rate probe marks it missing rather than reporting a zero rate.
	CaptureFps float64

	// What the publish leg costs a frame, and the stages a machine sending a stream can measure.
	//
	// TransitMs is the mean wall clock one frame spent between the capture stamping it and the
	// encoded stream leaving the pipeline, over the last interval: converting, encoding and parsing.
	// LinkMs is the delivery window the leg settled on with the relay, the delay every packet is held
	// for so a lost one has room to arrive again, and RttMs the round trip that says whether the
	// window has room for that.
	//
	// All three are an engine's to measure or to mark missing.
	// Only a pipeline this app runs itself can answer them, so the ffmpeg engine marks all three, and
	// a leg whose transport keeps no link counters marks the last two.
	TransitMs float64
	LinkMs    float64
	RttMs     float64

	Missing Missing
}

// Missing is the set of Stats figures a sample carries no measurement for.
// ffmpeg reports "N/A" until the first packet is muxed, a per-interval figure has none on the first
// sample of a run, and an engine instruments what its pipeline exposes and no more.
// None of the three is the measured zero that marks a stalled encoder.
// The zero value marks nothing missing, so an engine that measures a figure leaves its flag alone.
type Missing struct {
	Fps        bool
	CaptureFps bool
	SizeKiB    bool
	TimeSec    bool
	Speed      bool
	InstMbps   bool
	AvgMbps    bool
	TransitMs  bool
	LinkMs     bool
	RttMs      bool
}

// MarshalJSON writes every figure Missing marks as null, which keeps an unmeasured figure out of the
// UI's numbers.
// encoding/json carries no per-field presence for a float64, hence the pointer-shaped wire struct.
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
		TransitMs  *float64 `json:"transitMs"`
		LinkMs     *float64 `json:"linkMs"`
		RttMs      *float64 `json:"rttMs"`
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
		TransitMs:  measured(s.TransitMs, s.Missing.TransitMs),
		LinkMs:     measured(s.LinkMs, s.Missing.LinkMs),
		RttMs:      measured(s.RttMs, s.Missing.RttMs),
	})
}

// Proc is a running ffmpeg or ffplay child.
// Its methods are safe to call from any goroutine.
type Proc struct {
	cmd     *exec.Cmd
	running atomic.Bool
	stopped atomic.Bool // set by Stop so the exit that follows is not reported as a failure
	// Stdin is the child's stdin pipe, nil unless Start was told to open one.
	// Concurrent writers need coordination of their own.
	Stdin io.WriteCloser
}

// Running goes false when the child has been reaped, which is after its output pipes have drained.
func (p *Proc) Running() bool { return p.running.Load() }

// Stop kills the child.
// The exit that follows reaches onExit with a nil error, a requested stop being no failure.
//
// Idempotent: a child already gone is the state a stop asks for, so a second call does nothing.
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
// One line is one message and the roster the native grid answers grows with the stream count,
// so the ceiling is generous rather than tuned.
const (
	stdoutLineInitial = 64 * 1024
	stdoutLineMax     = 4 * 1024 * 1024
)

// Start launches exe with args and supervises it until it exits.
// The child outlives the call and is stopped through Proc.Stop, or by App.shutdown, or on Windows by
// the Job Object (KillOnAppExit).
//
// hideWindow hides the child's console window on Windows and does nothing elsewhere.
// It must be false for ffplay, whose video window the same flag would hide.
// wantStdin opens a pipe to the child's stdin, exposed as Proc.Stdin.
// Without it the child reads the null device.
// tag names the run log.
// onStats, when non-nil, takes one encoder progress sample per ffmpeg -progress block.
// onLine, when non-nil, takes every line the child writes to stdout, for a child that talks back
// rather than only logging.
// Both read the one pipe, so a caller passes one or the other.
// onExit fires once, carrying a non-nil error only on an unexpected exit, the bounded tail of stderr
// and the path of the full run log.
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
	opts ...Option,
) (*Proc, error) {
	assert.Assert(onStats == nil || onLine == nil, "one reader of the child's stdout", tag)
	assert.Assert(exe != "", "a child is launched from a resolved binary", tag)
	assert.Assert(tag != "", "a child names the run log it writes")

	logFile, logPath, err := NewRunLog(tag)
	if err != nil {
		return nil, err
	}

	full := args
	if onStats != nil {
		// Machine-readable progress on stdout, added here and not in BuildPublishArgs so the command the
		// UI shows stays the plain encoder line.
		full = append(append([]string{}, args...), "-progress", "pipe:1")
	}

	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}
	commandLine := fmt.Sprintf("%s %s", exe, strings.Join(full, " "))
	if cfg.redact != nil {
		commandLine = cfg.redact(commandLine)
	}
	fmt.Fprintf(logFile, "%s\n\n", commandLine)

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
	// Here and not beside the reaping goroutine below: the pid has to still be this child's,
	// which the process handle guarantees only until cmd.Wait releases it.
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
			// Mirrored into the run log a line at a time, so the log holds what a run with no reader on
			// the pipe would have written.
			sc := bufio.NewScanner(stdout)
			sc.Buffer(make([]byte, 0, stdoutLineInitial), stdoutLineMax)
			for sc.Scan() {
				fmt.Fprintln(logFile, sc.Text())
				onLine(sc.Text())
			}
		}()
	}

	go func() {
		readers.Wait() // every pipe drained before the child is reaped, so no output is lost to the exit
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
