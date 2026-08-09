package publish

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// buildPipeline assembles the gst-launch description: the capture backend's
// source elements, the encoder for the selected codec, and the transport's muxer
// and sink. capture is the already-built source, so a run and the displayed
// command differ only in what the backend put in it. meterPort is the loopback
// port the progress instrumentation writes to, empty to build the pipeline
// without it, and preview the loopback port the local preview's copy goes to,
// zero for the same reason.
func buildPipeline(s settings.Settings, capture []string, meterPort string, preview PreviewLeg) ([]string, error) {
	if err := capabilities.Validate(EngineGst, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublish(s.Publish.Transport, EngineGst, s.Publish.Codec); err != nil {
		return nil, err
	}
	if err := capabilities.ValidateAudio(EngineGst, s.Publish.AudioTrack()); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublishAudio(s.Publish.Transport, EngineGst, s.Publish.AudioTrack()); err != nil {
		return nil, err
	}
	if err := transport.ValidatePublishSettings(s); err != nil {
		return nil, err
	}

	sink, ok := transport.GstSink(s)
	if !ok {
		return nil, fmt.Errorf("transport %q has no GStreamer publish sink", s.Publish.Transport)
	}

	if s.Publish.Fps <= 0 {
		return nil, fmt.Errorf("the GStreamer publish engine needs a positive fps, got %d", s.Publish.Fps)
	}
	// The frame memory is resolved here as well as where the source was built, because
	// the encoder element is one of the things it decides: a family whose plugin ships
	// one element per memory kind is encoded by a different one on each path. Both
	// resolutions read the same two tables from the same settings, so the pipeline
	// cannot be assembled from a source and an encoder that disagree about where the
	// frames are.
	mem, err := gstMemory(s)
	if err != nil {
		return nil, err
	}
	encoder, link, err := gstEncoder(s, gstGop(s), mem.memory)
	if err != nil {
		return nil, err
	}
	audio, err := gstAudioBranch(s)
	if err != nil {
		return nil, err
	}
	assert.Assert(len(encoder) > 0, "a mapped codec yields an encoder", s.Publish.Codec)
	assert.Assert(len(capture) > 0, "a capture backend yields source elements", s.Publish.Capture)

	pipeline := append(append([]string{}, capture...), "!")
	pipeline = append(pipeline, encoder...)
	pipeline = append(pipeline, "!")
	// Most codecs put a parser or a capsfilter between encoder and sink; a codec
	// whose element leaves nothing for one to do links straight to the sink.
	if len(link) > 0 {
		pipeline = append(pipeline, link...)
		pipeline = append(pipeline, "!")
	}
	// The counter goes ahead of the tee, so what it counts is the stream rather than
	// one branch of it.
	if meterPort != "" {
		pipeline = append(pipeline, gstProgressElement...)
		pipeline = append(pipeline, "!")
	}
	// Both taps copy the stream the encoder already produced, which is the whole point
	// of teeing it: neither the meter nor the preview costs a second encode, and the
	// preview leaves this machine over no network at all.
	var taps [][]string
	if meterPort != "" {
		taps = append(taps, gstMeterTap(meterPort))
	}
	if preview.Wanted() {
		// A format with no local carriage publishes without a preview rather than
		// failing to publish, which is the same answer the ffmpeg engine gives.
		if tap, err := gstPreviewTap(s.Publish.Codec, preview); err == nil {
			taps = append(taps, tap)
		}
	}
	pipeline = append(pipeline, gstTapElements(taps)...)
	// With audio the muxer waits on two pads, and the queue keeps one pad's stall
	// from blocking the other branch upstream of the mux. A tap needs the same queue
	// for a parsing reason: the tee it inserts and every muxer and sink here expose
	// request pads only, and gst-launch refuses to link two unnamed request pads. The
	// queue's static sink pad breaks that pair, so the link resolves without pinning a
	// tee pad number.
	if len(audio) > 0 || len(taps) > 0 {
		pipeline = append(pipeline, "queue", "!")
	}
	pipeline = append(pipeline, sink...)
	pipeline = append(pipeline, audio...)
	return pipeline, nil
}

// gstGop returns the keyframe interval in frames. A settings value of zero is the
// form's automatic setting, a keyframe every two seconds. It is the counterpart of
// the ffmpeg builder's gopFor, and the encoder probe reads it as well, so a probe
// and the run it predicts code at the same interval.
func gstGop(s settings.Settings) int {
	if s.Publish.Gop > 0 {
		return s.Publish.Gop
	}
	return s.Publish.Fps * 2
}

// gstSourceOptions builds the parts of the source chain that follow from the tables
// alone: where the frames reach the encoder, the caps stating it, and the element
// converting into them. What a run adds on top is its instrumentation, and what the
// engine adds is the check that the machine can hold both ends on one device.
func gstSourceOptions(s settings.Settings) (gstCaptureOptions, error) {
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
func gstInputCaps(s settings.Settings, mem gstFrameMemory) (string, error) {
	if err := capabilities.Validate(EngineGst, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM); err != nil {
		return "", err
	}
	if err := transport.ValidatePublish(s.Publish.Transport, EngineGst, s.Publish.Codec); err != nil {
		return "", err
	}
	if err := capabilities.ValidateAudio(EngineGst, s.Publish.AudioTrack()); err != nil {
		return "", err
	}
	if err := transport.ValidatePublishAudio(s.Publish.Transport, EngineGst, s.Publish.AudioTrack()); err != nil {
		return "", err
	}
	if err := transport.ValidatePublishSettings(s); err != nil {
		return "", err
	}
	if s.Publish.Fps <= 0 {
		return "", fmt.Errorf("the GStreamer publish engine needs a positive fps, got %d", s.Publish.Fps)
	}
	return gstEncoderCaps(s, mem)
}

// gstEncoderCaps renders the capsfilter itself, without the checks gstInputCaps
// runs ahead of it.
//
// The split is the encoder probe's: it pins the same encoder input to time the
// encoder, and the transport checks would refuse the measurement over a leg that
// is no part of what it measures. What the caps depend on is checked here all the
// same, since both halves come from a table that can be missing the row.
func gstEncoderCaps(s settings.Settings, mem gstFrameMemory) (string, error) {
	format, err := gstChromaFormat(s.Publish.Codec, s.Publish.Chroma, mem.memory)
	if err != nil {
		return "", err
	}
	colorimetry, err := gstColorimetry(s)
	if err != nil {
		return "", err
	}

	caps := "video/x-raw" + mem.feature + ",format=" + format + ",colorimetry=" + colorimetry
	// The size is pinned on the encoder input rather than asked of a scaler, which is
	// what GStreamer negotiation is: the capsfilter states what the encoder is given and
	// the resampler upstream of it - videoscale on the CPU, the family's post-processor
	// on the device - produces it. That is the same statement on both paths, so the
	// device path needs no element of its own (gstgpu.go, gstSystemScale).
	size, scaled, err := s.Publish.OutputSize()
	if err != nil {
		return "", err
	}
	if scaled {
		caps += ",width=" + strconv.Itoa(size.Width) + ",height=" + strconv.Itoa(size.Height)
	}
	return caps, nil
}

// gstAudioBranch returns the elements that capture desktop audio and attach it
// to the muxer as a second track, or nil when audio is off.
//
// pulsesrc records platform.AudioMonitorDevice, the libpulse magic name for the
// monitor of the default sink: the mixed desktop audio. An attached record stream
// keeps the monitor source running, so silence flows even while nothing plays and
// the muxer's audio pad never starves. A backend whose platform serves no such
// source is refused before the elements are built, on the verdict and in the words
// publish.AudioAvailable reads off the source table - the same table the form greys
// the option by, so what a user is told before publishing and what a refused publish
// says are one sentence.
//
// The encoder element, the parser after it and the rate the capsfilter pins all
// come from the audio table. The capsfilter sits after audioresample because an
// encoder codes at one rate whatever rate the monitor runs at, and the parser is
// what puts the framed caps a muxer pad negotiates on the coded stream.
func gstAudioBranch(s settings.Settings) ([]string, error) {
	switch s.Publish.Audio {
	case "", platform.AudioSourceNone:
		return nil, nil
	case platform.AudioSourceDesktop:
		if available, _ := AudioAvailable(s.Publish.Capture, s.Publish.Audio); !available {
			return nil, fmt.Errorf("the %s backend cannot record %s audio", s.Publish.Capture, s.Publish.Audio)
		}
		a, ok := capabilities.GetAudio(s.Publish.AudioTrack())
		if !ok {
			return nil, fmt.Errorf("unknown audio codec %q", s.Publish.AudioTrack())
		}
		enc, ok := a.EncoderOn(EngineGst)
		if !ok {
			return nil, fmt.Errorf("audio codec %s has no GStreamer encoder", a.Name)
		}
		assert.Assert(enc.Parser != "", "a GStreamer audio encoder states its parser", a.Name)
		return []string{
			"pulsesrc", "device=" + platform.AudioMonitorDevice,
			"!", "queue",
			"!", "audioconvert",
			"!", "audioresample",
			"!", fmt.Sprintf("audio/x-raw,rate=%d,channels=2", a.Rate),
			"!", enc.Element, fmt.Sprintf("bitrate=%d", a.BitrateK*1000),
			"!", enc.Parser,
			"!", transport.GstMuxName + ".",
		}, nil
	default:
		return nil, fmt.Errorf("unknown audio source %q", s.Publish.Audio)
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
	"yuv422p": "Y42B",
	"yuv444p": "Y444",
	"p010le":  "I420_10LE",
}

// gstVaChromaFormats is the same mapping for the va plugin's encoders, which take
// the semi-planar layouts the VAAPI drivers store surfaces in and negotiate no
// planar format at all. It is the GStreamer counterpart of vaapiFormats in the
// ffmpeg builder, and the reason capabilities.Codecs declares no other chroma for
// the family.
//
// The same layouts on either path: a VA surface holds what these elements read whether
// vapostproc converted into it or the frames were converted on the CPU and uploaded, so
// the family's device row names this map as well (gstGpuMemories).
var gstVaChromaFormats = map[string]string{
	"yuv420p": "NV12",
	"p010le":  "P010_10LE",
}

// gstNvChromaFormats is the mapping for the nvcodec plugin's encoders, which negotiate
// the semi-planar 4:2:0 and 10-bit layouts and the packed 4:4:4 one, and no planar YUV
// at all: handed I420, Y42B or I420_10LE they refuse to link.
//
// The 4:4:4 and RGB entries are not every element's. Only the HEVC elements take GBR,
// and the AV1 one takes neither it nor Y444, which costs nothing here because the
// chroma a codec may be published at is its own row in capabilities.Codecs and no
// nvenc row offers a layout its element lacks. This map therefore states the family's
// union and the rows narrow it, the same division gstChromaFormats works under.
//
// The auto-GPU elements that encode the family's Direct3D 11 surfaces negotiate the same
// union there, so the device row names this map as well (gstGpuMemories).
var gstNvChromaFormats = map[string]string{
	"yuv420p": "NV12",
	"yuv444p": "Y444",
	"p010le":  "P010_10LE",
	"gbrp":    "GBR",
}

// gstFamilyChromaFormats is the raw-format mapping per encoder family for frames in
// system memory, keyed as capabilities.Codecs names the family. Every family with a row
// in gstCodecs carries an entry, so which layout an element negotiates is stated rather
// than assumed: a family added there without one is refused, where taking the planar
// layouts by default would pin caps its elements do not negotiate and the pipeline would
// fail in negotiation instead of naming the family.
//
// The device path reads gstGpuMemories instead, one family's device elements negotiating
// what they negotiate rather than what its system ones do (gstRawFormats).
//
// The QSV entry is the semi-planar one because the qsv plugin drives oneVPL over VA
// on Linux and D3D11 on Windows, and both store surfaces in the layouts the VAAPI
// drivers do.
var gstFamilyChromaFormats = map[string]map[string]string{
	capabilities.FamilySoftware: gstChromaFormats,
	capabilities.FamilyNvenc:    gstNvChromaFormats,
	capabilities.FamilyVaapi:    gstVaChromaFormats,
	capabilities.FamilyQsv:      gstVaChromaFormats,
}

// gstChromaFormat returns the raw format the capture chain pins ahead of the codec's
// encoder element, for frames reaching it in the resolved memory: a family's device
// elements can negotiate other layouts than its system ones, and gstRawFormats is where
// the memory decides which mapping applies.
func gstChromaFormat(codec, chroma, memory string) (string, error) {
	c, ok := capabilities.Get(codec)
	if !ok {
		return "", fmt.Errorf("unknown codec %q", codec)
	}
	formats, ok := gstRawFormats(c.Family, memory)
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
func gstColorimetry(s settings.Settings) (string, error) {
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
func gstColorRange(s settings.Settings) (string, error) {
	r, ok := gstColorRanges[s.Publish.ColorRange]
	if !ok {
		return "", fmt.Errorf("colour range %q has no GStreamer mapping, expected pc or tv", s.Publish.ColorRange)
	}
	return r, nil
}
