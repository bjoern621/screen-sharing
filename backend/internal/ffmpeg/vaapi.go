package ffmpeg

import (
	"fmt"
	"path/filepath"
)

// The VAAPI half of the publish command: the device the encoder runs on.
// The filter chain handing it frames it can read is the shared one (hwsurface.go).
//
// One option carries both jobs here, creating the device and making it the filter graph's,
// which is why this side spells a path where the QSV and Vulkan ones spell two options.

// VaapiDevice returns the global option opening the VAAPI device both the upload filter and the
// encoder use.
// It fails where the machine exposes no render node with a driver behind it,
// which is the condition every VAAPI codec's probe fails on, and the UI has greyed them out before
// this is reached.
func VaapiDevice() ([]string, error) {
	node := vaapiRenderNode()
	if node == "" {
		return nil, fmt.Errorf("no VAAPI render node under /dev/dri: this machine has no usable VAAPI device")
	}
	return []string{"-vaapi_device", node}, nil
}

// vaapiRenderNode returns the /dev/dri render node to encode on, or "" where none carries a driver.
// A render node is the unprivileged half of a DRM device and the conventional VAAPI target.
// As in drmCaptureDevice, the lowest-numbered node is no safe default: the boot framebuffer can hold
// an early one, so a node with no kernel driver bound, or the simple framebuffer's, is skipped.
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
