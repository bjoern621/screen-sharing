package ffmpeg

// The QSV half of the publish command: the device the encoder runs on.
// The filter chain that hands it frames it can read is the shared one (hwsurface.go).
//
// A QSV device is a oneVPL session over a child device, the API on top of the same silicon VAAPI
// reaches on Linux and D3D11 on Windows.
// ffmpeg creates that child itself from the platform it was built for, so this side names neither a
// render node nor an adapter, unlike VaapiDevice.

// qsvDeviceAlias is the name the created device is registered under, and the name -filter_hw_device
// refers back to.
// It never leaves the command line.
const qsvDeviceAlias = "qsv"

// qsvImplementation is the oneVPL implementation the session opens on.
// hw_any is any hardware one the dispatcher finds, which is what lets the device land on the Intel
// GPU of a machine that has a second card: the runtime enumerates implementations and only Intel
// silicon carries one, where naming a DRM render node the way the VAAPI path does would pick by
// device order and open the wrong GPU.
const qsvImplementation = "hw_any"

// QsvDevice returns the global options creating the QSV device that both the upload filter and the
// encoder use.
//
// A machine with no Intel GPU has no hardware implementation for the dispatcher to return,
// so the device fails to open.
// That is the same condition that fails every QSV codec's probe, and the UI has greyed them out by
// the time this could be reached.
// Nothing on this side can fail on its own; the error is the signature the device table shares with
// VaapiDevice.
func QsvDevice() ([]string, error) {
	return []string{
		"-init_hw_device", qsvDeviceAlias + "=" + qsvDeviceAlias + ":" + qsvImplementation,
		"-filter_hw_device", qsvDeviceAlias,
	}, nil
}
