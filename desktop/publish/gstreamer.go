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

	if s.Fps <= 0 {
		return nil, fmt.Errorf("the portal (GStreamer) backend needs a positive fps, got %d", s.Fps)
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

	// Portal capture is damage-driven: the compositor sends a frame only when
	// the screen changes, and the PipeWire graph clock stops ticking while the
	// captured node idles. Feeding the encoder straight from pipewiresrc
	// therefore fails twice. On a static screen the encoder starves, so no
	// keyframes reach the relay and viewers see black. And the first frame
	// after an idle spell is stamped far ahead of the frozen clock, so a
	// syncing sink waits on a clock that no longer advances, the SRT peer
	// times out, and the relay drops the stream while the pipeline keeps
	// running. (pipewiresrc's keepalive-time property covers only the first
	// failure and still forwards the portal's timestamps, so it dies the same
	// way on the first damage frame.)
	//
	// imagefreeze breaks both dependencies: allow-replace swaps in the newest
	// damage frame, is-live repeats it at the capsfilter framerate, and the
	// output carries imagefreeze's own monotonic timestamps, so the portal's
	// clock domain never reaches the encoder. provide-clock=false keeps the
	// freezing PipeWire clock from being elected pipeline clock; the system
	// clock paces imagefreeze instead.
	//
	// The single-slot leaky queue keeps only the newest frame when a damage
	// burst outruns videoconvert, which sits before imagefreeze so conversion
	// runs once per damage frame, not once per output frame.
	//
	// The capsfilter after videoconvert pins the encoder input to the
	// configured chroma, the counterpart to ffmpeg's -pix_fmt. Without it the
	// encoder picks its own preferred format (x264enc lands on 4:4:4, often
	// 10-bit), which not every viewer or browser decodes.
	pipeline := []string{
		"pipewiresrc", "fd=" + fd, "path=" + node, "provide-clock=false",
		"!", "queue", "max-size-buffers=1", "leaky=downstream",
		"!", "videoconvert",
		"!", "video/x-raw,format=" + format,
		"!", "imagefreeze", "is-live=true", "allow-replace=true",
		"!", "video/x-raw,framerate=" + strconv.Itoa(s.Fps) + "/1",
		"!",
	}
	audio, err := gstAudioBranch(s)
	if err != nil {
		return nil, err
	}

	pipeline = append(pipeline, encoder...)
	pipeline = append(pipeline, "!")
	pipeline = append(pipeline, parser...)
	pipeline = append(pipeline, "!")
	if len(audio) > 0 {
		// With audio the muxer waits on two pads; the queue keeps one pad's
		// stall from blocking the other branch upstream of the mux.
		pipeline = append(pipeline, "queue", "!")
	}
	pipeline = append(pipeline, sink...)
	pipeline = append(pipeline, audio...)
	return pipeline, nil
}

// gstAudioBranch returns the elements that capture desktop audio and attach it
// to the muxer as a second track, or nil when audio is off.
//
// pulsesrc records @DEFAULT_MONITOR@, the libpulse magic name for the monitor
// of the default sink: the mixed desktop audio. PipeWire's pulse server
// implements the same name. An attached record stream keeps the monitor source
// running, so silence flows even while nothing plays and the muxer's audio pad
// never starves. Opus because mpegtsmux carries it and MediaMTX, ffplay, mpv
// and browsers all decode it; opusparse puts the caps mpegtsmux expects on the
// stream. The capsfilter sits after audioresample because opusenc takes only
// 48 kHz, whatever rate the monitor runs at.
func gstAudioBranch(s settings.Stream) ([]string, error) {
	switch s.Audio {
	case "", "none":
		return nil, nil
	case "desktop":
		return []string{
			"pulsesrc", "device=@DEFAULT_MONITOR@",
			"!", "queue",
			"!", "audioconvert",
			"!", "audioresample",
			"!", "audio/x-raw,rate=48000,channels=2",
			"!", "opusenc", "bitrate=128000",
			"!", "opusparse",
			"!", transport.GstMuxName + ".",
		}, nil
	default:
		return nil, fmt.Errorf("unknown audio source %q", s.Audio)
	}
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
