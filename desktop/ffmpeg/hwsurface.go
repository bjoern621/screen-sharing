package ffmpeg

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
)

// The half of the publish command the encoder families that read GPU surfaces need:
// VAAPI and Vulkan Video. Their own pixel format is the opaque hardware one, so the
// command opens a device ahead of the input and ends the capture chain in a
// conversion and an upload instead of pinning a -pix_fmt.
//
// Every capture backend produces system memory (x11grab and the Windows grabbers
// copy, kmsgrab downloads its scanout buffer), so every frame makes that trip. It
// costs a round trip on kmsgrab, which had the frame on the GPU already; keeping it
// there would mean a hardware-mapped scanout buffer and a driver-side colour
// conversion, which is a capture-path change rather than an encoder one.
//
// The device is the family's own, since the two open a GPU differently. The filter
// chain is shared: both take the same two semi-planar layouts, and hwupload attaches
// to whichever device the command created.

// hwSurfaceFormats maps a settings chroma to the memory layout the upload converts to.
// VAAPI drivers store 4:2:0 semi-planar, 8-bit as nv12 and 10-bit as p010, and the
// Vulkan encode profiles take the same two, which is why capabilities.Codecs declares
// no other chroma for either family.
var hwSurfaceFormats = map[string]string{
	"yuv420p": "nv12",
	"p010le":  "p010",
}

// hwSurfaceDevices is the device options per family, keyed as capabilities.Codecs
// names them. A family absent here encodes from system memory and needs no device.
var hwSurfaceDevices = map[string]func() ([]string, error){
	capabilities.FamilyVaapi:  VaapiDevice,
	capabilities.FamilyVulkan: VulkanDevice,
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
	build, ok := hwSurfaceDevices[c.Family]
	if !ok {
		return nil, false, nil
	}
	device, err := build()
	assert.Assert(err != nil || len(device) > 0, "a surface family yields the options opening its device", c.Family)
	return device, true, err
}

// HwSurfaceFilters returns the filter chain converting frames to chroma's hardware
// layout and uploading them to the device.
//
// The conversion is a filter rather than a -pix_fmt because the encoder's own pixel
// format is the hardware one. It still honours the configured quantization range:
// ffmpeg propagates the encoder's -color_range through the filter graph, so the format
// filter converts to the range the stream signals.
func HwSurfaceFilters(chroma string) ([]string, error) {
	// capabilities.Validate has already held the chroma against the codec's list,
	// and no row on it is empty.
	assert.Assert(chroma != "", "a surface encode names the pixel format it uploads")

	format, ok := hwSurfaceFormats[chroma]
	if !ok {
		return nil, fmt.Errorf("chroma %q has no hardware surface layout", chroma)
	}
	return []string{"format=" + format, "hwupload"}, nil
}
