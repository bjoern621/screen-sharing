// The soak probe: it drives the control contract with random legal settings and states what the
// backend did about them.
//
// Three questions, one binary:
//
//	form     what the resolver owes every draft: repairs settle, an offered option is legal, a
//	         greying names a reason, a publishable draft renders a command.
//	encode   what an encoder does on the machine: a hardware family reaches the GPU encode engine
//	         and a software one does not, and the timing brackets are not zero.
//	publish  what a running stream reports: frames arrive at the rate that was asked for, the
//	         bitrate lands near the ceiling, nothing is dropped, nothing retries, and a stop leaves
//	         no child behind.
//	multi    what a second and third encode cost the first.
//
// It talks to whatever backend the endpoint names, so a run that must not touch the user's own
// instance points XDG_RUNTIME_DIR and XDG_CONFIG_HOME at a directory of its own.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// What one run holds: the connection, where findings go, and the process being watched.
type session struct {
	control    v1.ControlServiceClient
	conn       *grpc.ClientConn
	report     *reporter
	backendPid int
	ramp       []int
	cover      *coverage

	mu      sync.Mutex
	catalog *v1.Catalog
}

func main() {
	mode := flag.String("mode", "form", "form, encode, publish or multi")
	seed := flag.Int64("seed", time.Now().UnixNano(), "random seed, so a finding can be reproduced")
	runFor := flag.Duration("for", 10*time.Minute, "how long to probe")
	out := flag.String("out", "soak-findings.jsonl", "where findings are written")
	sock := flag.String("socket", "", "control socket, empty for the one XDG_RUNTIME_DIR names")
	pid := flag.Int("backend-pid", 0, "the backend process to watch, 0 to find it by socket owner")
	verbose := flag.Bool("v", false, "print every finding as it lands")
	ramp := flag.String("ramp", "0,1,3,6,9", "test-stream counts the multi mode measures at")
	publishFor := flag.Duration("publish-for", 25*time.Second, "how long one publish is held in publish mode")
	reset := flag.Bool("reset", true, "stop whatever is publishing before probing")
	dump := flag.String("dump", "", "print what the form says about one field key, then stop")
	capture := flag.String("capture", "", "capture backend a publish run holds, empty for the stored one")
	codec := flag.String("codec", "", "encoder a publish run holds, empty to let the walk move it")
	flag.Parse()

	if *sock == "" {
		*sock = socketPath()
	}

	rng := rand.New(rand.NewSource(*seed))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, control, err := dial(*sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach the control endpoint:", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := waitReady(ctx, control, 60*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "the backend at %s did not answer a handshake: %v\n", *sock, err)
		os.Exit(1)
	}

	watched := *pid
	if watched == 0 {
		watched = findBackend(*sock)
	}
	if watched == 0 {
		fmt.Fprintln(os.Stderr, "no backend process found to watch: pass -backend-pid")
		os.Exit(1)
	}

	reports, err := newReporter(*out, *seed, *verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot write findings:", err)
		os.Exit(1)
	}
	defer reports.close()

	run := &session{control: control, conn: conn, report: reports, backendPid: watched,
		ramp: parseRamp(*ramp), cover: newCoverage()}

	fmt.Printf("soak %s: seed %d, backend pid %d, socket %s, findings in %s\n",
		*mode, *seed, watched, *sock, *out)

	// A stream and its synthetic publishers outlive whatever started them, so a run that follows an
	// interrupted one opens on what that one left. A measurement is refused while anything
	// publishes, which would be the whole run.
	if *reset {
		_ = withTimeout(ctx, 30*time.Second, func(call context.Context) error {
			_, err := control.StopPublish(call, &v1.StopPublishRequest{})
			return err
		})
		_ = run.setLoad(ctx, 0)
	}

	// The probe once, as a shell owes: without it no codec is greyed for missing hardware, and the
	// run would spend itself measuring encoders this machine has no silicon for.
	if err := run.probeEncoders(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "the encoder probe did not finish:", err)
	}

	if *dump != "" {
		if err := run.dumpField(ctx, *dump); err != nil {
			fmt.Fprintln(os.Stderr, "cannot describe the field:", err)
			os.Exit(1)
		}
		return
	}

	until := time.Now().Add(*runFor)
	watchdog := run.watchMemory(ctx, until)

	switch *mode {
	case "form":
		err = runForm(ctx, run, rng, until)
	case "encode":
		err = runEncode(ctx, run, rng, until)
	case "publish":
		err = runPublish(ctx, run, rng, until, *publishFor, *capture, *codec)
	case "multi":
		err = runMulti(ctx, run, rng, until)
	default:
		err = fmt.Errorf("no such mode: %s", *mode)
	}

	<-watchdog
	fmt.Print(reports.summary())
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "run ended:", err)
		os.Exit(1)
	}
}

func parseRamp(spec string) []int {
	var out []int
	for _, part := range strings.Split(spec, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		out = []int{0, 1, 3}
	}
	return out
}

// findBackend is the process listening on the socket, which is what a memory reading is taken from.
//
// The socket carries no owner, so the match is on the executable a backend runs as, nearest first:
// a run that started its own backend passes -backend-pid and reaches none of this.
func findBackend(sock string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	best := 0
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile("/proc/" + entry.Name() + "/cmdline")
		if err != nil || len(raw) == 0 {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		if !strings.HasSuffix(argv[0], "/backend") && argv[0] != "backend" {
			continue
		}
		if len(argv) > 1 && argv[1] == "gst-publish" {
			continue
		}
		if pid > best {
			best = pid
		}
	}
	return best
}

func (s *session) alive() bool {
	_, ok := readStat(s.backendPid)
	return ok
}

func (s *session) settled(ctx context.Context) (*v1.Settings, error) {
	var settings *v1.Settings
	err := withTimeout(ctx, 10*time.Second, func(call context.Context) error {
		answer, err := s.control.GetSettings(call, &v1.GetSettingsRequest{})
		settings = answer.GetSettings()
		return err
	})
	return settings, err
}

func (s *session) resolve(ctx context.Context, settings *v1.Settings) (*v1.Form, error) {
	var form *v1.Form
	err := withTimeout(ctx, 20*time.Second, func(call context.Context) error {
		answer, err := s.control.ResolveForm(call, &v1.ResolveFormRequest{Settings: settings})
		form = answer.GetForm()
		return err
	})
	return form, err
}

func (s *session) fetchCatalog(ctx context.Context) (*v1.Catalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog != nil {
		return s.catalog, nil
	}
	err := withTimeout(ctx, 20*time.Second, func(call context.Context) error {
		answer, err := s.control.GetCatalog(call, &v1.GetCatalogRequest{})
		s.catalog = answer.GetCatalog()
		return err
	})
	return s.catalog, err
}

// families maps an encoder to the silicon it claims, which is what a GPU reading is held against.
func (s *session) families(ctx context.Context) (map[string]string, error) {
	catalog, err := s.fetchCatalog(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, codec := range catalog.GetCodecs() {
		out[codec.GetName()] = codec.GetFamily()
	}
	return out, nil
}

// codecOf is the encode a draft names, as the catalog spells it: the row its format and its encoder
// address between them.
// Empty for a pair no row carries, which is what the form greys and the repair walks off.
func (s *session) codecOf(settings *v1.Settings) string {
	format, encoder := readField(settings, "publish.format"), readField(settings, "publish.encoder")
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, codec := range s.catalog.GetCodecs() {
		if codec.GetFormat() == format && codec.GetEncoder() == encoder {
			return codec.GetName()
		}
	}
	return ""
}

// codecPair is the two fields a draft names one encoder by, and false for a name the catalog does not
// carry.
// The run names an encoder the way a person does, and the settings hold the pair, so the translation
// is here rather than on the command line.
func (s *session) codecPair(name string) (format, encoder string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, codec := range s.catalog.GetCodecs() {
		if codec.GetName() == name {
			return codec.GetFormat(), codec.GetEncoder(), true
		}
	}
	return "", "", false
}

func (s *session) codecNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, codec := range s.catalog.GetCodecs() {
		if codec.GetImplemented() {
			out = append(out, codec.GetName())
		}
	}
	return out
}

// setLoad puts the named number of synthetic publishers on the machine.
func (s *session) setLoad(ctx context.Context, count int) error {
	return withTimeout(ctx, 30*time.Second, func(call context.Context) error {
		if count == 0 {
			_, err := s.control.StopTestStreams(call, &v1.StopTestStreamsRequest{})
			return err
		}
		_, err := s.control.StartTestStreams(call, &v1.StartTestStreamsRequest{Count: int32(count)})
		return err
	})
}

// curve records one point of the degradation ramp.
func (s *session) curve(codec, family string, load int, fps, share float64, delta treeDelta) {
	fields := map[string]string{
		"codec": codec, "family": family,
		"streams":     fmt.Sprint(load),
		"low_fps":     fmt.Sprintf("%.1f", fps),
		"of_baseline": fmt.Sprintf("%.2f", share),
		"cores":       fmt.Sprintf("%.2f", delta.cores()),
		"engines":     fmt.Sprint(delta.EngineNs),
	}
	s.report.report("multi.point", fmt.Sprintf("multi/point/%s/%d", codec, load),
		fmt.Sprintf("%s at %d competing streams: %.1f fps, %.0f%% of the ramp's first reading",
			codec, load, fps, share*100), fields, nil)
}

// watchMemory reads the backend's process tree on an interval and reports what it never gives back.
//
// The reading is the whole tree, so a publish child that is never reaped counts as much as a heap
// that grows: both are the process holding what it stopped needing.
func (s *session) watchMemory(ctx context.Context, until time.Time) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		// base is the first reading past startup and never moves, so the drift a run ends on is
		// measured against where it opened.
		// settled moves to each reading that was reported, which is what keeps one climb from being
		// reported again on every tick that follows it.
		var base, settled, latest *treeSample
		defer func() { s.reportDrift(base, latest) }()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if time.Now().After(until) {
				return
			}
			if !s.alive() {
				s.report.report("backend.gone", "backend/gone",
					fmt.Sprintf("the backend process %d is no longer there", s.backendPid), nil, nil)
				return
			}

			sample := sampleTree(s.backendPid)
			latest = &sample
			// The first minute is startup, so the baseline is taken after it rather than at zero.
			if settled == nil {
				if time.Since(s.report.started) > time.Minute {
					settled, base = &sample, &sample
				}
				continue
			}

			grown := sample.RootRSSKiB - settled.RootRSSKiB
			elapsed := sample.At.Sub(settled.At).Minutes()
			// A tenth of a gigabyte an hour is a leak an all-day share meets and a cache does not, so
			// the bar sits there rather than at the point where the machine is already in trouble.
			if elapsed >= 5 && grown > 96*1024 && float64(grown)/elapsed > 8*1024 {
				s.report.report("backend.memory_growth", "backend/rss",
					fmt.Sprintf("the backend holds %d MiB more than %.0f minutes ago, %0.1f MiB a minute",
						grown/1024, elapsed, float64(grown)/1024/elapsed),
					map[string]string{
						"rss_mib":      fmt.Sprint(sample.RootRSSKiB / 1024),
						"base_mib":     fmt.Sprint(settled.RootRSSKiB / 1024),
						"tree_rss_mib": fmt.Sprint(sample.RSSKiB / 1024),
						"pids":         fmt.Sprint(sample.Pids),
						"fds":          fmt.Sprint(sample.RootFDs),
						"threads":      fmt.Sprint(sample.RootThreads),
					}, nil)
				settled = &sample
			}
			if sample.RootFDs-settled.RootFDs > 200 {
				s.report.report("backend.descriptor_growth", "backend/fds",
					fmt.Sprintf("the backend holds %d more descriptors than it did", sample.RootFDs-settled.RootFDs),
					map[string]string{"fds": fmt.Sprint(sample.RootFDs), "tree_fds": fmt.Sprint(sample.FDs)}, nil)
				settled = &sample
			}
			if sample.RootThreads-settled.RootThreads > 100 {
				s.report.report("backend.thread_growth", "backend/threads",
					fmt.Sprintf("the backend runs %d more threads than it did", sample.RootThreads-settled.RootThreads),
					map[string]string{"threads": fmt.Sprint(sample.RootThreads),
						"tree_threads": fmt.Sprint(sample.Threads)}, nil)
				settled = &sample
			}
		}
	}()
	return done
}

// reportDrift states what the process tree held at the end of a run against what it held at the
// start, whether or not the climb was steep enough to be reported while it happened.
//
// A threshold answers yes or no, and what a leak hunt needs is the figure: a run drifting 3 MiB a
// minute passes every bar here and is a gigabyte over a working day.
func (s *session) reportDrift(base, last *treeSample) {
	if base == nil || last == nil {
		return
	}
	elapsed := last.At.Sub(base.At).Minutes()
	if elapsed < 1 {
		return
	}

	grown := last.RootRSSKiB - base.RootRSSKiB
	s.report.report("backend.drift", "backend/drift",
		fmt.Sprintf("over %.0f minutes the backend went from %d to %d MiB, %.2f MiB a minute, from %d to %d descriptors and %d to %d threads",
			elapsed, base.RootRSSKiB/1024, last.RootRSSKiB/1024, float64(grown)/1024/elapsed,
			base.RootFDs, last.RootFDs, base.RootThreads, last.RootThreads),
		map[string]string{
			"minutes":      fmt.Sprintf("%.0f", elapsed),
			"base_mib":     fmt.Sprint(base.RootRSSKiB / 1024),
			"rss_mib":      fmt.Sprint(last.RootRSSKiB / 1024),
			"mib_per_min":  fmt.Sprintf("%.2f", float64(grown)/1024/elapsed),
			"fds":          fmt.Sprint(last.RootFDs),
			"base_fds":     fmt.Sprint(base.RootFDs),
			"threads":      fmt.Sprint(last.RootThreads),
			"base_threads": fmt.Sprint(base.RootThreads),
			// The tree beside it, so a figure moved by a pipeline that happened to be up reads as one.
			"tree_rss_mib": fmt.Sprint(last.RSSKiB / 1024),
			"pids":         fmt.Sprint(last.Pids),
		}, nil)
}

// probeEncoders runs the probe and takes the catalog it lands, so what the run measures is what
// this machine was found able to run.
func (s *session) probeEncoders(ctx context.Context) error {
	err := withTimeout(ctx, 3*time.Minute, func(call context.Context) error {
		_, err := s.control.ProbeEncoders(call, &v1.ProbeEncodersRequest{})
		return err
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.catalog = nil
	s.mu.Unlock()
	_, err = s.fetchCatalog(ctx)
	return err
}

// usable is what the probe found each engine able to run, keyed by codec.
// A codec absent from every engine's map was reached by no probe at all.
func (s *session) usable() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]bool{}
	for _, engine := range s.catalog.GetEncoders().GetEngines() {
		for codec, ok := range engine.GetProbed().GetUsable() {
			if ok {
				out[codec] = true
			}
		}
	}
	return out
}

// dumpField prints what a resolve says about one control: the value it holds, the ends it is offered
// between and the entries beside them.
//
// A control's shape is what a reader sees, and a probe that only asserts about it leaves nobody able
// to look.
func (s *session) dumpField(ctx context.Context, key string) error {
	settings, err := s.settled(ctx)
	if err != nil {
		return err
	}
	form, err := s.resolve(ctx, settings)
	if err != nil {
		return err
	}

	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			if field.GetKey() != key {
				continue
			}
			fmt.Printf("%s\n  control %v, visible %t, enabled %t\n  holds %s\n",
				field.GetKey(), field.GetControl(), field.GetVisible(), field.GetEnabled(),
				readField(form.GetSettings(), key))
			if r := field.GetRange(); r != nil {
				fmt.Printf("  range %d..%d step %d\n", r.GetMin(), r.GetMax(), r.GetStep())
			}
			for _, option := range field.GetOptions() {
				mark := " "
				if option.GetValue() == readField(form.GetSettings(), key) {
					mark = "*"
				}
				fmt.Printf("  %s entry %-8s enabled %t\n", mark, option.GetValue(), option.GetEnabled())
			}
			return nil
		}
	}
	return fmt.Errorf("the form carries no field %q", key)
}
