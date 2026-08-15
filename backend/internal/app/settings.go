package app

import (
	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
	"bjoernblessin.de/screenshare/internal/wire"
)

func (a *App) GetSettings() settings.Settings {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.settings
}

// SaveSettings persists the settings the form holds.
//
// A stream already publishing keeps the pipeline it was started on: both engines build a child
// process from an argv and neither takes a value back afterwards.
// So the publish state is announced from here too, because what the form shows and what the viewers
// are watching have just moved apart, and App.Republish is what closes the gap.
//
// A shell that did not make the change reads the move here rather than by asking again on a timer.
// StartPublish and Republish write the held settings too, each taking the draft the form handed it,
// and each announces the same way: a write that moved them and said nothing leaves the other shell
// with a stale draft to overwrite this one with.
// The announcement carries no settings, since a shell re-reads the state an event names, which is
// what keeps the persisted copy and the announced one from being two answers.
func (a *App) SaveSettings(s settings.Settings) error {
	a.settingsMu.Lock()
	a.settings = s
	a.settingsMu.Unlock()

	a.emitPublishState()
	a.emit(wire.SettingsChangedEvent())
	return settings.Save(s)
}

// SavePreset stores one way of publishing under name, overwriting a same-named preset.
// A preset is the publish group alone: where the relay is and how this machine watches are not part
// of what a saved configuration means.
func (a *App) SavePreset(name string, p settings.Publish) error {
	return settings.SavePreset(name, p)
}

func (a *App) DeletePreset(name string) error {
	return settings.DeletePreset(name)
}

// AudioSources is where the second track can come from, resolved for this machine's platform:
// every source the domain declares, in the order the form presents them, each carrying whether a
// session here serves it, what serves it where it does, and what the machine is missing where it
// does not.
//
// Which sources exist is the platform's answer and not a shell's: a shell keeps the label and the
// paragraph for each value, and reads the values themselves from here (docs/ipc-api.md).
//
// Answered for the platform the app detected rather than for one passed in: there is one machine,
// and a source list resolved for somebody else's operating system describes a machine the user is
// not sitting at.
func (a *App) AudioSources() []platform.AudioSource {
	return platform.AudioSources(platform.Detect())
}

// AudioCodecs is the audio codecs the second track can be encoded in, with the engines that code
// each (docs/domain-model.md).
// The form greys one the selected capture backend's engine cannot encode or the selected transport
// cannot carry, off this table and the carriage one.
func (a *App) AudioCodecs() []capabilities.AudioCodec {
	return capabilities.AudioCodecs
}

// WatchTransports is the legs a viewer program can be pointed at: every transport with an ffmpeg
// watch form.
// It is independent of the publish transport, so a stream can be offered on every leg regardless of
// how it was published.
// Which of them carry a particular stream is the narrower question WatchTransportsByFormat answers.
func (a *App) WatchTransports() []string {
	return transport.WatchNames(capabilities.EngineFfmpeg)
}

// WatchTransportsByFormat maps a bitstream format to the legs a viewer can receive that format over:
// those with an ffmpeg watch form, narrowed to the ones the relay re-serves that format on.
//
// The relay re-serves an ingested stream on the listeners whose protocol has a payload mapping for
// it and on no others, so the watch choice is per stream and not global.
// Offering the whole list instead would put SRT in front of an AV1 or VP9 stream, which MPEG-TS has
// no mapping for: the viewer opens, receives nothing, and the failure reads as a broken stream
// rather than as an impossible combination.
func (a *App) WatchTransportsByFormat() map[string][]string {
	out := map[string][]string{}
	for _, format := range capabilities.Formats() {
		out[format] = transport.WatchNamesFor(capabilities.EngineFfmpeg, format)
	}
	return out
}

// FrameMemories is the frame-memory setting's values, in display order.
func (a *App) FrameMemories() []string {
	return gpupath.Memories
}

// GpuPaths is the capture backend and encoder family pairs whose frames reach the encoder without a
// trip through system memory.
//
// Pairs rather than a yes/no per capture backend, because neither end decides on its own: the same
// portal capture has a GPU path into a va encoder and none into an x264 one.
// A selection matching no row leaves only the system-memory value pickable, and the row's Import is
// what the greyed one names instead.
func (a *App) GpuPaths() []gpupath.Path {
	return gpupath.Paths
}

// ForgetPortalConsent drops the stored ScreenCast restore token, so the next portal capture pops the
// compositor's picker again.
// It is how a share aimed at the wrong window or monitor is corrected: the token names one consent,
// and the compositor reuses it until it is asked to choose afresh.
func (a *App) ForgetPortalConsent() error {
	return settings.ForgetPortalToken()
}
