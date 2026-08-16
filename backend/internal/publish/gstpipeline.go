package publish

import (
	"fmt"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpu"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// gstEncodeQueue sheds ahead of the encoder: the newest frame waits, the rest are dropped.
//
// The capture paces itself off the clock, so an encoder or a transport short of that rate has to cost
// frames here.
// Without the drop the encode holds the capture up and every frame behind it ages by what the wait
// cost, which is a picture growing later for as long as the shortfall lasts.
// Dropping is on this side of the encoder because a dropped encoded frame is a reference a viewer
// decodes without.
var gstEncodeQueue = []string{"queue", "name=" + gstShedName, "max-size-buffers=1", "leaky=downstream"}

// buildPipeline renders the whole gst-launch description: the source it is handed, the encoder the
// codec resolves to, and the transport's muxer and sink.
// capture is what the backend built, which is the only thing a run and the displayed command differ
// in.
// An empty meterPort and an unwanted preview leave those branches out.
func buildPipeline(s settings.Settings, capture []string, meterPort string, preview PreviewLeg) ([]string, error) {
	if err := capabilities.Validate(EngineGst, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM, s.Publish.Gop, gpu.Device()); err != nil {
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
	// Resolved again here rather than carried over from the source, the memory deciding the encoder
	// element too: a family whose plugin ships one element per memory kind is encoded by a different
	// one on each path.
	// Both resolutions read the same tables off the same settings, so source and encoder cannot
	// disagree about where the frames are.
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
	pipeline = append(pipeline, gstEncodeQueue...)
	pipeline = append(pipeline, "!")
	pipeline = append(pipeline, encoder...)
	pipeline = append(pipeline, "!")
	// A codec whose element leaves a parser or capsfilter nothing to do links straight to the sink.
	if len(link) > 0 {
		pipeline = append(pipeline, link...)
		pipeline = append(pipeline, "!")
	}
	// Ahead of the tee, so the count is the stream and not one branch of it.
	if meterPort != "" {
		pipeline = append(pipeline, gstProgressElement...)
		pipeline = append(pipeline, "!")
	}
	// Both taps copy what the encoder already produced, so neither the meter nor the preview costs a
	// second encode.
	var taps [][]string
	if meterPort != "" {
		taps = append(taps, gstMeterTap(meterPort))
	}
	if preview.Wanted() {
		// A format with no local carriage publishes without a preview rather than not at all, the same
		// answer the ffmpeg engine gives.
		if tap, err := gstPreviewTap(s.Publish.Codec, preview); err == nil {
			taps = append(taps, tap)
		}
	}
	pipeline = append(pipeline, gstTapElements(taps)...)
	// With audio the muxer waits on two pads, and the queue keeps a stall on one from blocking the
	// branch upstream of the other.
	// A tap needs it for a parsing reason: the tee and every muxer and sink here expose request pads
	// only, and gst-launch links no two unnamed request pads.
	// The queue's static sink pad breaks that pair without pinning a tee pad number.
	if len(audio) > 0 || len(taps) > 0 {
		pipeline = append(pipeline, "queue", "!")
	}
	pipeline = append(pipeline, sink...)
	pipeline = append(pipeline, audio...)
	return pipeline, nil
}

// gstGop is the keyframe interval in frames.
// Zero is the form's automatic setting, a keyframe every two seconds.
// The encode probe resolves it through here too, so a probe and the run it predicts code at one
// interval (the ffmpeg builder's gopFor is the counterpart).
func gstGop(s settings.Settings) int {
	if s.Publish.Gop > 0 {
		return s.Publish.Gop
	}
	return s.Publish.Fps * 2
}

// gstSourceOptions builds what the source chain takes from the tables alone: where the frames reach
// the encoder, the caps stating it, and the element converting into them.
// A run adds its instrumentation on top, and the engine adds the check that this machine holds both
// ends on one device.
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

// gstInputCaps is the capsfilter every capture backend ends in, and the refusal for a settings
// combination this engine cannot encode.
// The engine calls it before it acquires anything, so a forbidden combination fails without opening
// a portal session or an X display.
//
// The filter pins the encoder input to the configured chroma, ffmpeg's -pix_fmt: left free,
// x264enc negotiates its own preference, 4:4:4 and often 10-bit, which not every viewer or browser
// decodes.
// The colorimetry field pins the quantization range and the colour space with it, ffmpeg's
// -color_range.
//
// The memory feature leads the caps because it decides which pads link at all.
// Plain video/x-raw is system memory, so a filter omitting the feature on the GPU path both fails
// negotiation against a source offering device memory alone and pins the frames back into the round
// trip that path exists to avoid.
func gstInputCaps(s settings.Settings, mem gstFrameMemory) (string, error) {
	if err := capabilities.Validate(EngineGst, s.Publish.Codec, s.Publish.CapabilityOptions(), s.Publish.Cq, s.Publish.BitrateM, s.Publish.Gop, gpu.Device()); err != nil {
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

// gstEncoderCaps renders the capsfilter alone, without the checks gstInputCaps runs ahead of it.
//
// The split is the encode probe's: it pins the same encoder input, and a transport check would
// refuse a measurement over a leg that is no part of what it measures.
// What the caps themselves depend on is checked here all the same, both mappings coming from tables
// that can be missing the row.
func gstEncoderCaps(s settings.Settings, mem gstFrameMemory) (string, error) {
	format, err := gstChromaFormat(s.Publish.Codec, s.Publish.Chroma, mem.memory)
	if err != nil {
		return "", err
	}
	colorimetries, err := gstColorimetries(s)
	if err != nil {
		return "", err
	}

	// The size is pinned on the encoder input rather than asked of a scaler: the resampler upstream
	// produces what the filter states, videoscale on the CPU and the family's post-processor on the
	// device.
	// One statement for both paths, so the device path needs no element of its own (gstgpu.go,
	// gstSystemScale).
	size := ""
	out, scaled, err := s.Publish.OutputSize()
	if err != nil {
		return "", err
	}
	if scaled {
		size = ",width=" + strconv.Itoa(out.Width) + ",height=" + strconv.Itoa(out.Height)
	}

	// One structure per colour the encoder input accepts, an alternative a value list cannot state:
	// videoconvert fixates a list to its first entry whatever the frames carry, so a list would
	// convert an HDR surface into the first row and call it negotiation.
	// Structures are what the child narrows before anything negotiates (gstrun, "Narrowing the
	// encoder input").
	//
	// The order is the answer for a run nobody narrows, a pasted command or the encode probe,
	// and the standard-range row leads because that is what a capture stating no transfer is.
	caps := make([]string, 0, len(colorimetries))
	for _, colorimetry := range colorimetries {
		caps = append(caps, "video/x-raw"+mem.feature+",format="+format+",colorimetry="+colorimetry+size)
	}
	return strings.Join(caps, ";"), nil
}

// gstAudioBranch is the chain mixing the recorded sources into the muxer's second track,
// nil where nothing is recorded.
//
// A backend whose platform serves no source is refused before an element is built, in the words
// publish.AudioAvailable reads off the source table, which is the table the form greys the option
// by: what a user is told beforehand and what a refused publish says are one sentence.
//
// Encoder element, parser and coded rate all come from the audio table.
// The capsfilter sits after audioresample because an encoder codes at one rate whatever rate a
// device runs at, and the parser is what puts the framed caps a muxer pad negotiates on the coded
// stream.
func gstAudioBranch(s settings.Settings) ([]string, error) {
	recorded := s.Publish.Recorded()
	if len(recorded) == 0 {
		return nil, nil
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

	// One track and not several is carriage rather than preference: RTMP carries one audio track and
	// the relay re-serves every ingest on all of its listeners, so a two-track stream would be
	// unplayable on the narrowest leg while the form said it published.
	//
	// audiomixer rather than adder, the sources being live and unsynchronised: adder mixes sample for
	// sample and drifts the moment one of them is late, where the mixer aligns on running time.
	// The receive side takes one for the same reason.
	branch := []string{}
	for i, source := range recorded {
		chain, err := gstAudioSource(s, source, i)
		if err != nil {
			return nil, err
		}
		branch = append(branch, chain...)
	}
	return append(branch,
		gstAudioMixName, "!", "audioconvert",
		"!", "audioresample",
		"!", fmt.Sprintf("audio/x-raw,rate=%d,channels=2", a.Rate),
		"!", enc.Element, fmt.Sprintf("bitrate=%d", a.BitrateK*1000),
		"!", enc.Parser,
		"!", transport.GstMuxName+".",
	), nil
}

// gstAudioMixName declares the mixer every source feeds, so a source chain can name it and the
// encoder chain can read from it.
const gstAudioMixName = "audiomixer name=" + gstAudioMixElement

// gstAudioMixElement is the mixer's element name alone, what a source chain links into.
const gstAudioMixElement = "amix"

// gstAudioVolumeName is one source's volume element, indexed by that source's place in the list,
// so a live gain write reaches exactly one branch.
func gstAudioVolumeName(i int) string {
	return fmt.Sprintf("%s%d", gstAudioVolumeElement, i)
}

// gstAudioVolumeElement is the prefix every volume element's name carries.
const gstAudioVolumeElement = "gain"

// gstAudioOpen is how one element opens one kind: the element, the property a handle rides on, the
// handle the kind's own default answers to, and whatever else the element needs to record that kind
// rather than another.
//
// self is empty where the element opens its own default and takes no handle for it, which is what
// separates a sound server addressing devices by name from an API that has a default of its own.
type gstAudioOpen struct {
	element    string
	handle     string
	self       string
	properties []string
}

// gstAudioElements is the element each kind is recorded through, per operating system.
//
// Keyed by the platform because the elements are the platform's: a sound server's clients on one, an
// operating system's audio API on the other, and no element spans both.
// A pair with no row is one this engine does not record, which the source table and the engine gate
// beside it have already refused (AudioAvailable), so reaching one here is a settings file that took
// another route.
//
// The Linux rows address devices by the libpulse magic names, one spelling shared with the ffmpeg
// engine (platform.AudioMonitorDevice).
// An application is a PipeWire node and not a sound device, so its row takes a different element and
// a different property, which is why the element is a table read rather than a device string.
//
// The Windows row takes no handle for the default: wasapi2src opens the default render device on
// its own, and loopback is what records what that device plays rather than what it hears.
// Per-application capture has no row there: it is a process id on that API rather than a device, and
// nothing enumerates one, so the kind stays refused on Windows rather than opened as the desktop.
var gstAudioElements = map[string]map[string]gstAudioOpen{
	"linux": {
		platform.AudioSourceDesktop:     {element: "pulsesrc", handle: "device", self: platform.AudioMonitorDevice},
		platform.AudioSourceApplication: {element: "pipewiresrc", handle: "target-object"},
	},
	"windows": {
		platform.AudioSourceDesktop: {element: "wasapi2src", handle: "device", properties: []string{"loopback=true"}},
	},
}

// gstAudioSource is one recorded source's chain, from its device into the mixer.
//
// The volume element carries gain and mute both, one value to an element that multiplies:
// a muted source is one at zero, which is what keeps unmuting a write to a running pipeline rather
// than a rebuild of the graph.
//
// The queue puts each source on a thread of its own.
// Without it the mixer pulls them in turn and the slowest device paces every other one.
// A recording client keeps a monitor source running, so silence flows while nothing plays and the
// muxer's audio pad never starves.
func gstAudioSource(s settings.Settings, a settings.AudioSource, i int) ([]string, error) {
	if available, _ := AudioAvailable(s.Publish.Capture, a.Source); !available {
		return nil, fmt.Errorf("the %s backend cannot record %s audio", s.Publish.Capture, a.Source)
	}
	need, gated := needsOf(s.Publish.Capture)
	assert.Assert(gated, "a capture backend building an audio branch is a registered one", s.Publish.Capture)

	open, mapped := gstAudioElements[need.os][a.Source]
	if !mapped {
		return nil, fmt.Errorf("no GStreamer element records %s audio on %s", a.Source, need.os)
	}
	source := append([]string{open.element}, open.properties...)
	if handle := a.Device; handle != "" {
		source = append(source, open.handle+"="+handle)
	} else if open.self != "" {
		source = append(source, open.handle+"="+open.self)
	}
	return append(source, []string{
		"!", "queue",
		"!", "audioconvert",
		"!", "volume", "name=" + gstAudioVolumeName(i), fmt.Sprintf("volume=%.3f", a.Volume()),
		"!", gstAudioMixElement + ".",
	}...), nil
}

// gstChromaFormats maps a settings chroma, spelled as ffmpeg names the pixel format, to the
// video/x-raw format of the same subsampling and bit depth.
// The 10-bit row is the planar layout, these elements negotiating no semi-planar input:
// what it promises is 10-bit 4:2:0 and not p010le's byte order.
//
// gbrp is absent because no encoder element here takes planar RGB.
// It is a per-engine chroma gap (gstNoPlanarRGB) refused by capabilities.Validate, so nothing here
// decides whether to convert RGB to YUV behind the user's back.
var gstChromaFormats = map[string]string{
	"yuv420p": "I420",
	"yuv422p": "Y42B",
	"yuv444p": "Y444",
	"p010le":  "I420_10LE",
}

// gstSemiPlanarChromaFormats is the same mapping for the elements that take the semi-planar layouts
// a fixed-function encoder stores surfaces in and negotiate no planar format at all.
// It is why capabilities.Codecs declares no other chroma for those families (vaapiFormats is the
// ffmpeg counterpart).
//
// Three families read it, and one layout is why: the va elements take what the VAAPI drivers store,
// the qsv plugin drives oneVPL over VA on Linux and D3D11 on Windows, and Apple's media block reads
// the CoreVideo buffers of the same two layouts.
// One map rather than three copies, since a second spelling of one layout is a second thing able to
// disagree about what an encoder reads.
//
// One mapping for both paths on the families that have a device row: a VA surface holds what those
// elements read whether vapostproc converted into it or CPU-converted frames were uploaded, so the
// device row names this map as well (gstGpuMemories).
var gstSemiPlanarChromaFormats = map[string]string{
	"yuv420p": "NV12",
	"p010le":  "P010_10LE",
}

// gstNvChromaFormats is the mapping for the nvcodec plugin's encoders: the semi-planar 4:2:0 and
// 10-bit layouts, the packed 4:4:4 one, and no planar YUV at all.
// Handed I420, Y42B or I420_10LE those elements refuse to link.
//
// The 4:4:4 and RGB rows are not every element's: only the HEVC elements take GBR, and the AV1 one
// takes neither it nor Y444.
// The map states the family's union and capabilities.Codecs narrows it per codec, the same division
// gstChromaFormats works under.
//
// The auto-GPU elements encoding the family's Direct3D 11 surfaces negotiate the same union there,
// so the device row names this map as well (gstGpuMemories).
var gstNvChromaFormats = map[string]string{
	"yuv420p": "NV12",
	"yuv444p": "Y444",
	"p010le":  "P010_10LE",
	"gbrp":    "GBR",
}

// gstFamilyChromaFormats is the raw-format mapping per encoder family for frames in system memory,
// keyed as capabilities.Codecs names the family.
// Every family with a row in gstCodecs carries an entry, and one added there without an entry is
// refused: defaulting to the planar layouts would pin caps its elements do not negotiate and fail
// in negotiation instead of naming the family.
//
// The device path reads gstGpuMemories instead, a family's device elements negotiating what they
// negotiate rather than what its system ones do (gstRawFormats).
//
// QSV takes the semi-planar mapping because the qsv plugin drives oneVPL over VA on Linux and D3D11
// on Windows, both storing surfaces in the layouts the VAAPI drivers use.
var gstFamilyChromaFormats = map[string]map[string]string{
	capabilities.FamilySoftware:     gstChromaFormats,
	capabilities.FamilyNvenc:        gstNvChromaFormats,
	capabilities.FamilyVaapi:        gstSemiPlanarChromaFormats,
	capabilities.FamilyQsv:          gstSemiPlanarChromaFormats,
	capabilities.FamilyVideoToolbox: gstSemiPlanarChromaFormats,
}

// gstChromaFormat is the raw format the capture chain pins ahead of the codec's encoder element,
// for frames reaching it in the resolved memory.
// A family's device elements negotiate other layouts than its system ones, and gstRawFormats is
// where the memory decides which mapping applies.
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

// The range component of the encoder-input colorimetry, as GstVideoColorRange: 0_255 is 1,
// 16_235 is 2.
const (
	gstRangeFull    = "1"
	gstRangeLimited = "2"
)

// The colour spaces the encoder input takes, spelled after the range as the GstVideoColorMatrix,
// GstVideoTransferFunction and GstVideoColorPrimaries enum values.
const (
	// gstBt709 is the colour space of every standard-range HD and larger picture, which is every SDR
	// screen this app captures.
	// The encoders write it into the bitstream, so a viewer converts back with the matrix the frames
	// were made with rather than one picked off the picture size.
	gstBt709 = "3:5:1"
	// The two BT.2100 curves an HDR surface carries, the absolute one mastered content is graded on
	// and the broadcast one.
	// Both ride the BT.2020 matrix and primaries, which is what makes them two rows and not four.
	gstBt2100Pq  = "6:14:7"
	gstBt2100Hlg = "6:15:7"
)

// gstColorimetry is the colour taken from a capture that states no transfer of its own, which is
// every standard-range surface.
func gstColorimetry(s settings.Settings) (string, error) {
	colorimetries, err := gstColorimetries(s)
	if err != nil {
		return "", err
	}
	return colorimetries[0], nil
}

// gstColorimetries is every colour the encoder input accepts, standard range first, and the refusal
// for a colour range this engine has no mapping for.
//
// All four components are named because a partial colorimetry is not partially applied.
// Left as "<range>:0:0:0", videoconvert drops the range along with the three unknown components and
// converts to limited range whatever the range said, so full-range white leaves the capture chain
// at Y=235 exactly like limited-range white.
// Spelled out, the range takes effect (Y=254) and the stream signals what it holds.
//
// The HDR rows are offered where the pixel format can hold them and nowhere else.
// An HDR surface cannot ride in eight bits, so offering them on an 8-bit format would offer a
// negotiation producing a stream tagged PQ over eight-bit samples.
// That combination is refused on the capture's own report instead (gsthdr.go).
//
// Which row a run ends on is the capture's answer and never a setting: the child narrows them to
// the transfer the surface carries, before anything negotiates.
func gstColorimetries(s settings.Settings) ([]string, error) {
	r, err := gstColorRange(s)
	if err != nil {
		return nil, err
	}
	out := []string{r + ":" + gstBt709}
	if s.Publish.Chroma == tenBitChroma {
		out = append(out, r+":"+gstBt2100Pq, r+":"+gstBt2100Hlg)
	}
	return out, nil
}

// gstColorRanges maps a settings colour range to its GstVideoColorRange value.
// Every chroma this engine encodes is a YUV one, so the range always applies: gbrp, full range by
// construction, does not reach this engine (gstNoPlanarRGB).
var gstColorRanges = map[string]string{
	"pc": gstRangeFull,
	"tv": gstRangeLimited,
}

// gstColorRange is the range the encoder input is pinned to, matching the ffmpeg builder's
// -color_range.
//
// A value with no mapping is refused rather than read as limited.
// The range rides in the bitstream and decides how every viewer expands the picture, so
// substituting one would change what the stream looks like with nothing said.
// The ffmpeg engine passes the same field straight to -color_range, which fails loudly on a value
// it does not know.
func gstColorRange(s settings.Settings) (string, error) {
	r, ok := gstColorRanges[s.Publish.ColorRange]
	if !ok {
		return "", fmt.Errorf("colour range %q has no GStreamer mapping, expected pc or tv", s.Publish.ColorRange)
	}
	return r, nil
}
