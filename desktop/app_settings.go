package main

import (
	"bjoernblessin.de/screenshare/settings"
	"bjoernblessin.de/screenshare/transport"
)

func (a *App) GetSettings() settings.Stream {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.settings
}

func (a *App) SaveSettings(s settings.Stream) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	return settings.Save(s)
}

// Transports lists the registered transports for the UI dropdown.
func (a *App) Transports() []string {
	return transport.Names()
}
