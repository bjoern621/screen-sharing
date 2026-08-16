package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// GPU engine time below this over a whole measurement is a run that did not reach the silicon.
// A hardware encoder that never submits work is the defect this catches.
const engineFloorNs = 1_000_000

// runEncode times the configured encoder on generated frames and holds what it did against what
// the settings asked for.
//
// The measurement needs no screen, so it reaches every codec on the machine without a capture
// consent, and it runs the encoder the form named rather than a stand-in.
func runEncode(ctx context.Context, run *session, rng *rand.Rand, until time.Time) error {
	families, err := run.families(ctx)
	runnable := run.usable()
	if err != nil {
		return err
	}

	settings, err := run.settled(ctx)
	if err != nil {
		return err
	}
	form, err := run.resolve(ctx, settings)
	if err != nil {
		return err
	}
	settings = form.GetSettings()

	for iteration := 0; time.Now().Before(until); iteration++ {
		if ctx.Err() != nil {
			return nil
		}
		run.report.setIteration(iteration)
		run.report.progress("")

		// A few moves between measurements, so consecutive runs are not one setting apart.
		for i := 0; i < 1+rng.Intn(4); i++ {
			moves := mutables(form, frozen)
			if len(moves) == 0 {
				break
			}
			draft := proto.Clone(settings).(*v1.Settings)
			if _, err := mutate(rng, draft, moves[rng.Intn(len(moves))]); err != nil {
				continue
			}
			next, err := run.resolve(ctx, draft)
			if err != nil {
				break
			}
			form, settings = next, next.GetSettings()
		}

		if !form.GetPublishable() {
			continue
		}

		codec := run.codecOf(settings)
		family := families[codec]
		before := sampleTree(run.backendPid)
		engines := watchEngines(run.backendPid)

		var rate *v1.EncodeRate
		start := time.Now()
		err := withTimeout(ctx, 90*time.Second, func(call context.Context) error {
			answer, err := run.control.MeasureEncodeRate(call, &v1.MeasureEncodeRateRequest{Settings: settings})
			rate = answer.GetRate()
			return err
		})
		took := time.Since(start)
		after := sampleTree(run.backendPid)
		delta := diff(before, after)
		delta.PipelineNs = engines.stop()

		fields := map[string]string{
			"codec":     codec,
			"family":    family,
			"capture":   readField(settings, "publish.capture"),
			"mode":      readField(settings, "publish.mode"),
			"chroma":    readField(settings, "publish.chroma"),
			"fps":       readField(settings, "publish.fps"),
			"bitrate":   readField(settings, "publish.bitrate_mbps"),
			"effort":    readField(settings, "publish.effort"),
			"took":      took.Round(time.Millisecond).String(),
			"cores":     fmt.Sprintf("%.2f", delta.cores()),
			"engine_ns": fmt.Sprint(delta.engineTotal()),
			"engines":   fmt.Sprint(delta.PipelineNs),
		}

		if err != nil {
			if !run.alive() {
				run.report.report("backend.gone", "backend/gone",
					fmt.Sprintf("the backend died measuring %s: %v", codec, err), fields, settings)
				return fmt.Errorf("the backend is gone after measuring %s: %w", codec, err)
			}
			// The probe is what a form greys against, so an encoder it passed and the machine cannot
			// run is the probe answering for a codec it never really started.
			kind := "encode.measure_failed"
			if runnable[codec] && status.Code(err) != codes.Canceled && status.Code(err) != codes.DeadlineExceeded {
				kind = "encode.probed_usable_but_unrunnable"
			}
			run.report.report(kind, "encode/"+codec+"/"+status.Code(err).String(),
				err.Error(), fields, settings)
			continue
		}

		checkRate(run, codec, family, rate, delta, fields, settings)
	}
	return nil
}

// checkRate holds one measurement to what the settings asked for.
func checkRate(run *session, codec, family string, rate *v1.EncodeRate, delta treeDelta, fields map[string]string, settings *v1.Settings) {
	fields["low_fps"] = fmt.Sprintf("%.1f", rate.GetLowFps())
	fields["high_fps"] = fmt.Sprintf("%.1f", rate.GetHighFps())

	if rate.GetLowFps() <= 0 {
		run.report.report("encode.no_rate", "encode/zero-rate/"+codec,
			"the encoder was timed at no frame rate at all", fields, settings)
		return
	}
	if rate.GetHighFps() < rate.GetLowFps() {
		run.report.report("encode.inverted_bracket", "encode/inverted/"+codec,
			fmt.Sprintf("the bracket is %.1f..%.1f", rate.GetLowFps(), rate.GetHighFps()), fields, settings)
	}

	encodeNs := delta.PipelineNs["drm-engine-enc"] + delta.PipelineNs["drm-engine-vcn_enc"] + delta.PipelineNs["drm-engine-video"]
	switch {
	case family == "" || family == "software":
		if encodeNs > engineFloorNs {
			run.report.report("encode.software_used_gpu", "encode/gpu-on-software/"+codec,
				fmt.Sprintf("a software encoder ran %d ns of GPU encode engine", encodeNs), fields, settings)
		}
		if delta.cores() < 0.2 {
			run.report.report("encode.software_idle_cpu", "encode/idle-cpu/"+codec,
				fmt.Sprintf("a software encode held %.2f cores", delta.cores()), fields, settings)
		}
	default:
		if encodeNs < engineFloorNs {
			run.report.report("encode.hardware_never_reached_gpu", "encode/no-gpu/"+codec,
				fmt.Sprintf("a %s encoder was timed at %.1f fps and submitted %d ns of GPU encode work",
					family, rate.GetLowFps(), encodeNs), fields, settings)
		}
	}
	run.report.pass()
}

// runMulti measures what a second, third and fourth encode on this machine costs the first.
//
// The synthetic publishers are the load: each runs its own software encoder, so the ramp says what
// competing encodes do to the one being timed, and a hardware encoder that shares no silicon with
// them should barely move.
func runMulti(ctx context.Context, run *session, rng *rand.Rand, until time.Time) error {
	families, err := run.families(ctx)
	if err != nil {
		return err
	}

	settings, err := run.settled(ctx)
	if err != nil {
		return err
	}
	form, err := run.resolve(ctx, settings)
	if err != nil {
		return err
	}
	settings = form.GetSettings()

	for iteration := 0; time.Now().Before(until); iteration++ {
		if ctx.Err() != nil {
			return nil
		}
		run.report.setIteration(iteration)

		// A codec per ramp, so the run covers what each engine does under competition.
		// Both fields, an encoder being addressed by the pair: the format alone would leave whichever
		// encoder the draft arrived on producing it.
		codecs := run.codecNames()
		draft := proto.Clone(settings).(*v1.Settings)
		if len(codecs) > 0 {
			format, encoder, ok := run.codecPair(codecs[rng.Intn(len(codecs))])
			if !ok {
				return fmt.Errorf("the catalog offered an encoder it does not carry")
			}
			if err := setOption(draft, "publish.format", format); err != nil {
				return err
			}
			if err := setOption(draft, "publish.encoder", encoder); err != nil {
				return err
			}
		}
		// A rate every encoder takes. What this mode measures is what competing encodes cost, and a
		// draft carrying a rate one of them refuses would measure nothing at all.
		if err := setNumber(draft, "publish.bitrate_mbps", 8); err != nil {
			return err
		}
		if err := setNumber(draft, "publish.maxrate_mbps", 8); err != nil {
			return err
		}
		next, err := run.resolve(ctx, draft)
		if err != nil {
			return err
		}
		form, settings = next, next.GetSettings()
		if !form.GetPublishable() {
			continue
		}

		codec := run.codecOf(settings)
		family := families[codec]

		var baseline float64
		for _, load := range run.ramp {
			if ctx.Err() != nil {
				return nil
			}
			if err := run.setLoad(ctx, load); err != nil {
				run.report.report("multi.load_refused", fmt.Sprintf("multi/load/%d", load),
					err.Error(), map[string]string{"streams": fmt.Sprint(load)}, nil)
				continue
			}
			// The publishers that were running have to be gone before the next reading, or the ramp
			// prices the machine's recovery instead of its load.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(10 * time.Second):
			}

			before := sampleTree(run.backendPid)
			engines := watchEngines(run.backendPid)
			var rate *v1.EncodeRate
			err := withTimeout(ctx, 120*time.Second, func(call context.Context) error {
				answer, err := run.control.MeasureEncodeRate(call, &v1.MeasureEncodeRateRequest{Settings: settings})
				rate = answer.GetRate()
				return err
			})
			delta := diff(before, sampleTree(run.backendPid))
			delta.PipelineNs = engines.stop()

			fields := map[string]string{
				"codec": codec, "family": family,
				"streams": fmt.Sprint(load),
				"cores":   fmt.Sprintf("%.2f", delta.cores()),
				"engines": fmt.Sprint(delta.PipelineNs),
			}
			if err != nil {
				run.report.report("multi.measure_failed", "multi/"+codec+"/"+status.Code(err).String(),
					err.Error(), fields, settings)
				continue
			}

			fields["low_fps"] = fmt.Sprintf("%.1f", rate.GetLowFps())
			if load == run.ramp[0] {
				baseline = rate.GetLowFps()
			}
			if baseline > 0 {
				share := rate.GetLowFps() / baseline
				fields["of_baseline"] = fmt.Sprintf("%.2f", share)
				run.curve(codec, family, load, rate.GetLowFps(), share, delta)
			}
		}
		if err := run.setLoad(ctx, 0); err != nil {
			return err
		}
	}
	return nil
}
