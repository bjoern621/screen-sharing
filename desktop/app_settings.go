package main

import (
	"bjoernblessin.de/screenshare/publish"
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

// Transports lists the registered transports for the publish dropdown, the
// publisher-to-relay leg. WatchTransports answers the other leg.
func (a *App) Transports() []string {
	return transport.Names()
}

// WatchTransports lists the transports a stream can be received over: every
// transport with a viewer watch form. It is independent of the publish
// transport, so the Live table offers a Watch control per transport regardless
// of how a stream was published.
func (a *App) WatchTransports() []string {
	return transport.WatchNames()
}

// CaptureTransports maps each capture backend to the publish transports its
// engine can carry. The UI disables a transport the selected capture cannot publish and
// repairs a stranded selection from it, so the portal (GStreamer) path never
// offers WebRTC, which has no GStreamer sink.
func (a *App) CaptureTransports() map[string][]string {
	out := map[string][]string{}
	for _, capture := range publish.Captures() {
		ts, err := publish.TransportsFor(capture)
		if err != nil {
			continue
		}
		out[capture] = ts
	}
	return out
}

// CaptureEngines maps each capture backend to the publish engine that runs it,
// "ffmpeg" or "gstreamer". The two engines build their encoder settings through
// different knobs, so the UI greys a rate-control field the selected capture's
// engine does not forward (the GStreamer encoders take no NVENC preset ladder)
// instead of letting it look effective.
func (a *App) CaptureEngines() map[string]string {
	out := map[string]string{}
	for _, capture := range publish.Captures() {
		engine, err := publish.EngineFor(capture)
		if err != nil {
			continue
		}
		out[capture] = engine
	}
	return out
}
