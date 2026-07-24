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

// GetPresets lists the user's saved presets for the UI dropdown.
// A nil slice would cross the Wails boundary as JSON null and break the
// frontend, whose type expects an array, so an empty list is returned instead.
func (a *App) GetPresets() []settings.Preset {
	presets := settings.LoadPresets()
	if presets == nil {
		return []settings.Preset{}
	}
	return presets
}

// SavePreset stores the current settings under name, overwriting a same-named preset.
func (a *App) SavePreset(name string, s settings.Stream) error {
	return settings.SavePreset(name, s)
}

// DeletePreset removes a named preset.
func (a *App) DeletePreset(name string) error {
	return settings.DeletePreset(name)
}

// Transports lists the registered transports for the UI dropdown.
func (a *App) Transports() []string {
	return transport.Names()
}
