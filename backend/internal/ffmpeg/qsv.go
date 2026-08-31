package ffmpeg

// The QSV half of the publish command: the device the encoder runs on.
// The filter chain handing it frames it can read is the shared one (hwsurface.go).
//
// A QSV device is a oneVPL session over a child device, the API above the same silicon VAAPI
// reaches on Linux and D3D11 on Windows.
// ffmpeg creates that child from the platform it was built for,
// so this side names neither a render node nor an adapter, unlike VaapiDevice.
//
// The session opens only where a oneVPL runtime is installed:
// the dispatcher ffmpeg links loads one by filename off the distro library paths,
// or off ONEVPL_SEARCH_PATH where those carry none (flake.nix).
// A missing runtime and a machine with no Intel GPU give one answer,
// an implementation list with no hardware entry in it.

// qsvDeviceAlias is the name the created device is registered
// under and the name -filter_hw_device refers back to.
// It never leaves the command line.
const qsvDeviceAlias = "qsv"

// qsvImplementation is the oneVPL implementation the session opens on.
// hw_any is whichever hardware one the dispatcher finds, landing the device on the Intel GPU
// of a machine carrying a second card: the runtime enumerates implementations
// and only Intel silicon carries one.
// Naming a DRM render node, the way the VAAPI path does,
// would pick by device order and open the wrong GPU.
const qsvImplementation = "hw_any"

// QsvDevice returns the global options creating the QSV
// device both the upload filter and the encoder use.
//
// Nothing on this side can fail on its own.
// The error is the signature the device table shares with VaapiDevice.
// A machine with no hardware implementation for the dispatcher to return fails when the device
// is opened, which is the condition every QSV codec's probe fails on,
// and the UI has greyed them out before this is reached.
func QsvDevice() ([]string, error) {
	return []string{
		"-init_hw_device", qsvDeviceAlias + "=" + qsvDeviceAlias + ":" + qsvImplementation,
		"-filter_hw_device", qsvDeviceAlias,
	}, nil
}
