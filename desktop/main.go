package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"bjoernblessin.de/screenshare/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is this build's stamp, which the control handshake answers with so a shell
// can name the backend it is talking to (docs/ipc-api.md, "Versioning").
//
// It lives here because this is what the linker writes into: a release build sets it
// with -ldflags "-X main.version=...". A build nobody stamped says so rather than
// claiming a number, since "dev" is a truthful answer to which build this is and an
// invented version is not.
var version = "dev"

func main() {
	// The backend lives in internal/app; what stays in package main is what go:embed
	// pins here, since it reads no path above its own directory - the frontend bundle
	// above and the tray icon in tray_icon_windows.go / tray_icon_other.go - and the
	// build stamp above, which is the linker's to write.
	a := app.New(trayIcon, version)
	startup, shutdown := app.Hooks(a)

	err := wails.Run(&options.App{
		Title:  "screen-sharing",
		Width:  900,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 9, G: 9, B: 11, A: 1},
		// Closing the window hides it instead of quitting, so the native grid the
		// app spawned keeps running; the tray icon brings the settings back.
		HideWindowOnClose: true,
		OnStartup:         startup,
		OnShutdown:        shutdown,
		Bind: []interface{}{
			a,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
