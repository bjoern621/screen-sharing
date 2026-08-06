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

func main() {
	// The backend lives in internal/app; what stays in package main is what go:embed
	// pins here, since it reads no path above its own directory: the frontend bundle
	// above and the tray icon in tray_icon_windows.go / tray_icon_other.go.
	a := app.New(trayIcon)
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
