package ffmpeg

import (
	"fmt"
	"path/filepath"
)

// The VAAPI half of the publish command: the device the encoder runs on.
// The filter chain that hands it frames it can read is the shared one (hwsurface.go).

// VaapiDevice returns the global option opening the VAAPI device that both the upload filter and
// the encoder use.
// It fails when the machine exposes no render node, which is the same condition that makes every
// VAAPI codec fail its probe, so the UI has already greyed them out by the time this could be
// reached.
func VaapiDevice() ([]string, error) {
	node := vaapiRenderNode()
	if node == "" {
		return nil, fmt.Errorf("no VAAPI render node under /dev/dri: this machine has no usable VAAPI device")
	}
	return []string{"-vaapi_device", node}, nil
}

// vaapiRenderNode returns the /dev/dri render node to encode on, or "" when none carries a real
// driver.
// A render node is the unprivileged half of a DRM device and the conventional VAAPI target.
// As in drmCaptureDevice, the lowest-numbered node is not a safe default on its own:
// the boot framebuffer can hold an early one, so a node whose kernel driver is missing or is the
// simple framebuffer is skipped.
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
