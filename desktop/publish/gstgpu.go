package publish

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/gpupath"
	"bjoernblessin.de/screenshare/settings"
)

// The device half of the GStreamer publish engine: where a run's frames reach the
// encoder, and what each encoder family's elements read, convert and encode them with
// once they are there.
//
// gpupath.Paths says which capture backend and encoder family have a device path at all
// and what it does to the colour. This file is the engine's half of those rows: the caps
// feature, the converter, the layouts the family's device elements negotiate and the
// element that encodes from them. The two halves are checked against each other, a row
// with no half here being a pair no pipeline can reach.

// gstSystemConvert is the element that converts captured frames on the CPU. It is
// the conversion every pair without a GPU path runs, whichever end lacks one.
const gstSystemConvert = "videoconvert"

// gstGpuMemory is how one encoder family's elements read frames on the GPU path: the
// caps feature its surfaces carry, and the element that converts captured frames into
// them without leaving the device.
type gstGpuMemory struct {
	feature string
	convert string
	// formats maps a settings chroma to the raw format the family's elements negotiate
	// on the device, in the vocabulary gstFamilyChromaFormats uses for system memory.
	// It is a mapping of its own because a plugin's device elements are not its system
	// ones, and every row states it: which layout a run pins then follows from where its
	// frames are rather than from a fallback that happens to hold.
	formats map[string]string
	// encoders maps a codec to the element encoding it from these surfaces, for a plugin
	// that ships one element per memory kind. A family whose single element reads frames
	// wherever they live leaves this empty, and gstCodecs names its element for both
	// paths.
	encoders map[string]string
	// upload moves system frames into this memory without converting them. No publish
	// needs it, every backend with a row capturing on the device already; the encode
	// probe does, generating its frames in system memory and having to reach the run's
	// encoder the way the run reaches it. It is empty where the converter takes system
	// frames on its own sink pad.
	upload string
}

// gstGpuMemories is the GPU path per encoder family, keyed as capabilities.Codecs
// names the family, and the engine's half of the pairs gpupath.Paths declares. A
// family named in a row here and not there has no pipeline reaching it; one named
// there and not here is a table half declared, which the builder asserts.
//
// The va elements negotiate VASurface caps and take DMABuf on their sink pads, and
// vapostproc is the VA post-processor: it imports the portal's dmabuf, converts to the
// encoder's semi-planar layout and applies the quantization range, all on the device. One
// element per codec encodes either memory, so the row names none, and vapostproc takes
// system frames on its sink pad as well, so nothing has to upload into it.
//
// The nvcodec family reaches the device through Direct3D 11, which is what the one
// Windows capture backend hands out. d3d11convert is the Direct3D post-processor: it takes
// the layout and the colorimetry from its output caps and states them again downstream, so
// the colour contract holds there exactly as it does on vapostproc. The auto-GPU encoder
// elements are what read the result: nvh264enc and its siblings negotiate CUDA and D3D12
// memory and refuse D3D11, where the nvautogpu ones take D3D11, CUDA, GL and system memory
// alike. They also take their adapter from the frames they are handed rather than opening
// a device of their own, which is what keeps a monitor on a second card from being encoded
// on the first one's.
//
// The qsv family has no entry: its elements take VA surfaces only through an interop the
// plugin does not negotiate from a foreign dmabuf.
var gstGpuMemories = map[string]gstGpuMemory{
	capabilities.FamilyVaapi: {
		feature: "(memory:VAMemory)",
		convert: "vapostproc",
		formats: gstVaChromaFormats,
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

// gstFrameMemory is where a run's frames reach the encoder, in the vocabulary the
// pipeline states it in: the caps feature the encoder input carries, and the element
// that converts into it.
type gstFrameMemory struct {
	// memory is the resolved gpupath value, one of the two device ones or MemorySystem.
	// gpupath.OnDevice is what reads it: a check written against MemoryGpu alone would
	// build the round trip for the other device value.
	memory string
	// feature is the caps feature both the encoder input and everything pinned
	// downstream of it carry, empty for system memory.
	feature string
	convert string
	// upload is the element a source of system frames needs ahead of convert, empty
	// where the converter takes them itself (gstGpuMemory.upload).
	upload string
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
	if !gpupath.OnDevice(memory) {
		return gstFrameMemory{memory: memory, convert: gstSystemConvert}, nil
	}
	gpu, ok := gstGpuMemories[c.Family]
	assert.Assert(ok, "a family with a GPU path states the memory its surfaces carry", c.Family)
	return gstFrameMemory{
		memory:  memory,
		feature: gpu.feature,
		convert: gpu.convert,
		upload:  gpu.upload,
	}, nil
}

// gstRawFormats returns the mapping from a settings chroma to the raw format the
// family's encoder elements negotiate for frames arriving in this memory, and false for
// a family this engine has no mapping for at all.
//
// Exactly one mapping applies and the memory is the whole of what picks it. A family's
// device elements are not its system ones, so which layouts they negotiate is a fact of
// its own; read as a fallback chain both would apply on the device path, and the layout a
// run codes at would follow the order the two lookups are written in rather than where
// its frames are.
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

// gstDeviceEncoderElement returns the element that encodes codec from frames in this
// memory, and false where the codec's own mapping already names the element this memory
// is read by.
//
// The nvcodec plugin is the family that needs it: it ships one encoder element per memory
// kind, so which element a run launches is decided by where its frames are and not by the
// codec alone. Everything else about the encode is the base class's and shared by both
// elements, which is why this answers with the name and nothing more.
func gstDeviceEncoderElement(family, codec, memory string) (string, bool) {
	if !gpupath.OnDevice(memory) {
		return "", false
	}
	gpu, ok := gstGpuMemories[family]
	assert.Assert(ok, "a family reaching the encoder on the device states how its elements read frames there", family)
	elem, named := gpu.encoders[codec]
	return elem, named
}
