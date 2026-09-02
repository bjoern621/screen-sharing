package publish

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// GstExe is the launcher the measuring pipelines are played by, resolved through FindGstExe.
// A publish plays in a process of this app's own instead (FindGstRunner).
const GstExe = "gst-launch-1.0"

// gstEngine runs one GStreamer capture backend: capture, encode and transport in one pipeline and
// one process.
//
// The backend is a field rather than a branch, keeping framework and source on separate axes:
// a source only one framework has an element for is a backend only that engine is instantiated
// with.
type gstEngine struct {
	capture gstCapture
}

func (g gstEngine) Command(s settings.Settings) (string, error) {
	opts, err := g.captureOptions(s)
	if err != nil {
		return "", err
	}
	// A run's own branches, the meter and the preview, are left out.
	// Each carries a port the kernel handed out for one launch, so a rendered command showing them
	// would differ between two renderings of one pipeline, and whether two settings build one pipeline
	// is decided by comparing exactly that string (SamePipeline).
	pipeline, err := buildPipeline(s, g.capture.Describe(s, opts), "", PreviewLeg{})
	if err != nil {
		return "", err
	}
	// Runner and subcommand lead the line, and everything after the subcommand is the pipeline
	// gst-launch-1.0 takes unchanged.
	exe, err := FindGstRunner()
	if err != nil {
		return "", err
	}
	return exe + " " + GstSubcommand + " " + strings.Join(pipeline, " "), nil
}

// captureOptions builds the run-independent parts of the source chain and holds them against this
// machine.
//
// The device check runs here rather than at Start, so a rendered command is refused for the same
// reason a publish is: a machine whose capture and encode ends are not one GPU would otherwise be
// shown a command the button beside it rejects.
func (g gstEngine) captureOptions(s settings.Settings) (gstCaptureOptions, error) {
	opts, err := gstSourceOptions(s)
	if err != nil {
		return gstCaptureOptions{}, err
	}
	if gpupath.OnDevice(opts.Memory) {
		if err := g.capture.HoldsOneDevice(); err != nil {
			return gstCaptureOptions{}, err
		}
	}
	return opts, nil
}

func (gstEngine) Engine() string {
	return EngineGst
}

// Release drops what the capture backend holds between launches, and does nothing for one
// that holds nothing.
func (g gstEngine) Release() {
	if holder, ok := g.capture.(sourceHolder); ok {
		holder.Release()
	}
}

// Carries reports whether the transport terminates a GStreamer publish pipeline.
func (gstEngine) Carries(transportName string) bool {
	return transport.CanPublish(transportName, EngineGst)
}

func (g gstEngine) Start(s settings.Settings, tag string, preview PreviewLeg, cb Callbacks) (Handle, error) {
	// Validated before anything is acquired, so a combination this engine cannot encode and a machine
	// whose two ends are not one GPU never pop the compositor's picker.
	opts, err := g.captureOptions(s)
	if err != nil {
		return nil, err
	}

	// The runner is this executable, so nothing is looked up on the machine ahead of the picker.
	// Which elements the linked GStreamer registers is the encoder probe's question.
	exe, err := FindGstRunner()
	if err != nil {
		return nil, err
	}

	// Instrumentation belongs to a run and not to the pipeline: without OnStats the child runs what
	// the displayed command reads, rate probe included.
	// The meter's socket is open from here on, and the port it landed on is what the meter branch
	// is pointed at.
	// A connection back to this process rather than an inherited descriptor, a Windows child
	// inheriting none.
	var meter *gstMeter
	meterArg := ""
	if cb.OnStats != nil {
		meter, err = newGstMeter(cb.OnStats)
		if err != nil {
			return nil, fmt.Errorf("progress meter: %w", err)
		}
		meterArg = meter.port()
		opts.RateProbe = gstCaptureProbe
	}

	source, files, closeSource, err := g.capture.Open(s, opts)
	if err != nil {
		meter.close()
		return nil, err
	}

	pipeline, err := buildPipeline(s, source, meterArg, preview)
	if err != nil {
		meter.close()
		closeSource()
		return nil, err
	}

	// The child's standard output carries the meter's progress and the caps the capture negotiated,
	// and the caps decide whether this publish may continue at all.
	//
	// The reader starts the moment the process does and the handle exists only once supervise
	// returns, so caps arriving before that find it nil and stop nothing.
	// The caps follow the pipeline reaching PLAYING, so they do not arrive before it.
	//
	// That is an argument about timing, not about the memory model:
	// the handle is written here and read on the reader goroutine,
	// so the mutex makes the write visible to that read.
	// stopped needs none, being written and read inside this callback alone, which is one goroutine.
	var handleMu sync.Mutex
	var handle Handle
	stopped := false
	parseStdout := func(r io.Reader) {
		gstReadChild(r, meter, func(caps string) {
			if stopped {
				return
			}
			refusal := hdrRefusal(s, caps)
			if refusal == nil {
				return
			}
			stopped = true
			logger.Warnf("stopping the publish: %v", refusal)
			handleMu.Lock()
			running := handle
			handleMu.Unlock()
			if running != nil {
				running.Stop()
			}
		}, cb.OnPointer)
	}

	// One socket per run, opened by the child before its pipeline plays and removed with it.
	// A run nobody can talk to is a stream that has to be relaunched to change, which costs every
	// viewer a reconnect.
	socket := gstControlSocket(tag)

	started, err := supervise(superviseConfig{
		exe: exe,
		env: GstChildEnv(),
		// The subcommand leads, so this executable plays a pipeline rather than starting a second
		// backend (cmd/backend).
		// The control flag follows it, so the pipeline starts at the word gst-launch would start at.
		args: append(append([]string{GstSubcommand}, gstChildArgs(s, socket, meterArg != "")...), pipeline...),
		tag:  tag,
		// The pipeline spells the relay token and the SRT passphrase out in full, and the log is a file
		// the app offers to open and a reader forwards.
		redact:      func(text string) string { return transport.Redact(s, text) },
		extraFiles:  files,
		parseStdout: parseStdout,
		onExit:      cb.OnExit,
		onCleanup: func() {
			closeSource()
			meter.close()
		},
	})
	if err != nil {
		meter.close()
		closeSource()
		return nil, err
	}

	handleMu.Lock()
	handle = started
	handleMu.Unlock()

	return &gstHandle{Handle: started, socket: socket}, nil
}
