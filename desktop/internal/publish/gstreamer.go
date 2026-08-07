package publish

import (
	"fmt"
	"io"
	"strconv"
	"strings"

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

func (g gstEngine) Command(s settings.Stream) (string, error) {
	opts, err := g.captureOptions(s)
	if err != nil {
		return "", err
	}
	// The empty meter fd and the empty rate probe leave the progress
	// instrumentation out, the counterpart to the ffmpeg engine appending
	// -progress only for a run.
	pipeline, err := buildPipeline(s, g.capture.Describe(s, opts), "")
	if err != nil {
		return "", err
	}
	return GstExe + " " + strings.Join(pipeline, " "), nil
}

// captureOptions builds the source chain's run-independent parts and holds them
// against the machine.
//
// The device check runs here rather than at Start so that the rendered command is
// refused for the same reason a publish is. A machine whose capture and encode ends
// are not one GPU cannot run the pipeline this would display, and displaying it anyway
// would put a command in front of the user that the button beside it rejects.
func (g gstEngine) captureOptions(s settings.Stream) (gstCaptureOptions, error) {
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

func (g gstEngine) Start(s settings.Stream, tag string, cb Callbacks) (Handle, error) {
	// Validating first means a settings combination this engine cannot encode, and a
	// machine whose two ends are not one GPU, never pop the compositor's picker.
	opts, err := g.captureOptions(s)
	if err != nil {
		return nil, err
	}

	// A missing launcher belongs to the same set: it is known before anything is
	// opened, so it is answered before the picker rather than after the user has
	// chosen a surface for a pipeline nothing can run.
	exe, err := FindGstExe()
	if err != nil {
		return nil, err
	}

	// The instrumentation exists only for a caller that wants progress; without
	// OnStats the pipeline runs as the displayed command reads. The rate probe is
	// part of it, so the source the backend builds differs between a run that
	// reports progress and one that does not, exactly as the encode path does.
	var meter *gstMeter
	if cb.OnStats != nil {
		meter, err = newGstMeter(cb.OnStats)
		if err != nil {
			return nil, fmt.Errorf("progress meter: %w", err)
		}
		opts.RateProbe = gstCaptureProbe
	}

	source, files, closeSource, err := g.capture.Open(s, opts)
	if err != nil {
		meter.close()
		return nil, err
	}

	// The meter's pipe follows the backend's own files, so the descriptor it lands
	// on depends on how many of those there are.
	meterArg := ""
	if meter != nil {
		meterArg = strconv.Itoa(childFdBase + len(files))
	}

	pipeline, err := buildPipeline(s, source, meterArg)
	if err != nil {
		meter.close()
		closeSource()
		return nil, err
	}

	var parseStdout func(io.Reader)
	if meter != nil {
		files = append(files, meter.w)
		parseStdout = meter.parse
	}

	handle, err := supervise(superviseConfig{
		exe:         exe,
		env:         GstChildEnv(),
		args:        pipeline,
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
	// The child has its own copy of the write end from here on.
	meter.closeChildEnd()
	return handle, nil
}
