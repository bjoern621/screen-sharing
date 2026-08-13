// Package encoderate measures how many frames per second this machine encodes at the configured
// stream settings, so the UI can warn when the target rate is above what the encoder sustains.
//
// It is the encoder's counterpart to the uplink probe.
// Both answer the same shape of question, "does this machine carry what these settings ask of it",
// and both answer it by measuring rather than by reading a specification: an encoder's throughput
// depends on the CPU or the fixed-function block, on the picture size, on the chroma and on the
// rate-control mode together, and no table holds that product.
//
// A publish leg that asks for more than the answer is not a stream that degrades.
// The encoder falls behind, the frames it cannot take are discarded ahead of it,
// and what reaches the relay is the rate it managed, with the difference gone.
// That failure is silent everywhere else: the pipeline runs, the transport connects,
// and only the frame counters say that most of the capture never left the machine.
//
// # What the figure covers
//
// The result is a range, not a number, because encode cost depends on content the way bitrate does.
// The two ends are measured on generated frames at the two extremes an encoder can be handed:
// uncorrelated noise, where nothing predicts and nothing repeats, and a moving object on a flat
// field, where almost everything does.
// A screen sits between them and moves within them as its content changes.
//
// Everything else is the run's own: the same encoder, the same rate-control properties,
// the same conversion into the same encoder input, at the same picture size.
// Only frame acquisition and frame delivery are replaced, and both are replaced by the cheapest
// thing that fits, so what is left in the middle is the encoder.
package encoderate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The two ends of the content range, named where they are asked for so a call site reads as the end
// it measures rather than as a bare true or false.
// Heavy content yields the low rate and light content the high one.
const (
	lightContent = false
	heavyContent = true
)

// probeFrames is how many frames each timed run encodes.
// It is the trade the uplink probe's payload size makes: long enough that the per-frame figure
// settles and a scheduling hiccup does not carry the whole reading, short enough that a slow
// combination still answers in seconds.
const probeFrames = 60

// measureTimeout bounds the whole measurement, every run of it together.
// A hung encoder fails the probe instead of pinning the button's loading state open.
const measureTimeout = 60 * time.Second

// boundedFraction is how close to the generator's own rate an encode may be timed before the
// reading is one of the generator instead.
// A probe cannot measure an encoder faster than the frames reaching it, and the honest answer there
// is that the ceiling was not found, not the ceiling of the instrument.
const boundedFraction = 0.9

// Rate is what this machine encodes the configured stream at, in frames per second.
//
// The two ends bracket content rather than measurement error: LowFps is what the hardest content
// codes at and HighFps the easiest, so a target below LowFps is one no content can push the encoder
// off and a target above HighFps is one none can reach.
//
// The bounded flags mark an end timed against the frame generator rather than against the encoder,
// which happens where the encoder is faster than a probe can feed it.
// Such an end is a floor: the machine encodes at least that fast, and how much faster this cannot
// say.
//
// They are one flag per end because the two ends carry the reading in opposite directions.
// A floor at the low end understates where the safe rate begins, which costs a target more caution
// than it needed.
// A floor at the high end is what a target above the range is refused on, so reading it as the
// encoder's ceiling would call a rate unreachable that was never measured.
type Rate struct {
	LowFps      float64 `json:"lowFps"`
	HighFps     float64 `json:"highFps"`
	LowBounded  bool    `json:"lowBounded"`
	HighBounded bool    `json:"highBounded"`
}

// engineProbe is one publish engine's half of the measurement: where its executable is,
// what its child needs in the environment, the command that encodes generated frames,
// and the command that generates them and encodes nothing.
//
// The probe runs the same binary a publish would, so it resolves and launches it the same way.
// A measurement taken against a different copy of the encoder than the one that will encode the
// stream is a figure about another machine's install.
type engineProbe struct {
	exe     func() (string, error)
	env     func() []string
	encode  func(s settings.Settings, width, height, frames int, heavy bool) ([]string, error)
	ceiling func(s settings.Settings, width, height, frames int, heavy bool) []string
}

// engineProbes holds the probe per publish engine.
// Which one runs follows from the capture backend, exactly as a publish does,
// so the figure describes the engine that would encode the stream and not whichever one the machine
// also happens to carry.
var engineProbes = map[string]engineProbe{
	publish.EngineFfmpeg: {
		exe: func() (string, error) { return ffmpeg.FindExe("ffmpeg") },
		encode: func(s settings.Settings, width, height, frames int, heavy bool) ([]string, error) {
			return ffmpeg.BuildEncodeProbeArgs(s, width, height, frames, heavy)
		},
		ceiling: func(s settings.Settings, width, height, frames int, heavy bool) []string {
			return ffmpeg.BuildProbeCeilingArgs(width, height, s.Publish.Fps, frames, heavy)
		},
	},
	publish.EngineGst: {
		exe:    publish.FindGstExe,
		env:    publish.GstChildEnv,
		encode: publish.GstEncodeProbe,
		ceiling: func(s settings.Settings, width, height, frames int, heavy bool) []string {
			return publish.GstProbeCeiling(s, width, height, frames, heavy)
		},
	},
}

// Measure times the configured encoder on generated frames of both content extremes and returns the
// rate this machine sustains.
//
// A picture size of zero is refused rather than measured at some size of this package's choosing.
// The rate is a fact about the frames the encoder is handed, and the monitor the stream captures is
// what decides how large those are.
// The caller reaches this with an unresolved size where display enumeration is unavailable,
// which is an Umgebungsfehler and leaves as an error: a measurement taken at a size the stream will
// not use is worse than none.
func Measure(ctx context.Context, s settings.Settings, width, height int) (Rate, error) {
	assert.IsNotNil(ctx, "a measurement runs under a context")

	if width <= 0 || height <= 0 {
		return Rate{}, fmt.Errorf("the encode rate is measured at the captured picture size, and this machine reports none for monitor %d", s.Publish.Monitor)
	}

	engine, err := publish.EngineFor(s.Publish.Capture)
	if err != nil {
		return Rate{}, err
	}
	probe, ok := engineProbes[engine]
	assert.Assert(ok, "every publish engine states how its encoder is timed", engine)

	exe, err := probe.exe()
	if err != nil {
		return Rate{}, fmt.Errorf("the %s publish engine cannot be timed here: %w", engine, err)
	}

	ctx, cancel := context.WithTimeout(ctx, measureTimeout)
	defer cancel()

	low, lowBounded, err := measureEnd(ctx, probe, exe, s, width, height, heavyContent)
	if err != nil {
		return Rate{}, err
	}
	high, highBounded, err := measureEnd(ctx, probe, exe, s, width, height, lightContent)
	if err != nil {
		return Rate{}, err
	}

	// The heavy end codes the content nothing predicts, so it cannot come out above the light one
	// unless something other than the encoder set one of the two rates.
	// Where that happens the reading is the machine's load rather than its encoder,
	// and a range whose ends are the wrong way round would put a warning threshold above the rate it
	// is supposed to bound.
	if low > high {
		return Rate{}, fmt.Errorf("the encoder timed faster on the harder content (%.1f fps against %.1f fps), so something other than the encoder paced at least one run: measure again on an otherwise idle machine", low, high)
	}

	rate := Rate{LowFps: low, HighFps: high, LowBounded: lowBounded, HighBounded: highBounded}
	assert.Assert(rate.LowFps > 0 && rate.HighFps >= rate.LowFps,
		"a measured range runs from the hardest content up to the easiest", rate.LowFps, rate.HighFps)

	logger.Infof("encode rate measured: %.1f-%.1f fps for %s at %dx%d on %s (bounded by the frame generator: low %t, high %t)",
		rate.LowFps, rate.HighFps, s.Publish.Codec, width, height, engine, rate.LowBounded, rate.HighBounded)
	return rate, nil
}

// measureEnd times one end of the content range, and reports whether the frame generator rather
// than the encoder set the pace.
//
// Three runs make one figure.
// The encode of probeFrames frames is the measurement, the encode of a single frame is what a
// pipeline costs to reach its first frame at all, and the difference between them is the time that
// went into frames.
// The third run generates the same frames and encodes none, which is the rate no encode here can
// exceed.
func measureEnd(
	ctx context.Context,
	probe engineProbe,
	exe string,
	s settings.Settings,
	width, height int,
	heavy bool,
) (fps float64, bounded bool, err error) {
	assert.IsNotNil(ctx, "a timed run runs under a context")
	assert.IsNotNil(probe.encode, "an engine probe states how its encoder is launched")
	assert.IsNotNil(probe.ceiling, "an engine probe states what its frame generator alone costs")
	assert.Assert(exe != "", "a timed run names the binary it launches")
	assert.Assert(width > 0 && height > 0, "an encoder is timed at the captured picture size", width, height)

	full, err := probe.encode(s, width, height, probeFrames, heavy)
	if err != nil {
		return 0, false, err
	}
	first, err := probe.encode(s, width, height, 1, heavy)
	if err != nil {
		return 0, false, err
	}

	// Read once and given to all three runs, so the three are timed under one environment and a
	// difference between them is the pipeline's.
	var env []string
	if probe.env != nil {
		env = probe.env()
	}

	startup, err := run(ctx, exe, env, first)
	if err != nil {
		return 0, false, err
	}
	elapsed, err := run(ctx, exe, env, full)
	if err != nil {
		return 0, false, err
	}
	fps, err = rate(probeFrames-1, elapsed-startup)
	if err != nil {
		return 0, false, err
	}

	// The ceiling keeps its own startup cost, unlike the encode it bounds.
	// Left in, it reads a little low, which can only make an encode look closer to it than it is:
	// the probe then reports a floor where it might have reported a rate, and never a rate where it
	// should have reported a floor.
	ceilingElapsed, err := run(ctx, exe, env, probe.ceiling(s, width, height, probeFrames, heavy))
	if err != nil {
		return 0, false, err
	}
	ceiling, err := rate(probeFrames, ceilingElapsed)
	if err != nil {
		return 0, false, err
	}

	assert.Assert(fps > 0, "a timed end is a positive rate", fps)
	return fps, fps >= ceiling*boundedFraction, nil
}

// run times one probe process from launch to exit.
//
// A process that fails carries the reason in its own output, which is another program's error text,
// so it is handed back as one.
// Neither engine writes anything on a successful run: both probes are launched at an error-only log
// level.
func run(ctx context.Context, exe string, env []string, args []string) (time.Duration, error) {
	assert.IsNotNil(ctx, "a probe process runs under a context")
	assert.Assert(exe != "", "a probe process names the binary it launches")
	assert.Assert(len(args) > 0, "a probe process is launched with arguments", exe)

	cmd := exec.CommandContext(ctx, exe, args...)
	// Added to this process's environment rather than replacing it, the way every other child here is
	// launched.
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("the encode probe did not finish within %s: these settings encode slower than the measurement allows for", measureTimeout)
		}
		return 0, fmt.Errorf("the encode probe failed: %w: %s", err, out)
	}
	return elapsed, nil
}

// rate turns a count of frames and the time they took into frames per second.
//
// A run that measures no time at all is refused rather than divided by.
// It means the clock could not tell the two ends of the run apart, which says nothing about how
// fast the encoder is, and the figure every frame-rate warning is judged against is worth nothing
// if it is a guess.
func rate(frames int, elapsed time.Duration) (float64, error) {
	assert.Assert(frames > 0, "a rate counts at least one frame", frames)

	if elapsed <= 0 {
		return 0, fmt.Errorf("the encode probe finished with no measurable time in it")
	}

	fps := float64(frames) / elapsed.Seconds()
	assert.Assert(fps > 0, "a rate over measurable time is positive", fps, frames, elapsed)
	return fps, nil
}
