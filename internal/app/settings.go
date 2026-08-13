package app

import (
	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/wire"
)

func (a *App) GetSettings() settings.Settings {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.settings
}

// SaveSettings takes the settings the form holds and persists them.
//
// A stream that is already publishing keeps running the pipeline it was started on,
// since both engines build a child process from an argv and neither takes a value back afterwards.
// So the publish state is announced from here too: what the form shows and what the viewers are
// watching have just moved apart, and App.Republish is what closes the gap.
//
// The settings themselves are announced for the same reason one leg further out.
// This is the one place the held settings are written, so it is the one place that can say they
// moved, and a shell that did not make the change reads it here rather than by asking again on a
// timer.
// The announcement carries no settings: on the contract's rule a shell re-reads the state the event
// names, which is what keeps the persisted copy and the announced one from being two answers.
func (a *App) SaveSettings(s settings.Settings) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	a.emitPublishState()
	a.emit(wire.SettingsChangedEvent())
	return settings.Save(s)
}

// GetPresets lists the user's saved presets for the UI dropdown.
// A nil slice would cross the Wails boundary as JSON null and break the frontend,
// whose type expects an array, so an empty list is returned instead.
//
// A presets file that could not be read yields the reason alongside the empty list.
// The file has been moved aside by then, so the list is empty because nothing readable remains
// rather than because the presets were deleted, and the difference is the user's to see.
func (a *App) GetPresets() ([]settings.Preset, error) {
	presets, err := settings.LoadPresets()
	if presets == nil {
		presets = []settings.Preset{}
	}
	if err != nil {
		logger.Warnf("presets not restored: %v", err)
	}
	return presets, err
}

// SavePreset stores one way of publishing under name, overwriting a same-named preset.
// A preset is the publish group alone: where the relay is and how this machine watches are not part
// of what a saved configuration means.
func (a *App) SavePreset(name string, p settings.Publish) error {
	return settings.SavePreset(name, p)
}

// DeletePreset removes a named preset.
func (a *App) DeletePreset(name string) error {
	return settings.DeletePreset(name)
}

// TransportCarriage is one transport's carriage on one leg for one engine,
// the binding shape of transport.Formats.
// The Wails generator reaches a struct through a return type and not through a map value,
// so the two maps cross as one list with the leg and the engine as columns.
type TransportCarriage struct {
	Name string `json:"name"`
	// Leg is "publish" or "watch".
	Leg string `json:"leg"`
	// Engine is the publish or watch engine this row belongs to, one of capabilities.Engines.
	Engine string   `json:"engine"`
	Video  []string `json:"video"`
	Audio  []string `json:"audio"`
}

// TransportFormats lists what each registered transport carries, one row per leg and engine.
// The settings form greys a codec the selected publish transport and engine have no mapping for,
// and the native-grid verdict reads the GStreamer watch row of the selected watch leg,
// both off this one table rather than a copy per rule.
//
// A leg an engine cannot serialize contributes no row, which is what keeps the wire shape and the
// registry's own invariant the same statement.
func (a *App) TransportFormats() []TransportCarriage {
	out := []TransportCarriage{}
	for _, name := range transport.Names() {
		f, ok := transport.FormatsOf(name)
		assert.Assert(ok, "a listed transport is a registered one", name)
		for _, engine := range capabilities.Engines {
			if c, ok := f.Publish[engine]; ok {
				out = append(out, TransportCarriage{
					Name: name, Leg: "publish", Engine: engine, Video: c.Video, Audio: c.Audio,
				})
			}
			if c, ok := f.Watch[engine]; ok {
				out = append(out, TransportCarriage{
					Name: name, Leg: "watch", Engine: engine, Video: c.Video, Audio: c.Audio,
				})
			}
		}
	}
	return out
}

// AudioSources lists where the second track can come from, resolved for this machine's platform:
// every source the domain declares, in the order the form presents them, each carrying whether a
// session here serves it, the sentence saying what the machine is missing where it does not,
// and what serves it where it does.
//
// Which sources exist is the platform's answer and not the frontend's.
// The list was typed into util/domain.ts as AUDIO_META's keys and the reasons into util/deps.ts as
// AUDIO_SOURCE_NEEDS, which is one rule written twice in two languages - the drift docs/ipc-api.md
// exists to end.
// This is the binding that takes the second copy away: the frontend keeps the label and the
// paragraph for each value, which is a shell's own work, and reads which values there are and which
// of them this machine serves from here.
//
// It is answered for the platform the app detected rather than for one passed in,
// because there is one machine and the frontend already reads it through Platform:
// a source list resolved for somebody else's operating system is a screen describing a machine the
// user is not sitting at.
func (a *App) AudioSources() []platform.AudioSource {
	return platform.AudioSources(platform.Detect())
}

// AudioCodecs lists the audio codecs the second track can be encoded in, with the engines that code
// each.
// The settings form greys one the selected capture backend's engine cannot encode or the selected
// transport cannot carry, both off this table and the carriage one.
func (a *App) AudioCodecs() []capabilities.AudioCodec {
	return capabilities.AudioCodecs
}

// WatchTransports lists the transports a stream can be received over: every transport with a viewer
// watch form.
// It is independent of the publish transport, so the Live table offers a Watch control per
// transport regardless of how a stream was published.
// Which of them can carry a particular stream is the narrower question WatchTransportsByFormat
// answers.
func (a *App) WatchTransports() []string {
	return transport.WatchNames(capabilities.EngineFfmpeg)
}

// GridTransports lists the watch legs the native grid can open on: the transports a receiving
// GStreamer pipeline decodes from.
// It is not WatchTransports: a viewer program needs a URL, and a receiving pipeline reaches WHEP,
// which is not one, while nothing here decodes the relay's HLS segments.
// The two lists therefore each hold a transport the other lacks, and the grid button reads this
// one.
func (a *App) GridTransports() []string {
	return transport.WatchNames(capabilities.EngineGst)
}

// WatchTransportsByFormat maps a bitstream format to the transports a viewer can actually receive
// that format over: those with a viewer watch form, narrowed to the ones the relay re-serves that
// format on.
//
// The relay re-serves an ingested stream on the listeners whose protocol has a payload mapping for
// it, and on no others, so the watch choice is per stream and not global.
// Offering the whole list instead would put SRT in front of an AV1 or VP9 stream,
// which MPEG-TS has no mapping for: the viewer opens, receives nothing, and the failure reads as a
// broken stream rather than an impossible combination.
func (a *App) WatchTransportsByFormat() map[string][]string {
	out := map[string][]string{}
	for _, format := range capabilities.Formats() {
		out[format] = transport.WatchNamesFor(capabilities.EngineFfmpeg, format)
	}
	return out
}

// CaptureTransports maps each capture backend to the publish transports its engine can carry.
// The UI disables a transport the selected capture cannot publish and repairs a stranded selection
// from it, so the portal (GStreamer) path never offers WebRTC, which has no GStreamer sink.
func (a *App) CaptureTransports() map[string][]string {
	out := map[string][]string{}
	for _, capture := range publish.Captures() {
		ts, err := publish.TransportsFor(capture)
		// Captures lists the capture backends the publish registry holds, so every name here resolves.
		// Skipping one instead would leave it out of the map, and a capture the map does not name is a
		// capture the form imposes no transport restriction for.
		assert.IsNil(err, "a listed capture backend has a publisher", capture)
		out[capture] = ts
	}
	return out
}

// FrameMemories lists the values the frame-memory setting takes, in display order.
func (a *App) FrameMemories() []string {
	return gpupath.Memories
}

// GpuPaths lists the capture backend and encoder family pairs whose frames reach the encoder
// without a trip through system memory.
//
// The form reads the pairs rather than a yes/no per capture backend, because neither end decides on
// its own: the same portal capture has a GPU path into a va encoder and none into an x264 one.
// A selection matching no row leaves only the system-memory value pickable,
// and the row's Import is what the greyed one says instead.
func (a *App) GpuPaths() []gpupath.Path {
	return gpupath.Paths
}

// ForgetPortalConsent drops the stored ScreenCast restore token, so the next portal capture pops
// the compositor's picker again.
// It is how a share aimed at the wrong window or monitor is corrected: the token names one consent,
// and the compositor reuses it until it is asked to choose afresh.
func (a *App) ForgetPortalConsent() error {
	return settings.ForgetPortalToken()
}

// CaptureEngines maps each capture backend to the publish engine that runs it,
// "ffmpeg" or "gstreamer".
// The two engines build their encoder settings through different knobs, so the UI greys a
// rate-control field the selected capture's engine does not forward (the GStreamer nvcodec elements
// take no effort step) instead of letting it look effective.
func (a *App) CaptureEngines() map[string]string {
	out := map[string]string{}
	for _, capture := range publish.Captures() {
		engine, err := publish.EngineFor(capture)
		assert.IsNil(err, "a listed capture backend has a publisher", capture)
		out[capture] = engine
	}
	return out
}
