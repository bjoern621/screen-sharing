package publish

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/gpupath"
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

// audioRate is the sample rate the audio branch resamples to, because opusenc
// takes only 48 kHz whatever rate the monitor runs at.
const audioRate = 48000

// opusBitrate is the desktop track's bitrate in bits per second.
const opusBitrate = 128000

// buildPipeline assembles the gst-launch description: the capture backend's
// source elements, the encoder for the selected codec, and the transport's muxer
// and sink. capture is the already-built source, so a run and the displayed
// command differ only in what the backend put in it. meterFd is the descriptor
// the progress instrumentation writes to, empty to build the pipeline without it.
func buildPipeline(s settings.Stream, capture []string, meterFd string) ([]string, error) {
	if err := capabilities.Validate(EngineGst, s.Codec, s.CapabilityOptions(), s.Cq, s.BitrateM); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublish(s.Transport, s.Codec); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublishSettings(s); err != nil {
		return nil, err
	}

	sink, ok := transport.GstSink(s)
	if !ok {
		return nil, fmt.Errorf("transport %q has no GStreamer publish sink", s.Transport)
	}

	if s.Fps <= 0 {
		return nil, fmt.Errorf("the GStreamer publish engine needs a positive fps, got %d", s.Fps)
	}
	gop := s.Gop
	if gop <= 0 {
		gop = s.Fps * 2
	}
	encoder, link, err := gstEncoder(s, gop)
	if err != nil {
		return nil, err
	}
	audio, err := gstAudioBranch(s)
	if err != nil {
		return nil, err
	}
	assert.Assert(len(encoder) > 0, "a mapped codec yields an encoder", s.Codec)
	assert.Assert(len(capture) > 0, "a capture backend yields source elements", s.Capture)

	pipeline := append(append([]string{}, capture...), "!")
	pipeline = append(pipeline, encoder...)
	pipeline = append(pipeline, "!")
	// Most codecs put a parser or a capsfilter between encoder and sink; a codec
	// whose element leaves nothing for one to do links straight to the sink.
	if len(link) > 0 {
		pipeline = append(pipeline, link...)
		pipeline = append(pipeline, "!")
	}
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

// gstSystemConvert is the element that converts captured frames on the CPU. It is
// the conversion every pair without a GPU path runs, whichever end lacks one.
const gstSystemConvert = "videoconvert"

// gstGpuMemory is how one encoder family's elements read frames on the GPU path: the
// caps feature its surfaces carry, and the element that converts captured frames into
// them without leaving the device.
type gstGpuMemory struct {
	feature string
	convert string
}

// gstGpuMemories is the GPU path per encoder family, keyed as capabilities.Codecs
// names the family, and the engine's half of the pairs gpupath.Paths declares. A
// family named in a row here and not there has no pipeline reaching it; one named
// there and not here is a table half declared, which the builder asserts.
//
// Only the va family has an entry. Its elements negotiate VASurface caps and take
// DMABuf on their sink pads, and vapostproc is the VA post-processor: it imports the
// portal's dmabuf, converts to the encoder's semi-planar layout and applies the
// quantization range, all on the device. The nvcodec elements read system memory
// unless the whole graph runs on CUDA memory, which no capture backend here produces,
// and the qsv ones take VA surfaces only through an interop the plugin does not
// negotiate from a foreign dmabuf.
var gstGpuMemories = map[string]gstGpuMemory{
	capabilities.FamilyVaapi: {feature: "(memory:VAMemory)", convert: "vapostproc"},
}

// gstFrameMemory is where a run's frames reach the encoder, in the vocabulary the
// pipeline states it in: the caps feature the encoder input carries, and the element
// that converts into it.
type gstFrameMemory struct {
	// memory is the resolved gpupath value, MemoryGpu or MemorySystem.
	memory string
	// feature is the caps feature both the encoder input and everything pinned
	// downstream of it carry, empty for system memory.
	feature string
	convert string
}

// gstMemory resolves the frame memory for these settings against the pair table, and
// refuses a demand this engine cannot meet.
//
// The device check is the caller's, not this function's: it reads the machine, and
// this answers from the tables alone so that a settings combination is refused for
// what it names rather than for the hardware it happens to run on.
func gstMemory(s settings.Stream) (gstFrameMemory, error) {
	c, ok := capabilities.Get(s.Codec)
	if !ok {
		return gstFrameMemory{}, fmt.Errorf("unknown codec %q", s.Codec)
	}
	memory, err := gpupath.Resolve(EngineGst, s.Capture, c.Family, s.CaptureMemory)
	if err != nil {
		return gstFrameMemory{}, err
	}
	if memory == gpupath.MemorySystem {
		return gstFrameMemory{memory: memory, convert: gstSystemConvert}, nil
	}
	gpu, ok := gstGpuMemories[c.Family]
	assert.Assert(ok, "a family with a GPU path states the memory its surfaces carry", c.Family)
	return gstFrameMemory{memory: memory, feature: gpu.feature, convert: gpu.convert}, nil
}

// gstSourceOptions builds the parts of the source chain that follow from the tables
// alone: where the frames reach the encoder, the caps stating it, and the element
// converting into them. What a run adds on top is its instrumentation, and what the
// engine adds is the check that the machine can hold both ends on one device.
func gstSourceOptions(s settings.Stream) (gstCaptureOptions, error) {
	mem, err := gstMemory(s)
	if err != nil {
		return gstCaptureOptions{}, err
	}
	inCaps, err := gstInputCaps(s, mem)
	if err != nil {
		return gstCaptureOptions{}, err
	}
	return gstCaptureOptions{
		Memory:  mem.memory,
		InCaps:  inCaps,
		Feature: mem.feature,
		Convert: mem.convert,
	}, nil
}

// gstInputCaps returns the capsfilter each capture backend ends in, and rejects a
// settings combination this engine cannot encode. The engine calls it before it
// acquires anything, so a combination the table forbids fails without opening a
// portal session or an X display.
//
// The capsfilter pins the encoder input to the configured chroma, the counterpart
// to ffmpeg's -pix_fmt. Without it the encoder picks its own preferred format
// (x264enc lands on 4:4:4, often 10-bit), which not every viewer or browser
// decodes. The colorimetry field pins the quantization range the same way
// ffmpeg's -color_range does, and the colour space along with it.
//
// The memory feature leads the caps because it decides which pads can link at all.
// Plain video/x-raw means system memory and nothing else, so a capsfilter that omits
// the feature on the GPU path pins the frames back into the round trip the path
// exists to avoid, and the negotiation fails against a source that only offers
// device memory.
func gstInputCaps(s settings.Stream, mem gstFrameMemory) (string, error) {
	if err := capabilities.Validate(EngineGst, s.Codec, s.CapabilityOptions(), s.Cq, s.BitrateM); err != nil {
		return "", err
	}
	if err := transport.ValidatePublish(s.Transport, s.Codec); err != nil {
		return "", err
	}
	if err := transport.ValidatePublishSettings(s); err != nil {
		return "", err
	}
	if s.Fps <= 0 {
		return "", fmt.Errorf("the GStreamer publish engine needs a positive fps, got %d", s.Fps)
	}
	format, err := gstChromaFormat(s.Codec, s.Chroma)
	if err != nil {
		return "", err
	}
	colorimetry, err := gstColorimetry(s)
	if err != nil {
		return "", err
	}
	return "video/x-raw" + mem.feature + ",format=" + format + ",colorimetry=" + colorimetry, nil
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
// GStreamer video/x-raw format carrying the same subsampling and bit depth. The
// 10-bit entry is the planar layout, since these elements take no semi-planar input;
// what the row promises is 10-bit 4:2:0, not p010le's byte order.
//
// gbrp is absent because no encoder element here takes planar RGB: it is declared as
// a per-engine chroma gap (gstNoPlanarRGB) and refused by capabilities.Validate, so
// the pipeline never has to decide whether to convert RGB to YUV behind the user's
// back.
var gstChromaFormats = map[string]string{
	"yuv420p": "I420",
	"yuv444p": "Y444",
	"p010le":  "I420_10LE",
}

// gstVaChromaFormats is the same mapping for the va plugin's encoders, which take
// the semi-planar layouts the VAAPI drivers store surfaces in and negotiate no
// planar format at all. It is the GStreamer counterpart of vaapiFormats in the
// ffmpeg builder, and the reason capabilities.Codecs declares no other chroma for
// the family.
var gstVaChromaFormats = map[string]string{
	"yuv420p": "NV12",
	"p010le":  "P010_10LE",
}

// gstFamilyChromaFormats is the raw-format mapping per encoder family, keyed as
// capabilities.Codecs names the family. Every family with a row in gstCodecs carries
// an entry, so which layout an element negotiates is stated rather than assumed: a
// family added there without one is refused, where taking the planar layouts by
// default would pin caps its elements do not negotiate and the pipeline would fail
// in negotiation instead of naming the family.
//
// The QSV entry is the semi-planar one because the qsv plugin drives oneVPL over VA
// on Linux and D3D11 on Windows, and both store surfaces in the layouts the VAAPI
// drivers do.
var gstFamilyChromaFormats = map[string]map[string]string{
	capabilities.FamilySoftware: gstChromaFormats,
	capabilities.FamilyNvenc:    gstChromaFormats,
	capabilities.FamilyVaapi:    gstVaChromaFormats,
	capabilities.FamilyQsv:      gstVaChromaFormats,
}

// gstChromaFormat returns the raw format the capture chain pins ahead of the codec's
// encoder element.
func gstChromaFormat(codec, chroma string) (string, error) {
	c, ok := capabilities.Get(codec)
	if !ok {
		return "", fmt.Errorf("unknown codec %q", codec)
	}
	formats, ok := gstFamilyChromaFormats[c.Family]
	if !ok {
		return "", fmt.Errorf("the %s encoder family has no GStreamer raw-format mapping", c.Family)
	}
	format, ok := formats[chroma]
	if !ok {
		return "", fmt.Errorf("chroma %q has no GStreamer format mapping for codec %s", chroma, codec)
	}
	return format, nil
}

// The range component of the encoder-input colorimetry, the GstVideoColorRange
// enum values: 0_255 is 1, 16_235 is 2.
const (
	gstRangeFull    = "1"
	gstRangeLimited = "2"
)

// gstBt709 is the matrix, transfer function and primaries the publish pipeline
// encodes against, as the GstVideoColorMatrix, GstVideoTransferFunction and
// GstVideoColorPrimaries enum values the colorimetry field spells after the
// range. BT.709 is the colour space of every HD and larger picture, which is
// every screen this app captures, and the encoders write it into the bitstream,
// so a viewer converts back with the matrix the frames were made with instead of
// picking one from the picture size.
const gstBt709 = "3:5:1"

// gstColorimetry returns the complete colorimetry the encoder input is pinned to,
// and rejects a colour range this engine has no mapping for.
//
// All four components are named because a partial one is not partially applied.
// Left as "<range>:0:0:0", videoconvert drops the range along with the three
// unknown components and converts to limited range whatever the range says, so
// the colour-range setting reaches the caps and changes nothing about the frames:
// full-range white leaves the capture chain as Y=235 exactly like limited-range
// white. Spelled out, the range takes effect (Y=254) and the stream signals what
// it holds.
func gstColorimetry(s settings.Stream) (string, error) {
	r, err := gstColorRange(s)
	if err != nil {
		return "", err
	}
	return r + ":" + gstBt709, nil
}

// gstColorRanges maps a settings colour range to its GstVideoColorRange value.
// Every chroma this engine encodes is a YUV one, so the range always applies;
// gbrp, the format that is full range by construction, does not reach this engine
// (gstNoPlanarRGB).
var gstColorRanges = map[string]string{
	"pc": gstRangeFull,
	"tv": gstRangeLimited,
}

// gstColorRange returns the range the encoder input is pinned to, matching the
// ffmpeg builder's -color_range.
//
// A value with no mapping is refused rather than read as limited. The range is
// carried in the bitstream and decides how every viewer expands the picture, so
// substituting one would change what the stream looks like without saying so, and
// the ffmpeg engine passes the same field straight to -color_range, which fails
// loudly on a value it does not know.
func gstColorRange(s settings.Stream) (string, error) {
	r, ok := gstColorRanges[s.ColorRange]
	if !ok {
		return "", fmt.Errorf("colour range %q has no GStreamer mapping, expected pc or tv", s.ColorRange)
	}
	return r, nil
}
