package ffmpeg

import (
	"fmt"
	"path/filepath"
)

// The VAAPI half of the publish command: the device the encoder runs on, and the
// filter chain that hands it frames it can read.
//
// A VAAPI encoder's only pixel format is the opaque "vaapi" one: it encodes from
// GPU surfaces. Every capture backend produces system memory instead (x11grab and
// the Windows grabbers copy, kmsgrab downloads its scanout buffer), so the chain
// converts each frame to the layout the driver stores and uploads it. That costs a
// round trip on kmsgrab, which had the frame on the GPU already; keeping it there
// would mean a VAAPI-mapped scanout buffer and a driver-side colour conversion,
// which is a capture-path change rather than an encoder one.

// vaapiFormats maps a settings chroma to the VAAPI surface layout carrying it. The
// drivers store 4:2:0 semi-planar, 8-bit as nv12 and 10-bit as p010, which is why
// capabilities.Codecs declares no other chroma for the family.
var vaapiFormats = map[string]string{
	"yuv420p": "nv12",
	"p010le":  "p010",
}

// VaapiDevice returns the global option opening the VAAPI device that both the
// upload filter and the encoder use. It fails when the machine exposes no render
// node, which is the same condition that makes every VAAPI codec fail its probe,
// so the UI has already greyed them out by the time this could be reached.
func VaapiDevice() ([]string, error) {
	node := vaapiRenderNode()
	if node == "" {
		return nil, fmt.Errorf("no VAAPI render node under /dev/dri: this machine has no usable VAAPI device")
	}
	return []string{"-vaapi_device", node}, nil
}

// VaapiFilters returns the filter chain converting frames to chroma's VAAPI
// layout and uploading them to the device.
//
// The conversion is a filter rather than a -pix_fmt because the encoder's own
// pixel format is the hardware one. It still honours the configured quantization
// range: ffmpeg propagates the encoder's -color_range through the filter graph, so
// the format filter converts to the range the stream signals.
func VaapiFilters(chroma string) ([]string, error) {
	format, ok := vaapiFormats[chroma]
	if !ok {
		return nil, fmt.Errorf("chroma %q has no VAAPI surface layout", chroma)
	}
	return []string{"format=" + format, "hwupload"}, nil
}

// vaapiRenderNode returns the /dev/dri render node to encode on, or "" when none
// carries a real driver. A render node is the unprivileged half of a DRM device
// and the conventional VAAPI target. As in drmCaptureDevice, the lowest-numbered
// node is not a safe default on its own: the boot framebuffer can hold an early
// one, so a node whose kernel driver is missing or is the simple framebuffer is
// skipped.
func vaapiRenderNode() string {
	nodes, err := filepath.Glob("/dev/dri/renderD[0-9]*")
	if err != nil {
		return ""
	}
	for _, node := range nodes {
		switch captureDriver(node) {
		case "", "simple-framebuffer":
			continue
		default:
			return node
		}
	}
	return ""
}
