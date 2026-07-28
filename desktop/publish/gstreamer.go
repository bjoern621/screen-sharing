package publish

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// gstExe is the GStreamer pipeline launcher. It is supervised as a child process
// exactly like ffmpeg, so it reuses the same lifecycle above the seam.
const gstExe = "gst-launch-1.0"

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
	inCaps, err := gstInputCaps(s)
	if err != nil {
		return "", err
	}
	// The empty meter fd and the empty rate probe leave the progress
	// instrumentation out, the counterpart to the ffmpeg engine appending
	// -progress only for a run.
	pipeline, err := buildPipeline(s, g.capture.Describe(s, gstCaptureOptions{InCaps: inCaps}), "")
	if err != nil {
		return "", err
	}
	return gstExe + " " + strings.Join(pipeline, " "), nil
}

func (gstEngine) Engine() string {
	return EngineGst
}

// Carries reports whether the transport can terminate a GStreamer pipeline.
func (gstEngine) Carries(transportName string) bool {
	return transport.CanGstPublish(transportName)
}

func (g gstEngine) Start(s settings.Stream, tag string, cb Callbacks) (Handle, error) {
	// Validating first means a settings combination this engine cannot encode
	// never pops the compositor's picker.
	inCaps, err := gstInputCaps(s)
	if err != nil {
		return nil, err
	}

	// The instrumentation exists only for a caller that wants progress; without
	// OnStats the pipeline runs as the displayed command reads. The rate probe is
	// part of it, so the source the backend builds differs between a run that
	// reports progress and one that does not, exactly as the encode path does.
	opts := gstCaptureOptions{InCaps: inCaps}
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
		exe:         gstExe,
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
