package ffmpeg

// The Vulkan Video half of the publish command: the device the encoder runs on.
// The filter chain that hands it frames it can read is the shared one (hwsurface.go).
//
// Vulkan has no counterpart to -vaapi_device, the single option that both creates a device and
// makes it the filter graph's.
// A Vulkan device is created under a name and that name is then handed to the filter graph,
// which is why this side spells out two options where the VAAPI one spells a path.

// vulkanDeviceAlias is the name the created device is registered under, and the name
// -filter_hw_device refers back to.
// It never leaves the command line.
const vulkanDeviceAlias = "vk"

// VulkanDevice returns the global options creating the Vulkan device that both the upload filter
// and the encoder use.
//
// Which GPU that is, is ffmpeg's choice: the Vulkan loader enumerates the physical devices and the
// device is created on one of them, unlike VAAPI, where the render node has to be named.
// A GPU whose driver implements no video-encode queue fails to open the encoder,
// which is the same condition that fails every Vulkan codec's probe, so the UI has already greyed
// them out by the time this could be reached.
// Nothing on this side can fail on its own; the error is the signature the device table shares with
// VaapiDevice.
func VulkanDevice() ([]string, error) {
	return []string{
		"-init_hw_device", "vulkan=" + vulkanDeviceAlias,
		"-filter_hw_device", vulkanDeviceAlias,
	}, nil
}
