package ffmpeg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
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

// drmMapFor returns the strategy row the settings name.
//
// A name no row carries is refused rather than read as auto. The whole point of the
// setting is overriding the driver guess, so resolving a misspelled "vaapi" to auto
// would run the strategy the user chose against, and the frame either arrives mapped
// through another device or fails with EINVAL, neither of which names the setting.
// A settings file from before the option arrives carrying the table's own default
// (settings.migrateStream), so the empty name never reaches here.
//
// The lookup is separate from resolving a device so that it holds against the table
// alone: a machine with no DRM node would otherwise answer a misspelled strategy with
// its own missing hardware.
func drmMapFor(name string) (DrmMap, error) {
	for _, m := range DrmMaps {
		if m.Name == name {
			return m, nil
		}
	}
	return DrmMap{}, fmt.Errorf("DRM download strategy %q is not one of %s", name, strings.Join(drmMapNames(), ", "))
}

// drmMapNames lists the strategy names for the sentence a refusal ends in, so the
// message cannot name a set the table no longer holds.
func drmMapNames() []string {
	out := make([]string, 0, len(DrmMaps))
	for _, m := range DrmMaps {
		out = append(out, m.Name)
	}
	return out
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

// kmsgrabCaptureArgs builds the kmsgrab input arguments, and on the system-memory
// path the DRM download filter chain for the selected strategy.
//
// kmsgrab reads scanout buffers below the compositor and needs CAP_SYS_ADMIN
// (run through the capability wrapper, see FindCaptureExe, or via sudo).
//
// The GPU path takes no strategy. A strategy names the device a tiled scanout buffer
// is mapped through so that hwdownload can read it, and the GPU path downloads
// nothing: the map that follows targets the encoder's own device and is decided by the
// encoder family, not by the user (GpuFilters). The setting is therefore unread rather
// than overridden, which is what the form greys it for.
func kmsgrabCaptureArgs(s settings.Stream, fps, memory string) (captureSource, error) {
	device, err := drmCaptureDevice()
	if err != nil {
		return captureSource{}, err
	}
	input := []string{"-device", device, "-f", "kmsgrab", "-framerate", fps, "-i", "-"}
	if gpupath.OnDevice(memory) {
		return captureSource{args: input}, nil
	}

	// The strategy is checked before the machine is looked at, so a name no row
	// carries is answered with the setting rather than with whatever the sysfs probe
	// finds.
	m, err := drmMapFor(s.DrmMap)
	if err != nil {
		return captureSource{}, err
	}
	dev := m.Device
	if m.Auto {
		dev = autoDrmMap(device)
	}
	var filters []string
	if dev != "" {
		filters = append(filters, "hwmap=derive_device="+dev)
	}
	filters = append(filters, "hwdownload", "format=bgr0")

	return captureSource{args: input, filters: filters}, nil
}

// drmCaptureDevice returns the /dev/dri card node kmsgrab should capture from: the
// first card whose kernel driver is neither absent nor the boot framebuffer.
//
// card0 is not a safe default, which is why no default is taken. On machines with a
// discrete GPU the boot framebuffer often holds card0 under the simple-framebuffer
// driver while the real display controller lands on card1 or higher, so naming card0
// when the probe finds nothing hands kmsgrab a node with no scanout to read and the
// failure reads as a broken capture rather than as a machine with no display
// controller the probe could see.
func drmCaptureDevice() (string, error) {
	// filepath.Glob returns matches in lexical order, so the lowest-numbered
	// real GPU is selected.
	cards, err := filepath.Glob("/dev/dri/card[0-9]*")
	if err != nil {
		return "", fmt.Errorf("cannot list the DRM card nodes under /dev/dri: %w", err)
	}
	for _, dev := range cards {
		switch captureDriver(dev) {
		case "", "simple-framebuffer":
			continue // no driver bound, or the boot framebuffer: skip it
		default:
			return dev, nil
		}
	}
	return "", fmt.Errorf("no DRM card node under /dev/dri carries a display driver: kmsgrab has no scanout to capture")
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
