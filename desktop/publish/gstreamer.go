package publish

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"bjoernblessin.de/screenshare/portal"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// gstExe is the GStreamer pipeline launcher. It is supervised as a child process
// exactly like ffmpeg, so it reuses the same lifecycle above the seam.
const gstExe = "gst-launch-1.0"

// childFd is the descriptor number the portal PipeWire remote lands on inside
// the child: ExtraFiles[0] is inherited as fd 3.
const childFd = 3

// meterFd is where the byte meter's pipe lands in the child: ExtraFiles[1] is
// inherited as fd 4.
const meterFd = 4

// The placeholders the displayed command carries where a run passes the portal's
// real values, which exist only after the handshake.
const (
	fdPlaceholder   = "<portal-fd>"
	nodePlaceholder = "<portal-node>"
)

// gstEngine runs the xdg-desktop-portal capture path. It opens a ScreenCast
// session, then feeds the PipeWire node into a GStreamer graph that encodes and
// ships in one process, so this backend owns its whole pipeline.
type gstEngine struct{}

func (gstEngine) Command(s settings.Stream) (string, error) {
	// The empty meter fd leaves the progress instrumentation out, the counterpart
	// to the ffmpeg engine appending -progress only for a run.
	pipeline, err := buildPipeline(s, fdPlaceholder, nodePlaceholder, "")
	if err != nil {
		return "", err
	}
	return gstExe + " " + strings.Join(pipeline, " "), nil
}

func (gstEngine) Engine() string {
	return "gstreamer"
}

// Carries reports whether the transport can terminate a GStreamer pipeline.
func (gstEngine) Carries(transportName string) bool {
	return transport.CanGstPublish(transportName)
}

func (gstEngine) Start(s settings.Stream, tag string, cb Callbacks) (Handle, error) {
	session, err := portal.Open(portal.Options{})
	if err != nil {
		return nil, fmt.Errorf("portal ScreenCast: %w", err)
	}

	// The instrumentation exists only for a caller that wants progress; without
	// OnStats the pipeline runs as the displayed command reads.
	var meter *gstMeter
	meterArg := ""
	if cb.OnStats != nil {
		meter, err = newGstMeter(cb.OnStats)
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("progress meter: %w", err)
		}
		meterArg = strconv.Itoa(meterFd)
	}

	pipeline, err := buildPipeline(s, strconv.Itoa(childFd), strconv.FormatUint(uint64(session.NodeID), 10), meterArg)
	if err != nil {
		meter.close()
		session.Close()
		return nil, err
	}

	extraFiles := []*os.File{session.Fd}
	var parseStdout func(io.Reader)
	if meter != nil {
		extraFiles = append(extraFiles, meter.w)
		parseStdout = meter.parse
	}

	handle, err := supervise(superviseConfig{
		exe:         gstExe,
		args:        pipeline,
		tag:         tag,
		extraFiles:  extraFiles,
		parseStdout: parseStdout,
		onExit:      cb.OnExit,
		onCleanup: func() {
			session.Close()
			meter.close()
		},
	})
	if err != nil {
		meter.close()
		session.Close()
		return nil, err
	}
	// The child has its own copy of the write end from here on.
	meter.closeChildEnd()
	return handle, nil
}
