package publish

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// audioRate is the sample rate the audio branch resamples to, because opusenc
// takes only 48 kHz whatever rate the monitor runs at.
const audioRate = 48000

// opusBitrate is the desktop track's bitrate in bits per second.
const opusBitrate = 128000

// buildPipeline assembles the gst-launch description: PipeWire source, a colour
// convert, the encoder for the selected codec, and the transport's muxer and
// sink. fd and node are strings so the display command can pass placeholders
// where a run passes real values. meterFd is the descriptor the progress
// instrumentation writes to, empty to build the pipeline without it.
func buildPipeline(s settings.Stream, fd, node, meterFd string) ([]string, error) {
	if err := capabilities.Validate(s.Codec, s.Chroma, s.Transport, s.Mode, s.Cq); err != nil {
		return nil, err
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
	audio, err := gstAudioBranch(s)
	if err != nil {
		return nil, err
	}
	assert.Assert(len(encoder) > 0 && len(parser) > 0, "a mapped codec yields an encoder and a parser", s.Codec)

	pipeline := gstCapture(s, fd, node, format)
	pipeline = append(pipeline, encoder...)
	pipeline = append(pipeline, "!")
	pipeline = append(pipeline, parser...)
	pipeline = append(pipeline, "!")
	if meterFd != "" {
		pipeline = append(pipeline, gstProgressElements(meterFd)...)
	}
	// With audio the muxer waits on two pads, and the queue keeps one pad's stall
	// from blocking the other branch upstream of the mux. Instrumentation needs the
	// same queue for a parsing reason: the tee it inserts and every muxer and sink
	// here expose request pads only, and gst-launch refuses to link two unnamed
	// request pads. The queue's static sink pad breaks that pair, so the link
	// resolves without pinning a tee pad number.
	if len(audio) > 0 || meterFd != "" {
		pipeline = append(pipeline, "queue", "!")
	}
	pipeline = append(pipeline, sink...)
	pipeline = append(pipeline, audio...)
	return pipeline, nil
}

// gstCapture is the part of the pipeline ahead of the encoder: the portal node,
// converted to the configured chroma and paced to the configured framerate.
//
// Portal capture is damage-driven: the compositor sends a frame only when the
// screen changes, and the PipeWire graph clock stops ticking while the captured
// node idles. Feeding the encoder straight from pipewiresrc therefore fails twice.
// On a static screen the encoder starves, so no keyframes reach the relay and
// viewers see black. And the first frame after an idle spell is stamped far ahead
// of the frozen clock, so a syncing sink waits on a clock that no longer advances,
// the SRT peer times out, and the relay drops the stream while the pipeline keeps
// running. (pipewiresrc's keepalive-time property covers only the first failure
// and still forwards the portal's timestamps, so it dies the same way on the first
// damage frame.)
//
// imagefreeze breaks both dependencies: allow-replace swaps in the newest damage
// frame, is-live repeats it at the capsfilter framerate, and the output carries
// imagefreeze's own monotonic timestamps, so the portal's clock domain never
// reaches the encoder. provide-clock=false keeps the freezing PipeWire clock from
// being elected pipeline clock; the system clock paces imagefreeze instead.
//
// The single-slot leaky queue keeps only the newest frame when a damage burst
// outruns videoconvert, which sits before imagefreeze so conversion runs once per
// damage frame, not once per output frame.
//
// The capsfilter after videoconvert pins the encoder input to the configured
// chroma, the counterpart to ffmpeg's -pix_fmt. Without it the encoder picks its
// own preferred format (x264enc lands on 4:4:4, often 10-bit), which not every
// viewer or browser decodes. The colorimetry field pins the quantization range the
// same way ffmpeg's -color_range does; only its range component is set, leaving
// matrix/transfer/primaries to negotiation.
func gstCapture(s settings.Stream, fd, node, format string) []string {
	inCaps := "video/x-raw,format=" + format + ",colorimetry=" + gstColorRange(s) + ":0:0:0"
	return []string{
		"pipewiresrc", "fd=" + fd, "path=" + node, "provide-clock=false",
		"!", "queue", "max-size-buffers=1", "leaky=downstream",
		"!", "videoconvert",
		"!", inCaps,
		"!", "imagefreeze", "is-live=true", "allow-replace=true",
		"!", "video/x-raw,framerate=" + strconv.Itoa(s.Fps) + "/1",
		"!",
	}
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
			"!", fmt.Sprintf("audio/x-raw,rate=%d,channels=2", audioRate),
			"!", "opusenc", fmt.Sprintf("bitrate=%d", opusBitrate),
			"!", "opusparse",
			"!", transport.GstMuxName + ".",
		}, nil
	default:
		return nil, fmt.Errorf("unknown audio source %q", s.Audio)
	}
}

// gstChromaFormats maps a settings chroma (the ffmpeg pixel-format name) to the
// GStreamer video/x-raw format that names the same layout. gbrp is planar RGB,
// which these encoders do not take, so videoconvert bridges it to 4:4:4 YUV.
var gstChromaFormats = map[string]string{
	"yuv420p": "I420",
	"yuv444p": "Y444",
	"gbrp":    "Y444",
	"p010le":  "I420_10LE",
}

func gstChromaFormat(chroma string) (string, error) {
	format, ok := gstChromaFormats[chroma]
	if !ok {
		return "", fmt.Errorf("chroma %q has no GStreamer format mapping", chroma)
	}
	return format, nil
}

// The range component of the encoder-input colorimetry, the GstVideoColorRange
// enum values: 0_255 is 1, 16_235 is 2.
const (
	gstRangeFull    = "1"
	gstRangeLimited = "2"
)

// gstColorRange returns the range the encoder input is pinned to. gbrp reaches the
// encoder as full-range 4:4:4 YUV after videoconvert, so it is always full; the YUV
// formats honor s.ColorRange (pc is full, tv is limited), matching the ffmpeg
// builder, which sets -color_range for every format but gbrp.
func gstColorRange(s settings.Stream) string {
	if s.Chroma == "gbrp" || s.ColorRange == "pc" {
		return gstRangeFull
	}
	return gstRangeLimited
}
