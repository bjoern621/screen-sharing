package main

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/capabilities"
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
//
// A presets file that could not be read yields the reason alongside the empty
// list. The file has been moved aside by then, so the list is empty because
// nothing readable remains rather than because the presets were deleted, and the
// difference is the user's to see.
func (a *App) GetPresets() ([]settings.Preset, error) {
	presets, err := settings.LoadPresets()
	if presets == nil {
		presets = []settings.Preset{}
	}
	if err != nil {
		logger.Errorf("presets not restored: %v", err)
	}
	return presets, err
}

// SavePreset stores the current settings under name, overwriting a same-named preset.
func (a *App) SavePreset(name string, s settings.Stream) error {
	return settings.SavePreset(name, s)
}

// DeletePreset removes a named preset.
func (a *App) DeletePreset(name string) error {
	return settings.DeletePreset(name)
}

// Transports lists the transports the publish dropdown offers, the
// publisher-to-relay leg: the ones a publish engine here can serialize.
// WatchTransports answers the other leg, which is a wider list wherever the
// relay serves a protocol it does not ingest.
func (a *App) Transports() []string {
	return transport.PublishNames()
}

// TransportCarriage is one transport's format sets on the wire, the binding
// shape of transport.Formats. The Wails generator reaches a struct through a
// return type and not through a map value, so the table crosses as a list.
type TransportCarriage struct {
	Name    string   `json:"name"`
	Publish []string `json:"publish"`
	Watch   []string `json:"watch"`
}

// TransportFormats lists each registered transport with the bitstream formats it
// carries per leg. The settings form greys a codec the selected publish
// transport has no mapping for, and the native-grid verdict reads the watch set
// of the selected watch leg, both off this one table rather than a copy per
// rule.
func (a *App) TransportFormats() []TransportCarriage {
	out := []TransportCarriage{}
	for _, name := range transport.Names() {
		f, ok := transport.FormatsOf(name)
		assert.Assert(ok, "a listed transport is a registered one", name)
		out = append(out, TransportCarriage{Name: name, Publish: f.Publish, Watch: f.Watch})
	}
	return out
}

// WatchTransports lists the transports a stream can be received over: every
// transport with a viewer watch form. It is independent of the publish
// transport, so the Live table offers a Watch control per transport regardless
// of how a stream was published. Which of them can carry a particular stream is
// the narrower question WatchTransportsByFormat answers.
func (a *App) WatchTransports() []string {
	return transport.WatchNames()
}

// GridTransports lists the watch legs the native grid can open on: the
// transports a receiving GStreamer pipeline decodes from. It is not
// WatchTransports: a viewer program needs a URL, and a receiving pipeline
// reaches WHEP, which is not one, while nothing here decodes the relay's HLS
// segments. The two lists therefore each hold a transport the other lacks, and
// the grid button reads this one.
func (a *App) GridTransports() []string {
	return transport.GstWatchNames()
}

// WatchTransportsByFormat maps a bitstream format to the transports a viewer can
// actually receive that format over: those with a viewer watch form, narrowed to
// the ones the relay re-serves that format on.
//
// The relay re-serves an ingested stream on the listeners whose protocol has a
// payload mapping for it, and on no others, so the watch choice is per stream and
// not global. Offering the whole list instead would put SRT in front of an AV1 or
// VP9 stream, which MPEG-TS has no mapping for: the viewer opens, receives
// nothing, and the failure reads as a broken stream rather than an impossible
// combination.
func (a *App) WatchTransportsByFormat() map[string][]string {
	out := map[string][]string{}
	for _, format := range capabilities.Formats() {
		out[format] = transport.WatchNamesFor(format)
	}
	return out
}

// CaptureTransports maps each capture backend to the publish transports its
// engine can carry. The UI disables a transport the selected capture cannot publish and
// repairs a stranded selection from it, so the portal (GStreamer) path never
// offers WebRTC, which has no GStreamer sink.
func (a *App) CaptureTransports() map[string][]string {
	out := map[string][]string{}
	for _, capture := range publish.Captures() {
		ts, err := publish.TransportsFor(capture)
		// Captures lists the capture backends the publish registry holds, so every
		// name here resolves. Skipping one instead would leave it out of the map, and a
		// capture the map does not name is a capture the form imposes no transport
		// restriction for.
		assert.IsNil(err, "a listed capture backend has a publisher", capture)
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
		assert.IsNil(err, "a listed capture backend has a publisher", capture)
		out[capture] = engine
	}
	return out
}
