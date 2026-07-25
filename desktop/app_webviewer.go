package main

import (
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/webviewer"
)

// startWebViewer launches the in-process viewer service that re-serves relay
// streams to the browser over WebSocket, carrying the VP9 4:4:4 modes WebRTC
// cannot negotiate. The relay location is read from current settings on each
// connection, so a host or port change takes effect on the next viewer without
// restarting the service. A bind failure is logged, not fatal: the rest of the
// app runs without the browser viewer path.
func (a *App) startWebViewer() {
	a.webviewer = webviewer.New(webviewer.Config{
		Port: webviewer.DefaultPort,
		Relay: func() (string, int) {
			a.settingsMu.Lock()
			defer a.settingsMu.Unlock()
			return a.settings.RelayHost, a.settings.RtspPort
		},
	})
	if err := a.webviewer.Start(); err != nil {
		logger.Warnf("viewer service did not start: %v", err)
	}
}
