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

// Carries reports whether the transport can terminate a GStreamer pipeline.
func (gstEngine) Carries(transportName string) bool {
	return transport.CanGstPublish(transportName)
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
	// 10-bit), which not every viewer or browser decodes. The colorimetry field
	// pins the quantization range the same way ffmpeg's -color_range does; only
	// its range component is set, leaving matrix/transfer/primaries to
	// negotiation.
	inCaps := "video/x-raw,format=" + format + ",colorimetry=" + gstColorRange(s) + ":0:0:0"
	pipeline := []string{
		"pipewiresrc", "fd=" + fd, "path=" + node, "provide-clock=false",
		"!", "queue", "max-size-buffers=1", "leaky=downstream",
		"!", "videoconvert",
		"!", inCaps,
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

// gstColorRange returns the range component of the encoder-input colorimetry:
// "2" for full (JPEG) range, "1" for limited (MPEG). gbrp reaches the encoder as
// full-range 4:4:4 YUV after videoconvert, so it is always full; the YUV formats
// honor s.ColorRange (pc is full, tv is limited), matching the ffmpeg builder,
// which sets -color_range for every format but gbrp.
func gstColorRange(s settings.Stream) string {
	if s.Chroma == "gbrp" || s.ColorRange == "pc" {
		return "2"
	}
	return "1"
}

// gstEncoder returns the GStreamer encoder element (with its properties) and the
// parser for the selected codec. It is the GStreamer engine's half of the codec
// facts declared once in capabilities.Codecs, and the counterpart to encoderArgs
// in the ffmpeg builder: the rate-control mode selects the same behavior through
// each backend's own knobs.
// config-interval=-1 makes the parser insert SPS/PPS (H.264) or VPS/SPS/PPS
// (H.265) ahead of every IDR frame. Without it the parameter sets travel once at
// stream start, so a viewer that joins the relay mid-stream never receives them
// and its decoder cannot start. The ffmpeg publish path repeats them by default,
// which is why only this backend needed the property.
func gstEncoder(s settings.Stream, gop int) (encoder []string, parser []string, err error) {
	kbps := strconv.Itoa(s.BitrateM * 1000)
	maxkbps := strconv.Itoa(s.MaxrateM * 1000)
	cq := strconv.Itoa(s.Cq)
	g := strconv.Itoa(gop)
	switch s.Codec {
	case "libx264":
		return x264Encoder(s, kbps, cq, g), []string{"h264parse", "config-interval=-1"}, nil
	case "libx265":
		return x265Encoder(s, kbps, cq, g), []string{"h265parse", "config-interval=-1"}, nil
	case "h264_nvenc":
		return nvencEncoder("nvh264enc", s, kbps, maxkbps, cq, g), []string{"h264parse", "config-interval=-1"}, nil
	case "hevc_nvenc":
		return nvencEncoder("nvh265enc", s, kbps, maxkbps, cq, g), []string{"h265parse", "config-interval=-1"}, nil
	// The non-NVIDIA hardware families in capabilities.Codecs (vaapi, qsv, amf,
	// v4l2, rkmpp, vulkan) are declared Implemented:false and rejected before a
	// GStreamer pipeline is built, so they have no case yet. When VAAPI is wired
	// up, target the stateless "va" plugin (gst-plugins-bad): vah264enc,
	// vah265enc, vaav1enc. Avoid the older "vaapi" plugin (gstreamer-vaapi:
	// vaapih264enc, vaapih265enc). The va plugin is the maintained one, exposes
	// AV1 encoding, and negotiates the DMABuf/VAMemory caps the portal capture
	// path already produces; gstreamer-vaapi is effectively frozen and has no AV1
	// encoder. QSV uses the "qsv" plugin (qsvh264enc, qsvh265enc, qsvav1enc).
	default:
		return nil, nil, fmt.Errorf("codec %q has no GStreamer encoder mapping", s.Codec)
	}
}

// x264Encoder maps the rate-control mode onto x264enc's pass property, the
// counterpart to the libx264 branch of encoderArgs.
//   - cbr: pass=cbr targets the bitrate and bounds the VBV to it; low delay.
//   - crf: pass=qual holds a constant quantizer (the s.Cq value), bitrate free.
//   - lossless: pass=quant at quantizer 0, x264's bit-exact coding mode.
//   - abr, vbr: pass=cbr with vbv-buf-capacity=0 disables the VBV, giving
//     one-pass ABR toward the target. x264enc cannot raise the VBV maxrate above
//     the bitrate (pass=cbr locks them equal), so the vbr ceiling binds only on
//     the ffmpeg and nvenc paths; here both run as uncapped average bitrate.
//
// cbr and lossless take tune=zerolatency to hold live delay; the bitrate-bursting
// modes keep B-frames and lookahead for efficiency. The p1-p7 preset ladder is
// NVENC-only.
func x264Encoder(s settings.Stream, kbps, cq, g string) []string {
	switch s.Mode {
	case "crf":
		return []string{"x264enc", "pass=qual", "quantizer=" + cq, "speed-preset=slow", "key-int-max=" + g}
	case "lossless":
		return []string{"x264enc", "pass=quant", "quantizer=0", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + g}
	case "abr", "vbr":
		return []string{"x264enc", "bitrate=" + kbps, "pass=cbr", "vbv-buf-capacity=0", "speed-preset=medium", "key-int-max=" + g}
	default: // cbr
		enc := []string{"x264enc", "bitrate=" + kbps, "pass=cbr", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + g}
		if s.VbvMs > 0 {
			enc = append(enc, "vbv-buf-capacity="+strconv.Itoa(s.VbvMs))
		}
		return enc
	}
}

// x265Encoder maps the rate-control mode onto x265enc, the HEVC counterpart to
// x264Encoder. x265enc has no pass property: rate control comes from the bitrate
// and qp properties plus an option-string of libx265 knobs.
//   - crf: qp holds a constant quantizer (s.Cq), x265's CQP mode, matching
//     x264enc's quantizer property.
//   - lossless: option-string lossless=1. Unlike x264, qp 0 is not bit-exact on
//     x265, so the dedicated flag is required; zerolatency drops B-frames.
//   - abr, vbr: bitrate alone is one-pass average bitrate. As on x264enc the vbr
//     ceiling does not bind here, only on the ffmpeg and nvenc paths.
//   - cbr: bitrate plus a vbv-maxrate=bitrate ceiling and a vbv-bufsize window,
//     x265's constrained constant bitrate; zerolatency for low delay.
func x265Encoder(s settings.Stream, kbps, cq, g string) []string {
	switch s.Mode {
	case "crf":
		return []string{"x265enc", "qp=" + cq, "speed-preset=slow", "key-int-max=" + g}
	case "lossless":
		return []string{"x265enc", "option-string=lossless=1", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + g}
	case "abr", "vbr":
		return []string{"x265enc", "bitrate=" + kbps, "speed-preset=medium", "key-int-max=" + g}
	default: // cbr
		// vbv-bufsize is in kbit: the bitrate held over the VBV window, one
		// second when unset, matching ffmpeg's bufsizeArg.
		bufKbit := kbps
		if s.VbvMs > 0 {
			bufKbit = strconv.Itoa(s.BitrateM * s.VbvMs)
		}
		opts := "vbv-maxrate=" + kbps + ":vbv-bufsize=" + bufKbit
		return []string{"x265enc", "bitrate=" + kbps, "option-string=" + opts, "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + g}
	}
}

// nvencEncoder maps the rate-control mode onto the nvcodec element's properties,
// the counterpart to the NVENC branch of encoderArgs. The knobs differ from
// ffmpeg's: nvh264enc/nvh265enc expose rc-mode plus a constant-QP target rather
// than the SDK preset ladders and tunes.
//   - cbr: rc-mode=cbr with zero-latency reordering.
//   - vbr: rc-mode=vbr targeting the bitrate with max-bitrate as the ceiling.
//   - abr: rc-mode=vbr toward the bitrate with no ceiling.
//   - crf: rc-mode=constqp at s.Cq.
//   - lossless: the element's lossless preset, rate control dropped.
//
// B-frames apply only to the lossy bursting modes. The p1-p7 preset ladder in
// s.EncPreset has no equivalent on these elements and is not forwarded.
func nvencEncoder(elem string, s settings.Stream, kbps, maxkbps, cq, g string) []string {
	bf := strconv.Itoa(s.Bframes)
	withBframes := func(enc []string) []string {
		if s.Bframes > 0 {
			return append(enc, "bframes="+bf)
		}
		return enc
	}
	switch s.Mode {
	case "lossless":
		return withBframes([]string{elem, "preset=lossless", "gop-size=" + g})
	case "crf":
		return withBframes([]string{elem, "rc-mode=constqp", "qp-const=" + cq, "gop-size=" + g})
	case "abr":
		return withBframes([]string{elem, "rc-mode=vbr", "bitrate=" + kbps, "gop-size=" + g})
	case "vbr":
		return withBframes([]string{elem, "rc-mode=vbr", "bitrate=" + kbps, "max-bitrate=" + maxkbps, "gop-size=" + g})
	default: // cbr
		return []string{elem, "rc-mode=cbr", "bitrate=" + kbps, "zerolatency=true", "gop-size=" + g}
	}
}
