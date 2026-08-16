// Package encoderate measures how many frames per second this machine encodes the configured stream
// at, so a target rate above what the encoder sustains can be warned about before the publish.
//
// It is the encoder's counterpart to the uplink probe, and it measures rather than reads a
// specification for the same reason: throughput is the product of the CPU or the fixed-function
// block, the picture size, the chroma and the rate-control mode, and no table holds that product.
//
// A publish that asks for more than the answer does not degrade visibly.
// The encoder falls behind, the frames it cannot take are discarded ahead of it, and what reaches
// the relay is the rate it managed, while the pipeline runs and the transport stays connected.
//
// The figure is a range, since encode cost follows content the way bitrate does.
// The ends are timed on generated frames at the two extremes an encoder can be handed: uncorrelated
// noise, where nothing predicts and nothing repeats, and a moving object on a flat field, where
// almost everything does.
// A screen sits between them and moves within them as its content changes.
//
// Everything but frame acquisition and frame delivery is the publish's own: the same encoder, the
// same rate-control properties, the same conversion into the same encoder input, at the same
// picture size.
// Those two are replaced by the cheapest thing that fits, so what is left in the middle is the
// encoder.
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

// The two ends of the content range, named so a call site reads as the end it measures rather than
// as a bare true or false.
// Heavy content yields the low rate and light content the high one.
const (
	lightContent = false
	heavyContent = true
)

// probeFrames is how many frames each timed run encodes.
// Long enough that the per-frame figure settles and a scheduling hiccup does not carry the reading,
// short enough that a slow combination still answers in seconds.
const probeFrames = 60

// measureTimeout bounds the whole measurement, every run of it together.
// A hung encoder fails the probe instead of pinning the button's loading state open.
const measureTimeout = 60 * time.Second

// boundedFraction is how close to the frame generator's own rate an encode may time before the
// reading is the generator's.
// No probe measures an encoder faster than the frames reaching it, so the answer past that point is
// that the encoder's ceiling was not found.
const boundedFraction = 0.9

// Rate is what this machine encodes the configured stream at, in frames per second.
//
// The ends bracket content rather than measurement error: LowFps is what the hardest content codes
// at and HighFps the easiest, so a target under LowFps is one no content pushes the encoder off and
// a target over HighFps is one none reaches.
//
// A bounded end was timed against the frame generator rather than against the encoder, which
// happens where the encoder is faster than a probe can feed it.
// Such an end is a floor: the machine encodes at least that fast, and how much faster is unmeasured.
//
// One flag per end, because the two carry the reading in opposite directions.
// A floor at the low end understates where the safe rate begins, costing a target more caution than
// it needed.
// A floor at the high end is what a target above the range is refused on, so reading it as the
// encoder's ceiling would call a rate unreachable that was never measured.
type Rate struct {
	LowFps      float64 `json:"lowFps"`
	HighFps     float64 `json:"highFps"`
	LowBounded  bool    `json:"lowBounded"`
	HighBounded bool    `json:"highBounded"`
}

// engineProbe is one publish engine's half of the measurement: where its executable is, what its
// child needs in the environment, the command that encodes generated frames, and the command that
// generates them and encodes nothing.
//
// It resolves and launches the binary a publish would, since a measurement taken against a
// different copy of the encoder is a figure about another install.
type engineProbe struct {
	exe     func() (string, error)
	env     func() []string
	encode  func(s settings.Settings, width, height, frames int, heavy bool) ([]string, error)
	ceiling func(s settings.Settings, width, height, frames int, heavy bool) []string
}

// engineProbes holds the probe per publish engine.
// The capture backend decides which one runs, exactly as it does for a publish, so the figure
// describes the engine that would encode the stream and not another one the machine carries.
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

// Measure times the configured encoder on generated frames at both content extremes.
//
// A picture size of zero is refused rather than measured at a size of this package's choosing.
// The rate is a fact about the frames the encoder is handed, and the captured monitor is what
// decides how large those are.
// An unresolved size arrives here where display enumeration is unavailable, an Umgebungsfehler that
// leaves as an error: a measurement at a size the stream will not use is worse than none.
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

	if low > high {
		logger.Warnf("the encoder timed faster on the harder content (%.1f fps against %.1f fps), so something other than the encoder paced at least one run: the bracket runs between the two readings",
			low, high)
	}

	rate := bracket(low, high, lowBounded, highBounded)
	assert.Assert(rate.LowFps > 0 && rate.HighFps >= rate.LowFps,
		"a measured range runs from the hardest content up to the easiest", rate.LowFps, rate.HighFps)

	logger.Infof("encode rate measured: %.1f-%.1f fps for %s at %dx%d on %s (bounded by the frame generator: low %t, high %t)",
		rate.LowFps, rate.HighFps, s.Publish.Codec, width, height, engine, rate.LowBounded, rate.HighBounded)
	return rate, nil
}

// bracket puts the two ends in the order a range runs in.
//
// The heavy end codes what nothing predicts, so it cannot time above the light one unless something
// other than the encoder paced a run, which is what a machine under load does to a measurement.
// Both figures are still readings of this encoder at these settings: reporting them in the order
// they were taken would put a warning threshold above the rate it bounds, and refusing would answer
// a measurement somebody asked for with nothing at all, on a machine where anything else encoding is
// enough to cause it.
//
// Each end's flag travels with its figure, whether the frame generator paced a run being a fact
// about that run rather than about the end it landed on.
func bracket(low, high float64, lowBounded, highBounded bool) Rate {
	if low > high {
		low, high = high, low
		lowBounded, highBounded = highBounded, lowBounded
	}
	return Rate{LowFps: low, HighFps: high, LowBounded: lowBounded, HighBounded: highBounded}
}

// measureEnd times one end of the content range and reports whether the frame generator rather than
// the encoder set the pace.
//
// Three runs make one figure.
// The encode of probeFrames frames is the measurement, the encode of a single frame is what the
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

	// One read, handed to all three runs, so they are timed under one environment and a difference
	// between them is the pipeline's.
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

	// The ceiling keeps its own startup cost, unlike the encode it bounds, so it reads a little low.
	// That error runs one way: the probe reports a floor where a rate was available, never a rate
	// where a floor was due.
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
// A failed process carries its reason in its own output, which is another program's error text and
// is handed back as one.
// Neither engine writes anything on a successful run: both probes launch at an error-only log
// level.
func run(ctx context.Context, exe string, env []string, args []string) (time.Duration, error) {
	assert.IsNotNil(ctx, "a probe process runs under a context")
	assert.Assert(exe != "", "a probe process names the binary it launches")
	assert.Assert(len(args) > 0, "a probe process is launched with arguments", exe)

	cmd := exec.CommandContext(ctx, exe, args...)
	// Added to this process's environment rather than replacing it, the way every other child here
	// is launched.
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

// rate turns a frame count and the time it took into frames per second.
//
// A run of no measurable time is refused rather than divided by.
// The clock could not tell its two ends apart, which says nothing about how fast the encoder is,
// and the figure every frame-rate warning is judged against is worth nothing as a guess.
func rate(frames int, elapsed time.Duration) (float64, error) {
	assert.Assert(frames > 0, "a rate counts at least one frame", frames)

	if elapsed <= 0 {
		return 0, fmt.Errorf("the encode probe finished with no measurable time in it")
	}

	fps := float64(frames) / elapsed.Seconds()
	assert.Assert(fps > 0, "a rate over measurable time is positive", fps, frames, elapsed)
	return fps, nil
}
