package ffmpeg

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// The system-memory half of the publish command for the encoder families that read GPU
// surfaces: VAAPI, QSV and Vulkan Video. Their own pixel format is the opaque hardware
// one, so the command opens a device ahead of the input and ends the capture chain in a
// conversion and an upload instead of pinning a -pix_fmt.
//
// This is the path for every pair the GPU one does not cover (gpupath.Paths): the
// capture arrives in system memory, swscale converts it there, and the upload hands the
// encoder surfaces it can read. Where the pair does have a GPU path, the frames never
// become software ones and gpu.go replaces all three pieces at once.
//
// The device is the family's own, since the three open a GPU differently. The
// conversion is shared: all three take the same two semi-planar layouts. The upload
// attaches to whichever device the command created, and carries the surface count the
// family's encoder holds in flight.

// hwSurfaceFormats maps a settings chroma to the memory layout the upload converts to.
// VAAPI drivers store 4:2:0 semi-planar, 8-bit as nv12 and 10-bit as p010, and the QSV
// and Vulkan encode paths take the same two, which is why capabilities.Codecs declares
// no other chroma for those families.
var hwSurfaceFormats = map[string]string{
	"yuv420p": "nv12",
	"p010le":  "p010",
}

// qsvExtraFrames is the number of surfaces the upload adds to its pool beyond what the
// filter graph itself needs, for the frames the QSV encoder holds: oneVPL runs an
// asynchronous pipeline and keeps several in flight, and it takes them from the pool
// hwupload allocated. A pool sized for the graph alone stalls the encode once they are
// all held. The count covers the encoder's async depth with room to spare and stays far
// below the transcode figure ffmpeg's own QSV examples carry, which would size a 4K
// 10-bit pool into a gigabyte of VRAM for frames a live encode never has in flight.
const qsvExtraFrames = "16"

// hwSurface is one encoder family's surface path: the options opening the device its
// encoder runs on, and the upload element that hands it frames.
type hwSurface struct {
	device func() ([]string, error)
	upload string
}

// hwSurfaces is the surface path per family, keyed as capabilities.Codecs names them. A
// family absent here encodes from system memory and needs no device.
var hwSurfaces = map[string]hwSurface{
	capabilities.FamilyVaapi:  {device: VaapiDevice, upload: "hwupload"},
	capabilities.FamilyVulkan: {device: VulkanDevice, upload: "hwupload"},
	capabilities.FamilyQsv:    {device: QsvDevice, upload: "hwupload=extra_hw_frames=" + qsvExtraFrames},
}

// HwSurfaceDevice returns the global options opening the device codec's encoder runs
// on, and false for a codec that reads system memory and needs none. The error is the
// family's own: this machine carries no device it can encode on.
func HwSurfaceDevice(codec string) ([]string, bool, error) {
	// The command builder validates the settings ahead of this, so an empty codec
	// is a caller that skipped it rather than a settings file naming nothing.
	assert.Assert(codec != "", "a publish command names the codec it encodes with")

	c, ok := capabilities.Get(codec)
	if !ok {
		return nil, false, fmt.Errorf("unknown codec %q", codec)
	}
	surface, ok := hwSurfaces[c.Family]
	if !ok {
		return nil, false, nil
	}
	device, err := surface.device()
	assert.Assert(err != nil || len(device) > 0, "a surface family yields the options opening its device", c.Family)
	return device, true, err
}

// HwSurfaceFilters returns the filter chain converting frames to chroma's hardware
// layout and uploading them to the device codec's encoder runs on.
//
// The conversion is a filter rather than a -pix_fmt because the encoder's own pixel
// format is the hardware one. It still honours the configured quantization range:
// ffmpeg propagates the encoder's -color_range through the filter graph, so the format
// filter converts to the range the stream signals.
func HwSurfaceFilters(codec, chroma string) ([]string, error) {
	// capabilities.Validate has already held the chroma against the codec's list,
	// and no row on it is empty.
	assert.Assert(chroma != "", "a surface encode names the pixel format it uploads")

	c, ok := capabilities.Get(codec)
	if !ok {
		return nil, fmt.Errorf("unknown codec %q", codec)
	}
	surface, ok := hwSurfaces[c.Family]
	// Only the callers that HwSurfaceDevice answered "surface" reach here, so a family
	// with no surface path is a caller that skipped that question.
	assert.Assert(ok, "a surface encode names a codec whose family reads GPU surfaces", codec)

	format, ok := hwSurfaceFormats[chroma]
	if !ok {
		return nil, fmt.Errorf("chroma %q has no hardware surface layout", chroma)
	}
	return []string{"format=" + format, surface.upload}, nil
}
