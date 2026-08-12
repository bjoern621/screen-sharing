package publish

import (
	"fmt"
	"io"
	"strings"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// GstExe is the GStreamer pipeline launcher. It is supervised as a child process
// exactly like ffmpeg, so it reuses the same lifecycle above the seam. The encode
// probe runs its pipelines through the same launcher, which is why the name is
// exported rather than spelled a second time there.
const GstExe = "gst-launch-1.0"

// gstEngine runs one GStreamer capture backend: the backend produces raw frames,
// this graph encodes and ships them, all in one process, so this engine owns the
// whole pipeline for the backends it is instantiated with.
//
// The backend is a field rather than a branch inside the engine, so the two are
// separate axes: which framework runs a capture is a row in captureBackends, and
// a source that only one framework has an element for is a backend only that
// engine is instantiated with.
type gstEngine struct {
	capture gstCapture
}

func (g gstEngine) Command(s settings.Settings) (string, error) {
	opts, err := g.captureOptions(s)
	if err != nil {
		return "", err
	}
	// The empty meter port, the empty rate probe and the absent preview leg leave a
	// run's own branches out, the counterpart to the ffmpeg engine appending -progress
	// only for a run. Each of them carries a port the kernel handed out for one launch,
	// so a rendered command that showed them would be a different string every time it
	// was rendered - and whether two settings build one pipeline is decided by comparing
	// exactly that string (SamePipeline).
	pipeline, err := buildPipeline(s, g.capture.Describe(s, opts), "", PreviewLeg{})
	if err != nil {
		return "", err
	}
	// The runner and its subcommand lead the line because they are what runs. The rest of
	// it is the pipeline gst-launch-1.0 would take unchanged, so a reader who wants to see
	// the same graph outside the app pastes everything after the subcommand into that
	// launcher.
	exe, err := FindGstRunner()
	if err != nil {
		return "", err
	}
	return exe + " " + GstSubcommand + " " + strings.Join(pipeline, " "), nil
}

// captureOptions builds the source chain's run-independent parts and holds them
// against the machine.
//
// The device check runs here rather than at Start so that the rendered command is
// refused for the same reason a publish is. A machine whose capture and encode ends
// are not one GPU cannot run the pipeline this would display, and displaying it anyway
// would put a command in front of the user that the button beside it rejects.
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

// Carries reports whether the transport can terminate a GStreamer pipeline.
func (gstEngine) Carries(transportName string) bool {
	return transport.CanPublish(transportName, EngineGst)
}

func (g gstEngine) Start(s settings.Settings, tag string, preview PreviewLeg, cb Callbacks) (Handle, error) {
	// Validating first means a settings combination this engine cannot encode, and a
	// machine whose two ends are not one GPU, never pop the compositor's picker.
	opts, err := g.captureOptions(s)
	if err != nil {
		return nil, err
	}

	// The runner is this executable, so there is nothing to find on the machine and
	// nothing to answer for before the picker: a build that starts at all carries the
	// GStreamer it links, and which elements that installation registers is the encoder
	// probe's question rather than the launcher's.
	exe, err := FindGstRunner()
	if err != nil {
		return nil, err
	}

	// The instrumentation exists only for a caller that wants progress; without
	// OnStats the pipeline runs as the displayed command reads. The rate probe is
	// part of it, so the source the backend builds differs between a run that
	// reports progress and one that does not, exactly as the encode path does.
	// The meter's socket is open from here on, and the port it landed on is what
	// the pipeline's meter branch is pointed at. It owes nothing to what the
	// capture backend passes the child: a connection back to this process is the
	// one mechanism Windows carries, where a child inherits no descriptors at all.
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

	// The child's standard output carries two kinds of line now: the meter's progress and
	// the caps the capture negotiated. Both are read here, and the caps are what decides
	// whether this publish may continue at all.
	//
	// A handle the refusal can stop does not exist until supervise returns, so it is held
	// for the reader that starts the moment the process does. A capture whose caps arrive
	// before that reads a nil handle and stops nothing, which cannot happen in practice
	// (the caps follow the pipeline reaching PLAYING) and is a stopped publish rather than
	// a wrong one if it ever did.
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
			if handle != nil {
				handle.Stop()
			}
		})
	}

	// One socket per run, which the child opens before its pipeline plays and removes with
	// itself. A run nobody can talk to is a stream that has to be relaunched to change,
	// which is what every publish was until the pipeline moved into this app's own process.
	socket := gstControlSocket(tag)

	handle, err = supervise(superviseConfig{
		exe: exe,
		env: GstChildEnv(),
		// The subcommand leads, which is what makes this executable play a pipeline
		// instead of opening a second control socket (cmd/backend), and the control flag
		// follows it so the pipeline itself starts at the same word gst-launch would.
		args:        append(append([]string{GstSubcommand}, gstLiveArgs(socket)...), pipeline...),
		tag:         tag,
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
	return &gstHandle{Handle: handle, socket: socket}, nil
}
