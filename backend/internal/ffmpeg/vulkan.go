package ffmpeg

// The Vulkan Video half of the publish command: the device the encoder runs on.
// The filter chain uploading system frames into the Vulkan images the encode operation takes
// is the shared one (hwsurface.go).
//
// Vulkan has no counterpart to -vaapi_device, the one option that both creates a device
// and makes it the filter graph's.
// A Vulkan device is created under a name and that name is handed to the filter graph,
// hence two options where the VAAPI side spells a path.

// vulkanDeviceAlias is the name the created device is registered under and the name
// -filter_hw_device refers back to.
// It never leaves the command line.
const vulkanDeviceAlias = "vk"

// VulkanDevice returns the global options creating the Vulkan
// device both the upload filter and the encoder use.
//
// Which GPU it lands on is ffmpeg's choice: the loader enumerates the physical devices
// and the device is created on one of them, where VAAPI has its render node named.
//
// Nothing on this side can fail on its own.
// The error is the signature the device table shares with VaapiDevice.
// A driver implementing no video-encode queue fails when the encoder is opened,
// which is the condition every Vulkan codec's probe fails on,
// and the UI has greyed them out before this is reached.
func VulkanDevice() ([]string, error) {
	return []string{
		"-init_hw_device", "vulkan=" + vulkanDeviceAlias,
		"-filter_hw_device", vulkanDeviceAlias,
	}, nil
}
