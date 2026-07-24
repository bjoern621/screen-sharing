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
	// The real fd and node id exist only after the portal handshake; the display
	// command shows their placeholders so the pipeline stays readable.
	pipeline, err := buildPipeline(s, "<portal-fd>", "<portal-node>")
	if err != nil {
		return "", err
	}
	return gstExe + " " + strings.Join(pipeline, " "), nil
}

func (gstEngine) Start(s settings.Stream, tag string, cb Callbacks) (Handle, error) {
	session, err := portal.Open(portal.Options{})
	if err != nil {
		return nil, fmt.Errorf("portal ScreenCast: %w", err)
	}

	pipeline, err := buildPipeline(s, strconv.Itoa(childFd), strconv.FormatUint(uint64(session.NodeID), 10))
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
// convert, the encoder for the selected codec, and the transport's muxer and
// sink. fd and node are strings so the display command can pass placeholders
// where a run passes real values.
func buildPipeline(s settings.Stream, fd, node string) ([]string, error) {
	// The pipeline targets a bitrate, so it implements only the latency mode.
	// The UI gates this (see deps.ts), and the check keeps a preset or a
	// hand-edited settings file from reaching an unimplemented rate control.
	if s.Mode != "latency" {
		return nil, fmt.Errorf("the portal (GStreamer) backend implements only latency rate control, not %q", s.Mode)
	}

	sink, ok := transport.GstSink(s)
	if !ok {
		return nil, fmt.Errorf("transport %q has no GStreamer sink", s.Transport)
	}

	gop := s.Gop
	if gop <= 0 {
		gop = s.Fps * 2
	}
	format, err := gstChromaFormat(s.Chroma)
	if err != nil {
		return nil, err
	}
	encoder, parser, err := gstEncoder(s, gop)
	if err != nil {
		return nil, err
	}

	// The capsfilter after videoconvert pins the encoder input to the configured
	// chroma, the counterpart to ffmpeg's -pix_fmt. Without it the encoder picks
	// its own preferred format (x264enc lands on 4:4:4, often 10-bit), which the
	// display-paced ffplay viewer cannot build a render filtergraph for, so the
	// viewer window never appears.
	pipeline := []string{
		"pipewiresrc", "fd=" + fd, "path=" + node, "do-timestamp=true",
		"!", "videoconvert",
		"!", "video/x-raw,format=" + format,
		"!",
	}
	pipeline = append(pipeline, encoder...)
	pipeline = append(pipeline, "!")
	pipeline = append(pipeline, parser...)
	pipeline = append(pipeline, "!")
	pipeline = append(pipeline, sink...)
	return pipeline, nil
}

// gstChromaFormat maps a settings chroma (the ffmpeg pixel-format name) to the
// GStreamer video/x-raw format that names the same layout. gbrp is planar RGB,
// which these encoders do not take, so videoconvert bridges it to 4:4:4 YUV.
func gstChromaFormat(chroma string) (string, error) {
	switch chroma {
	case "yuv420p":
		return "I420", nil
	case "yuv444p", "gbrp":
		return "Y444", nil
	case "p010le":
		return "I420_10LE", nil
	default:
		return "", fmt.Errorf("chroma %q has no GStreamer format mapping", chroma)
	}
}

// gstEncoder returns the GStreamer encoder element (with its properties) and the
// parser for the selected codec. It is the GStreamer engine's half of the codec
// facts declared once in capabilities.Codecs. The mapping targets a bitrate; the
// lossless and quality rate-control modes are not expressed through it.
// config-interval=-1 makes the parser insert SPS/PPS (H.264) or VPS/SPS/PPS
// (H.265) ahead of every IDR frame. Without it the parameter sets travel once at
// stream start, so a viewer that joins the relay mid-stream never receives them
// and its decoder cannot start. The ffmpeg publish path repeats them by default,
// which is why only this backend needed the property.
func gstEncoder(s settings.Stream, gop int) (encoder []string, parser []string, err error) {
	kbps := strconv.Itoa(s.BitrateM * 1000)
	g := strconv.Itoa(gop)
	switch s.Codec {
	case "libx264":
		return []string{"x264enc", "bitrate=" + kbps, "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + g}, []string{"h264parse", "config-interval=-1"}, nil
	case "h264_nvenc":
		return []string{"nvh264enc", "bitrate=" + kbps, "gop-size=" + g}, []string{"h264parse", "config-interval=-1"}, nil
	case "hevc_nvenc":
		return []string{"nvh265enc", "bitrate=" + kbps, "gop-size=" + g}, []string{"h265parse", "config-interval=-1"}, nil
	default:
		return nil, nil, fmt.Errorf("codec %q has no GStreamer encoder mapping", s.Codec)
	}
}
