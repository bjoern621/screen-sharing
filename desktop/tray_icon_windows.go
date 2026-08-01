package main

import _ "embed"

// trayIcon is the settings app's own icon, shown in the system tray. Windows loads a
// tray icon with LoadImageW, which decodes .ico and nothing else, so this build
// embeds the icon the Windows packaging already carries rather than the .png the
// other platforms use. Handing it a .png costs the whole tray, not just the picture:
// SetIcon fails, and the tray is what reopens the window and the one way to quit.
//
//go:embed build/windows/icon.ico
var trayIcon []byte
