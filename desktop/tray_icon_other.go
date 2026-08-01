//go:build !windows

package main

import _ "embed"

// trayIcon is the settings app's own icon, shown in the system tray. It is the
// image the packaged app already carries (desktop/build/appicon.png), which every
// tray host outside Windows decodes.
//
//go:embed build/appicon.png
var trayIcon []byte
