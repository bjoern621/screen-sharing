package publish

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The device half of the GStreamer publish engine: where a run's frames reach the encoder, and what
// the family's elements read, convert and encode them with once they are there.
//
// gpupath.Paths declares which capture backend and encoder family have a device path at all, and
// what it does to the colour.
// The rows here are that table's engine half: the caps feature, the converter, the layouts
// the family's device elements negotiate, and the element encoding from them.
// The two halves are checked against each other, a pair with no half here being one no pipeline
// reaches.

// gstSystemConvert is the CPU conversion, which every pair without a GPU path runs whichever end
// lacks one.
const gstSystemConvert = "videoconvert"

// gstSystemScale is the CPU resampler, placed ahead of the conversion on a run that scales.
//
// Ahead and not after: the captured frames are RGB and the conversion produces the encoder's
// subsampled layout, so scaling last would resample chroma already thrown away.
// The device path needs no counterpart, vapostproc, d3d11convert and qsvvpp resizing as part
// of the negotiation, so pinning the size on the encoder input is the whole of what asks them to.
const gstSystemScale = "videoscale"

// gstGpuMemory is how one encoder family's elements read frames on the GPU path: the caps feature
// its surfaces carry, and the element converting captured frames into them without leaving
// the device.
type gstGpuMemory struct {
	feature string
	convert string
	// formats maps a settings chroma to the raw format the family's device elements negotiate,
	// in the vocabulary gstFamilyChromaFormats uses for system memory.
	// Its own mapping, a plugin's device elements not being its system ones, and stated on every row,
	// so which layout a run pins follows from where its frames are rather than from a fallback
	// that happens to hold.
	formats map[string]string
	// encoders maps a codec to the element encoding it from these surfaces, for a plugin shipping one
	// element per memory kind.
	// Empty for a family whose single element reads frames wherever they live, gstCodecs naming
	// that element for both paths.
	encoders map[string]string
	// upload moves system frames into this memory without converting them.
	// No publish needs it, every backend with a row capturing on the device already.
	// The encode probe does, generating its frames in system memory and having to reach the run's
	// encoder the way the run does.
	// Empty where the converter takes system frames on its own sink pad.
	upload string
}

// gstGpuMemories is the GPU path per encoder family, keyed as capabilities.Codecs names the family,
// and the engine's half of the pairs gpupath.Paths declares.
// A family with a row here and none there has no pipeline reaching it.
// One with a row there and none here is a half-declared pair, which the builder asserts on.
//
// The va elements negotiate VASurface caps and take DMABuf on their sink pads.
// vapostproc is the VA post-processor: it imports the portal's dmabuf, converts to the encoder's
// semi-planar layout and applies the quantization range, all on the device.
// One element per codec encodes out of either memory, so the row names none, and vapostproc takes
// system frames on its sink pad, so nothing uploads into it.
//
// The nvcodec family reaches the device through Direct3D 11, what the Windows capture backend hands
// out.
// d3d11convert is the Direct3D post-processor: it takes the layout and the colorimetry off
// its output caps and states them again downstream, so the colour contract holds there as it does
// on vapostproc.
// The auto-GPU elements read the result: nvh264enc and its siblings negotiate CUDA and D3D12 memory
// and refuse D3D11, where the nvautogpu ones take D3D11, CUDA, GL and system memory alike.
// They also take their adapter off the frames they are handed rather than opening a device of their
// own, which keeps a monitor on a second card from being encoded on the first one's.
//
// The qsv family has no entry: its elements take VA surfaces only through an interop the plugin
// does not negotiate from a foreign dmabuf.
var gstGpuMemories = map[string]gstGpuMemory{
	capabilities.FamilyVaapi: {
		feature: "(memory:VAMemory)",
		convert: "vapostproc",
		formats: gstSemiPlanarChromaFormats,
	},
	capabilities.FamilyNvenc: {
		feature: "(memory:D3D11Memory)",
		convert: "d3d11convert",
		formats: gstNvChromaFormats,
		encoders: map[string]string{
			"h264_nvenc": "nvautogpuh264enc",
			"hevc_nvenc": "nvautogpuh265enc",
			"av1_nvenc":  "nvautogpuav1enc",
		},
		upload: "d3d11upload",
	},
}

// gstFrameMemory is where a run's frames reach the encoder, in the vocabulary the pipeline states
// it in: the caps feature the encoder input carries, and the elements converting into it.
type gstFrameMemory struct {
	// memory is the resolved gpupath value, a device one or MemorySystem.
	// gpupath.OnDevice reads it: a check written against MemoryGpu alone would build the round trip
	// for the other device value.
	memory string
	// feature is the caps feature the encoder input and everything pinned downstream of it carry,
	// empty for system memory.
	feature string
	// convert turns captured frames into the encoder input, one element per entry.
	// A chain rather than an element, a scaled run on the CPU needing two of them, and where
	// the second one sits is this file's fact rather than the placing backend's.
	convert []string
	// upload is what a source of system frames needs ahead of convert, empty where the converter
	// takes them itself (gstGpuMemory.upload).
	upload string
}

// gstMemory resolves the frame memory these settings ask for against the pair table,
// and refuses a demand this engine cannot meet.
//
// Whether the machine holds the device is the caller's check: this answers from the tables alone,
// so a settings combination is refused for what it names and not for the hardware it runs on.
func gstMemory(s settings.Settings) (gstFrameMemory, error) {
	c, ok := capabilities.Get(s.Publish.Codec())
	if !ok {
		return gstFrameMemory{}, fmt.Errorf("unknown codec %q", s.Publish.Codec())
	}
	memory, err := gpupath.Resolve(EngineGst, s.Publish.Capture, c.Family, s.Publish.CaptureMemory)
	if err != nil {
		return gstFrameMemory{}, err
	}
	// A malformed size is refused here rather than at the capsfilter, so the message names the setting
	// rather than a negotiation that failed for reasons of its own.
	_, scaled, err := s.Publish.OutputSize()
	if err != nil {
		return gstFrameMemory{}, err
	}
	if !gpupath.OnDevice(memory) {
		convert := []string{gstSystemConvert}
		if scaled {
			convert = []string{gstSystemScale, "!", gstSystemConvert}
		}
		return gstFrameMemory{memory: memory, convert: convert}, nil
	}
	gpu, ok := gstGpuMemories[c.Family]
	assert.Assert(ok, "a family with a GPU path states the memory its surfaces carry", c.Family)
	return gstFrameMemory{
		memory:  memory,
		feature: gpu.feature,
		convert: []string{gpu.convert},
		upload:  gpu.upload,
	}, nil
}

// gstRawFormats returns the mapping from a settings chroma to the raw format the family's encoder
// elements negotiate for frames arriving in this memory, and false for a family this engine has no
// mapping for.
//
// The memory picks the mapping and exactly one applies, a family's device elements not being
// its system ones.
// Read as a fallback chain instead, both would apply on the device path and the layout a run codes
// at would follow the order the two lookups are written in rather than where its frames are.
func gstRawFormats(family, memory string) (map[string]string, bool) {
	if gpupath.OnDevice(memory) {
		gpu, ok := gstGpuMemories[family]
		assert.Assert(ok, "a family reaching the encoder on the device states how its elements read frames there", family)
		assert.Assert(len(gpu.formats) > 0, "a GPU path states the layouts its elements negotiate on the device", family)
		return gpu.formats, true
	}
	formats, ok := gstFamilyChromaFormats[family]
	return formats, ok
}

// gstDeviceEncoderElement returns the element that encodes codec from frames in this memory,
// and false where the codec's own mapping already names the element that memory is read by.
//
// The nvcodec plugin needs it, shipping one encoder element per memory kind: which element a run
// launches follows from where its frames are and not from the codec alone.
// The rest of the encode is the shared base class's, so this answers with a name and nothing more.
func gstDeviceEncoderElement(family, codec, memory string) (string, bool) {
	if !gpupath.OnDevice(memory) {
		return "", false
	}
	gpu, ok := gstGpuMemories[family]
	assert.Assert(ok, "a family reaching the encoder on the device states how its elements read frames there", family)
	elem, named := gpu.encoders[codec]
	return elem, named
}
