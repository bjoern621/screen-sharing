package app

import (
	"fyne.io/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// startTray runs the system tray in the background.
//
// The window closes to a hidden state (main.go HideWindowOnClose), so the tray is
// what brings it back and the one way to actually quit. systray.Run drives its own
// event loop, a StatusNotifierItem over D-Bus on Linux, so it runs on a goroutine
// of its own rather than the one wails.Run holds.
func (a *App) startTray() {
	go systray.Run(a.trayReady, func() {})
}

// trayReady builds the tray once its host is up: an icon that reopens the settings
// window and a quit that exits for real.
//
// "Open settings" reuses showSettings, the same path the native grid's sidebar
// takes to raise this window (system.go). "Quit" goes through the Wails runtime
// so the ordinary shutdown runs and no child process is orphaned (app.go shutdown).
// The click loop runs on its own goroutine so this ready callback returns and the
// tray host sees the menu it built.
func (a *App) trayReady() {
	systray.SetIcon(a.trayIcon)
	systray.SetTitle("screen-sharing")
	systray.SetTooltip("screen-sharing")

	open := systray.AddMenuItem("Open settings", "Bring the settings window to the front")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Stop streaming and exit")

	go func() {
		for {
			select {
			case <-open.ClickedCh:
				a.showSettings()
			case <-quit.ClickedCh:
				runtime.Quit(a.ctx)
				return
			}
		}
	}()
}
