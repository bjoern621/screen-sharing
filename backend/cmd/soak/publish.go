package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// What the event stream said about one stream, gathered while it ran.
type watcher struct {
	mu       sync.Mutex
	stats    []*v1.PublishStats
	exits    []string
	attempts int32
	live     bool
	preview  bool
}

func (w *watcher) add(event *v1.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case event.GetPublishStats() != nil:
		w.stats = append(w.stats, event.GetPublishStats())
	case event.GetPublishExit() != nil:
		w.exits = append(w.exits, event.GetPublishExit().GetMessage())
	case event.GetPublishState() != nil:
		live := event.GetPublishState().GetLive()
		w.live = live != nil
		w.preview = live.GetPreview() != nil
		if attempt := live.GetRetry().GetAttempt(); attempt > w.attempts {
			w.attempts = attempt
		}
	}
}

func (w *watcher) reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stats, w.exits, w.attempts = nil, nil, 0
}

func (w *watcher) read() ([]*v1.PublishStats, []string, int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]*v1.PublishStats(nil), w.stats...), append([]string(nil), w.exits...), w.attempts
}

// runPublish puts real streams on the air and holds what they report against what was asked for.
//
// The capture stays where the run put it: a backend told to capture through the portal pops a
// consent picker on somebody's screen, which is not a thing a probe may do.
func runPublish(ctx context.Context, run *session, rng *rand.Rand, until time.Time, hold time.Duration, capture string) error {
	families, err := run.families(ctx)
	if err != nil {
		return err
	}

	events := &watcher{}
	go run.subscribe(ctx, events)

	settings, err := run.settled(ctx)
	if err != nil {
		return err
	}
	form, err := run.resolve(ctx, settings)
	if err != nil {
		return err
	}
	settings = form.GetSettings()

	// The capture decides the engine, and the two instrument what they run differently, so a run
	// that never left one of them would report half the product.
	if capture != "" {
		if err := setOption(settings, "publish.capture", capture); err != nil {
			return err
		}
		if form, err = run.resolve(ctx, settings); err != nil {
			return err
		}
		settings = form.GetSettings()
	}

	held := map[string]bool{"publish.capture": true}
	for key := range frozen {
		held[key] = true
	}

	baseline := proto.Clone(settings).(*v1.Settings)
	base := sampleTree(run.backendPid)

	for iteration := 0; time.Now().Before(until); iteration++ {
		if ctx.Err() != nil {
			return nil
		}
		run.report.setIteration(iteration)
		run.report.progress("")

		for i := 0; i < 1+rng.Intn(4); i++ {
			moves := mutables(form, held)
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

		events.reset()
		ok, err := run.oneStream(ctx, events, settings, families, hold, base)
		if err != nil {
			return err
		}
		// A start persists the draft it was given, so a stream that died leaves the settings it died
		// on as the ones the next walk moves from. Back to the draft the run opened with instead of
		// spending the rest of it in one dead corner.
		if !ok {
			settings = proto.Clone(baseline).(*v1.Settings)
			form, err = run.resolve(ctx, settings)
			if err != nil {
				return err
			}
			settings = form.GetSettings()
		}
	}
	return nil
}

// oneStream starts a stream, watches it for as long as the run holds it, and states what it did.
func (s *session) oneStream(ctx context.Context, events *watcher, settings *v1.Settings, families map[string]string, hold time.Duration, base treeSample) (bool, error) {
	codec := readField(settings, "publish.codec")
	family := families[codec]
	target := numeric(readField(settings, "publish.fps"))
	ceiling := numeric(readField(settings, "publish.bitrate_mbps"))
	mode := readField(settings, "publish.mode")

	fields := map[string]string{
		"codec": codec, "family": family, "mode": mode,
		"transport": readField(settings, "publish.publish_transport"),
		"capture":   readField(settings, "publish.capture"),
		"chroma":    readField(settings, "publish.chroma"),
		"memory":    readField(settings, "publish.capture_memory"),
		"fps":       fmt.Sprintf("%.0f", target),
		"bitrate":   fmt.Sprintf("%.0f", ceiling),
	}

	err := withTimeout(ctx, 30*time.Second, func(call context.Context) error {
		_, err := s.control.StartPublish(call, &v1.StartPublishRequest{Settings: settings})
		return err
	})
	if err != nil {
		kind := "publish.start_refused"
		if status.Code(err) == codes.Unavailable {
			kind = "backend.unavailable"
		}
		s.report.report(kind, "publish/start/"+codec+"/"+status.Code(err).String(), err.Error(), fields, settings)
		if !s.alive() {
			return false, fmt.Errorf("the backend is gone after starting %s", codec)
		}
		// A stream that died is retried for as long as its budget lasts, and every start made while it
		// waits is refused. Stopping here is what keeps one failure from being the rest of the run.
		_ = withTimeout(ctx, 20*time.Second, func(call context.Context) error {
			_, err := s.control.StopPublish(call, &v1.StopPublishRequest{})
			return err
		})
		return false, nil
	}

	// The same stream again is the state that already holds, so it succeeds and changes nothing.
	err = withTimeout(ctx, 15*time.Second, func(call context.Context) error {
		_, err := s.control.StartPublish(call, &v1.StartPublishRequest{Settings: settings})
		return err
	})
	if err != nil {
		s.report.report("publish.restart_not_idempotent", "publish/restart/"+status.Code(err).String(),
			fmt.Sprintf("starting the running stream again answered %s", err), fields, settings)
	}

	// A different pipeline over a running one is refused: that stream is one the user asked for and
	// has not stopped.
	//
	// What counts as different is the command the form renders, not the field that was moved: a
	// bitrate never reaches a lossless encoder, so a draft carrying another one builds the pipeline
	// already running and is the state that holds.
	other := proto.Clone(settings).(*v1.Settings)
	if err := setNumber(other, "publish.fps", int64(target)+7); err == nil {
		running, err1 := s.resolve(ctx, settings)
		changed, err2 := s.resolve(ctx, other)
		differs := err1 == nil && err2 == nil &&
			running.GetSummary().GetCommand() != "" &&
			running.GetSummary().GetCommand() != changed.GetSummary().GetCommand()

		if differs {
			err = withTimeout(ctx, 15*time.Second, func(call context.Context) error {
				_, err := s.control.StartPublish(call, &v1.StartPublishRequest{Settings: changed.GetSettings()})
				return err
			})
			if code := status.Code(err); code != codes.FailedPrecondition {
				s.report.report("publish.different_start_not_refused", "publish/second-start/"+code.String(),
					fmt.Sprintf("starting a pipeline the form renders differently, over a running one, answered %s", code),
					fields, settings)
			}
		}
	}

	before := sampleTree(s.backendPid)
	engines := watchEngines(s.backendPid)
	select {
	case <-ctx.Done():
	case <-time.After(hold):
	}
	delta := diff(before, sampleTree(s.backendPid))
	delta.PipelineNs = engines.stop()

	stats, exits, attempts := events.read()
	healthy := s.checkStream(stats, exits, attempts, delta, family, mode, target, ceiling, fields, settings)

	// Stopping twice: the second names a state that already holds, so it succeeds and does nothing.
	for i := 0; i < 2; i++ {
		err := withTimeout(ctx, 20*time.Second, func(call context.Context) error {
			_, err := s.control.StopPublish(call, &v1.StopPublishRequest{})
			return err
		})
		if err != nil {
			s.report.report("publish.stop_failed", fmt.Sprintf("publish/stop/%d/%s", i, status.Code(err)),
				err.Error(), fields, settings)
		}
	}

	// What a stop owes: no state, and no child left running.
	time.Sleep(2 * time.Second)
	var state *v1.PublishState
	_ = withTimeout(ctx, 10*time.Second, func(call context.Context) error {
		answer, err := s.control.GetPublishState(call, &v1.GetPublishStateRequest{})
		state = answer
		return err
	})
	if state.GetLive() != nil {
		s.report.report("publish.stop_left_state", "publish/stop-state",
			"a stopped stream is still reported live", fields, settings)
	}
	if leaked := pipelines(s.backendPid); leaked > 0 {
		s.report.report("publish.child_leaked", "publish/child-leak/"+codec,
			fmt.Sprintf("%d pipeline processes are still running after the stop", leaked), fields, settings)
	}
	after := sampleTree(s.backendPid)
	if grown := after.RSSKiB - base.RSSKiB; grown > 400*1024 {
		s.report.report("publish.cycle_memory", "publish/cycle-rss",
			fmt.Sprintf("the tree holds %d MiB more than before the first stream", grown/1024),
			fields, settings)
	}
	return healthy, nil
}

// checkStream holds one run's samples to the settings it was built from.
func (s *session) checkStream(stats []*v1.PublishStats, exits []string, attempts int32, delta treeDelta,
	family, mode string, target, ceiling float64, fields map[string]string, settings *v1.Settings) bool {

	fields["samples"] = fmt.Sprint(len(stats))
	fields["cores"] = fmt.Sprintf("%.2f", delta.cores())
	fields["engines"] = fmt.Sprint(delta.PipelineNs)
	fields["engines_all"] = fmt.Sprint(delta.EngineNs)

	if len(exits) > 0 {
		fields["exit"] = exits[len(exits)-1]
		s.report.report("publish.child_exited", "publish/exit/"+signature(exits[len(exits)-1]),
			exits[len(exits)-1], fields, settings)
	}
	if attempts > 0 {
		s.report.report("publish.retried", "publish/retry/"+fields["codec"],
			fmt.Sprintf("the pipeline died and was relaunched %d times", attempts), fields, settings)
	}
	if len(stats) == 0 {
		s.report.report("publish.no_stats", "publish/silent/"+fields["codec"],
			"a stream was started and reported no progress at all", fields, settings)
		return false
	}

	measured := median(stats, func(s *v1.PublishStats) (float64, bool) {
		return s.GetFps(), s.Fps != nil
	})
	rate := median(stats, func(s *v1.PublishStats) (float64, bool) {
		return s.GetInstMbps(), s.InstMbps != nil
	})
	speed := median(stats, func(s *v1.PublishStats) (float64, bool) {
		return s.GetSpeed(), s.Speed != nil
	})
	last := stats[len(stats)-1]

	fields["measured_fps"] = fmt.Sprintf("%.1f", measured)
	fields["measured_mbps"] = fmt.Sprintf("%.2f", rate)
	fields["speed"] = fmt.Sprintf("%.2f", speed)
	fields["dropped"] = fmt.Sprint(last.GetDroppedFrames())
	fields["frames"] = fmt.Sprint(last.GetFrameCount())

	if last.GetFrameCount() == 0 {
		s.report.report("publish.no_frames", "publish/no-frames/"+fields["codec"],
			"the encoder reported progress and no frame ever left it", fields, settings)
	}
	if measured > 0 && target > 0 && math.Abs(measured-target)/target > 0.25 {
		s.report.report("publish.fps_off_target", "publish/fps/"+fields["codec"],
			fmt.Sprintf("asked for %.0f fps and the encoder emitted %.1f", target, measured), fields, settings)
	}
	if speed > 0 && speed < 0.9 {
		s.report.report("publish.behind_real_time", "publish/speed/"+fields["codec"],
			fmt.Sprintf("the encode ran at %.2f of real time", speed), fields, settings)
	}
	if mode == "cbr" && ceiling > 0 && rate > 0 && (rate > ceiling*1.5 || rate < ceiling*0.5) {
		s.report.report("publish.bitrate_off_target", "publish/bitrate/"+fields["codec"],
			fmt.Sprintf("a constant rate of %.0f Mbit/s was asked for and %.2f measured", ceiling, rate), fields, settings)
	}
	if last.GetFrameCount() > 0 && float64(last.GetDroppedFrames())/float64(last.GetFrameCount()) > 0.05 {
		s.report.report("publish.dropping", "publish/drops/"+fields["codec"],
			fmt.Sprintf("%d of %d frames were dropped", last.GetDroppedFrames(), last.GetFrameCount()), fields, settings)
	}
	// A rate the screen can show: frames left the encoder, so the bytes they cost were measured by
	// something.
	if last.GetFrameCount() > 0 && !carries(stats, func(s *v1.PublishStats) bool { return s.InstMbps != nil || s.AvgMbps != nil }) {
		s.report.report("publish.no_bitrate_reported", "publish/no-bitrate/"+fields["transport"]+"/"+fields["codec"],
			fmt.Sprintf("%d frames were encoded over %d samples and no sample carried a bitrate",
				last.GetFrameCount(), len(stats)), fields, settings)
	}

	encodeNs := delta.PipelineNs["drm-engine-enc"] + delta.PipelineNs["drm-engine-vcn_enc"] + delta.PipelineNs["drm-engine-video"]
	switch {
	case family == "" || family == "software":
		if encodeNs > engineFloorNs {
			s.report.report("publish.software_used_gpu", "publish/gpu-on-software/"+fields["codec"],
				fmt.Sprintf("a software encoder ran %d ns of GPU encode engine", encodeNs), fields, settings)
		}
	default:
		if encodeNs < engineFloorNs {
			s.report.report("publish.hardware_never_reached_gpu", "publish/no-gpu/"+fields["codec"],
				fmt.Sprintf("a %s stream ran %.1f fps and submitted %d ns of GPU encode work",
					family, measured, encodeNs), fields, settings)
		}
	}
	s.report.pass()
	return len(exits) == 0 && attempts == 0 && last.GetFrameCount() > 0
}

// carries says whether any sample answered the figure a check needs.
func carries(stats []*v1.PublishStats, has func(*v1.PublishStats) bool) bool {
	for _, sample := range stats {
		if has(sample) {
			return true
		}
	}
	return false
}

// subscribe holds the event stream for the whole run, reopening it where it drops.
func (s *session) subscribe(ctx context.Context, events *watcher) {
	for ctx.Err() == nil {
		stream, err := s.control.Subscribe(ctx, &v1.SubscribeRequest{})
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		for {
			event, err := stream.Recv()
			if err != nil {
				if err != io.EOF && ctx.Err() == nil {
					s.report.report("events.stream_dropped", "events/dropped/"+status.Code(err).String(),
						err.Error(), nil, nil)
				}
				break
			}
			events.add(event)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// pipelines counts the media children the backend still holds.
func pipelines(root int) int {
	count := 0
	for _, pid := range descendants(root) {
		if pid == root {
			continue
		}
		raw, err := readCmdline(pid)
		if err != nil {
			continue
		}
		if strings.Contains(raw, "gst-launch") || strings.Contains(raw, "gst-publish") || strings.Contains(raw, "ffmpeg") {
			count++
		}
	}
	return count
}

func readCmdline(pid int) (string, error) {
	raw, err := readFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(raw, "\x00", " "), nil
}

func median(stats []*v1.PublishStats, read func(*v1.PublishStats) (float64, bool)) float64 {
	var values []float64
	// The first samples are a pipeline settling, so they say nothing about the rate it holds.
	for i, sample := range stats {
		if i < len(stats)/3 {
			continue
		}
		if value, ok := read(sample); ok && value > 0 {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	return values[len(values)/2]
}

func numeric(text string) float64 {
	var value float64
	fmt.Sscan(text, &value)
	return value
}

// signature keeps a failure's first line, so two runs of one failure are one entry.
func signature(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	if len(line) > 60 {
		line = line[:60]
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '/' {
			return '-'
		}
		return r
	}, line)
}
