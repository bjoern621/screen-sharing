package ffmpeg

import (
	"os"
	"path/filepath"

	"bjoernblessin.de/screenshare/settings"
)

// DrmMap is one strategy for landing a kmsgrab DRM_PRIME scanout frame in a
// linear system-memory buffer the encoder can read.
//
// A scanout framebuffer usually carries a GPU tiling or compression modifier, so
// hwdownload cannot map it directly and fails with EINVAL. Mapping the frame
// through a hwdevice that understands the modifier first solves this, but which
// device works depends on the GPU. The strategy is therefore selectable, and the
// table is the single source of truth for what the ffmpeg vf chain becomes.
type DrmMap struct {
	// Name is the settings value and UI key.
	Name string `json:"name"`
	// Device is the hwmap derive_device that understands the modifier. An empty
	// Device downloads the frame directly, correct only for a linear framebuffer.
	Device string `json:"device"`
	// Auto resolves Device from the capture device's kernel driver at runtime
	// instead of using the fixed Device above.
	Auto bool `json:"auto"`
}

// DrmMaps is the DRM-download strategy table. Order is the UI display order.
// A GPU whose driver auto-detection misjudges can be forced onto a concrete
// entry.
var DrmMaps = []DrmMap{
	{Name: "auto", Auto: true},
	{Name: "vaapi", Device: "vaapi"},
	{Name: "vulkan", Device: "vulkan"},
	{Name: "none", Device: ""},
}

// resolveDrmMap returns the hwmap derive_device for the selected strategy, or ""
// to download the frame directly. An unknown or empty name (a settings file from
// before this option existed) resolves as auto.
func resolveDrmMap(name, device string) string {
	for _, m := range DrmMaps {
		if m.Name == name && !m.Auto {
			return m.Device
		}
	}
	return autoDrmMap(device)
}

// autoDrmMap picks a derive_device from the capture device's kernel driver.
// Intel and AMD expose the scanout modifier through VAAPI; everything else,
// including NVIDIA and unknown drivers, goes through Vulkan, the cross-vendor
// DRM interop path.
func autoDrmMap(device string) string {
	switch captureDriver(device) {
	case "i915", "xe", "amdgpu", "radeon":
		return "vaapi"
	default:
		return "vulkan"
	}
}

// kmsgrabCaptureArgs builds the kmsgrab input arguments, including the DRM
// download filter chain for the selected strategy.
//
// kmsgrab reads scanout buffers below the compositor and needs CAP_SYS_ADMIN
// (run through the capability wrapper, see FindCaptureExe, or via sudo).
func kmsgrabCaptureArgs(s settings.Stream, fps string) captureSource {
	device := drmCaptureDevice()

	var filters []string
	if dev := resolveDrmMap(s.DrmMap, device); dev != "" {
		filters = append(filters, "hwmap=derive_device="+dev)
	}
	filters = append(filters, "hwdownload", "format=bgr0")

	return captureSource{
		args:    []string{"-device", device, "-f", "kmsgrab", "-framerate", fps, "-i", "-"},
		filters: filters,
	}
}

// drmCaptureDevice returns the /dev/dri card node kmsgrab should capture from.
// card0 is not a safe default: on machines with a discrete GPU the boot
// framebuffer often holds card0 under the simple-framebuffer driver while the
// real display controller lands on card1 or higher. The first card whose kernel
// driver is not simple-framebuffer wins; /dev/dri/card0 is the fallback when the
// sysfs probe finds nothing (no DRI nodes, or none with a readable driver).
func drmCaptureDevice() string {
	const fallback = "/dev/dri/card0"

	// filepath.Glob returns matches in lexical order, so the lowest-numbered
	// real GPU is selected.
	cards, err := filepath.Glob("/dev/dri/card[0-9]*")
	if err != nil {
		return fallback
	}
	for _, dev := range cards {
		switch captureDriver(dev) {
		case "", "simple-framebuffer":
			continue // no driver bound, or the boot framebuffer: skip it
		default:
			return dev
		}
	}
	return fallback
}

// captureDriver returns the base name of the kernel driver bound to a /dev/dri
// card node (e.g. i915, amdgpu, nvidia, simple-framebuffer), or "" when no
// driver is readable. The driver symlink under sysfs resolves to the bound
// module.
func captureDriver(device string) string {
	link, err := os.Readlink(filepath.Join("/sys/class/drm", filepath.Base(device), "device", "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}
