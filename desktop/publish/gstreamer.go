package publish

import (
	"fmt"
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

// gstEngine runs the xdg-desktop-portal capture path. It opens a ScreenCast
// session, then feeds the PipeWire node into a GStreamer graph that encodes and
// ships in one process, so this backend owns its whole pipeline.
type gstEngine struct{}

func (gstEngine) Command(s settings.Stream) (string, error) {
	url, ok := transport.PublishURL(s)
	if !ok {
		return "", fmt.Errorf("transport %q has no URL form for the GStreamer engine", s.Transport)
	}
	// The real fd and node id exist only after the portal handshake; the display
	// command shows their placeholders so the pipeline stays readable.
	pipeline, err := buildPipeline(s, url, "<portal-fd>", "<portal-node>")
	if err != nil {
		return "", err
	}
	return gstExe + " " + strings.Join(pipeline, " "), nil
}

func (gstEngine) Start(s settings.Stream, tag string, cb Callbacks) (Handle, error) {
	url, ok := transport.PublishURL(s)
	if !ok {
		return nil, fmt.Errorf("transport %q has no URL form for the GStreamer engine", s.Transport)
	}

	session, err := portal.Open(portal.Options{})
	if err != nil {
		return nil, fmt.Errorf("portal ScreenCast: %w", err)
	}

	pipeline, err := buildPipeline(s, url, strconv.Itoa(childFd), strconv.FormatUint(uint64(session.NodeID), 10))
	if err != nil {
		session.Close()
		return nil, err
	}

	handle, err := supervise(superviseConfig{
		exe:        gstExe,
		args:       pipeline,
		tag:        tag,
		extraFiles: []*os.File{session.Fd},
		onExit:     cb.OnExit,
		onCleanup:  session.Close,
	})
	if err != nil {
		session.Close()
		return nil, err
	}
	return handle, nil
}

// buildPipeline assembles the gst-launch description: PipeWire source, a colour
// convert, the encoder for the selected codec, an MPEG-TS mux aligned to the SRT
// packet size, and the SRT sink. fd and node are strings so the display command
// can pass placeholders where a run passes real values.
func buildPipeline(s settings.Stream, url, fd, node string) ([]string, error) {
	// The pipeline targets a bitrate, so it implements only the latency mode.
	// The UI gates this (see deps.ts), and the check keeps a preset or a
	// hand-edited settings file from reaching an unimplemented rate control.
	if s.Mode != "latency" {
		return nil, fmt.Errorf("the portal (GStreamer) backend implements only latency rate control, not %q", s.Mode)
	}

	gop := s.Gop
	if gop <= 0 {
		gop = s.Fps * 2
	}
	encoder, parser, err := gstEncoder(s, gop)
	if err != nil {
		return nil, err
	}

	pipeline := []string{
		"pipewiresrc", "fd=" + fd, "path=" + node, "do-timestamp=true",
		"!", "videoconvert",
		"!",
	}
	pipeline = append(pipeline, encoder...)
	// alignment=7 packs 7 * 188-byte TS packets into each buffer, matching the
	// SRT pkt_size of 1316 the transport sets.
	pipeline = append(pipeline,
		"!", parser,
		"!", "mpegtsmux", "alignment=7",
		"!", "srtsink", "uri="+url, "wait-for-connection=false",
	)
	return pipeline, nil
}

// gstEncoder returns the GStreamer encoder element (with its properties) and the
// parser for the selected codec. It is the GStreamer engine's half of the codec
// facts declared once in capabilities.Codecs. The mapping targets a bitrate; the
// lossless and quality rate-control modes are not expressed through it.
func gstEncoder(s settings.Stream, gop int) (encoder []string, parser string, err error) {
	kbps := strconv.Itoa(s.BitrateM * 1000)
	g := strconv.Itoa(gop)
	switch s.Codec {
	case "libx264":
		return []string{"x264enc", "bitrate=" + kbps, "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + g}, "h264parse", nil
	case "h264_nvenc":
		return []string{"nvh264enc", "bitrate=" + kbps, "gop-size=" + g}, "h264parse", nil
	case "hevc_nvenc":
		return []string{"nvh265enc", "bitrate=" + kbps, "gop-size=" + g}, "h265parse", nil
	default:
		return nil, "", fmt.Errorf("codec %q has no GStreamer encoder mapping", s.Codec)
	}
}
