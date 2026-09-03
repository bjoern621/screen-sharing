package wire

import (
	"bjoernblessin.de/go-utils/util/assert"

	"google.golang.org/protobuf/proto"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Settings carries a settings.Settings out onto the contract, group by group.
//
// The assignments follow the order both sides declare their fields in,
// so a reader can walk them against settings.proto line for line.
// The only defect these functions can have is a field that never crosses, or one written
// into its twin: BitrateM against bitrate_mbps, MaxrateM against maxrate_mbps, VbvMs against
// vbv_ms.
// The wrong half of a pair reads as plausible code and changes what the user's encoder is told.
// The round-trip test holds the twins together,
// the ordering only making the mistake visible to a person.
//
// The numbers narrow from int to int32 and nothing asserts their range on the way.
// Each is a port, a frame rate, a megabit figure or a millisecond window, all bounded by the form,
// so a value big enough to lose bits arrived through a hand-edited settings file:
// an Umgebungsfehler the app survives rather than a broken internal contract worth panicking on.
func Settings(s settings.Settings) *screensharev1.Settings {
	return &screensharev1.Settings{
		Relay:      RelaySettings(s.Relay),
		Publish:    PublishSettings(s.Publish),
		Viewer:     ViewerSettings(s.Viewer),
		StreamName: s.StreamName(),
	}
}

func RelaySettings(r settings.Relay) *screensharev1.RelaySettings {
	return &screensharev1.RelaySettings{
		Host:        r.Host,
		SrtPort:     int32(r.SrtPort),
		RtspPort:    int32(r.RtspPort),
		WebrtcPort:  int32(r.WebrtcPort),
		RtmpPort:    int32(r.RtmpPort),
		HlsPort:     int32(r.HlsPort),
		MoqPort:     int32(r.MoqPort),
		Tls:         r.Tls(),
		GroupKey:    r.GroupKey,
		DisplayName: r.DisplayName,
	}
}

// PublishSettings carries one way of publishing out, the whole of what a preset holds.
func PublishSettings(p settings.Publish) *screensharev1.PublishSettings {
	return &screensharev1.PublishSettings{
		PublishTransport: p.Transport,
		Format:           p.Format,
		Encoder:          p.Encoder,
		Mode:             p.Mode,
		Chroma:           p.Chroma,
		ColorRange:       p.ColorRange,
		Fps:              int32(p.Fps),
		Cq:               int32(p.Cq),
		BitrateMbps:      int32(p.BitrateM),
		MaxrateMbps:      int32(p.MaxrateM),
		VbvMs:            int32(p.VbvMs),
		Gop:              int32(p.Gop),
		Bframes:          int32(p.Bframes),
		Effort:           p.Effort,
		Tune:             p.Tune,

		Capture:       p.Capture,
		AudioSources:  audioSources(p.AudioSources),
		AudioCodec:    p.AudioCodec,
		DrmMap:        p.DrmMap,
		Monitor:       int32(p.Monitor),
		CaptureMemory: p.CaptureMemory,
		Cursor:        p.Cursor,

		SrtPublishLatencyMs: int32(p.SrtPublishLatencyMs),
		RtspPublishProtocol: p.RtspPublishProtocol,

		UplinkMbps: int32(p.UplinkMbps),

		OutputResolution: p.OutputResolution,
	}
}

func ViewerSettings(v settings.Viewer) *screensharev1.ViewerSettings {
	return &screensharev1.ViewerSettings{
		TileWatchTransport: v.TileWatchTransport,

		RtspWatchProtocol:  v.RtspWatchProtocol,
		SrtWatchLatencyMs:  int32(v.SrtWatchLatencyMs),
		RtspWatchLatencyMs: int32(v.RtspWatchLatencyMs),

		RenderChain:  v.RenderChain,
		PreviewRoute: v.PreviewRoute,
	}
}

// ToSettings reads a draft back off the contract.
//
// Every value goes through a generated GetX accessor rather than off the field,
// so a nil message reads as the zero settings.Settings instead of panicking.
// A request arriving with no settings set was written by another process,
// an Umgebungsfehler the caller answers: the control service rejects it with INVALID_ARGUMENT.
// An assert here would turn every malformed request into a crash of the process holding
// the encoder, which the error model forbids (docs/ipc-api.md, "Errors").
//
// The contract carries every settings field,
// so a draft read off it is whole and nothing is merged onto what the backend already held.
func ToSettings(m *screensharev1.Settings) settings.Settings {
	return settings.Settings{
		Relay:   ToRelay(m.GetRelay()),
		Publish: ToPublish(m.GetPublish()),
		Viewer:  ToViewer(m.GetViewer()),
	}
}

// ToRelay reads the relay group back off the contract.
//
// RelaySettings.tls is not read.
// Encryption is derived from the address on this side (settings.Relay.Tls),
// so the field crosses outward as a reading and a value coming back is a shell's copy of an answer
// this side already holds.
// Taking it would let a stale draft turn encryption off.
func ToRelay(m *screensharev1.RelaySettings) settings.Relay {
	return settings.Relay{
		Host:        m.GetHost(),
		SrtPort:     int(m.GetSrtPort()),
		RtspPort:    int(m.GetRtspPort()),
		WebrtcPort:  int(m.GetWebrtcPort()),
		RtmpPort:    int(m.GetRtmpPort()),
		HlsPort:     int(m.GetHlsPort()),
		MoqPort:     int(m.GetMoqPort()),
		GroupKey:    m.GetGroupKey(),
		DisplayName: m.GetDisplayName(),
	}
}

// ToPublish reads the publish group back off the contract, and a preset arrives the same way.
func ToPublish(m *screensharev1.PublishSettings) settings.Publish {
	return settings.Publish{
		Transport:  m.GetPublishTransport(),
		Format:     m.GetFormat(),
		Encoder:    m.GetEncoder(),
		Mode:       m.GetMode(),
		Chroma:     m.GetChroma(),
		ColorRange: m.GetColorRange(),
		Fps:        int(m.GetFps()),
		Cq:         int(m.GetCq()),
		BitrateM:   int(m.GetBitrateMbps()),
		MaxrateM:   int(m.GetMaxrateMbps()),
		VbvMs:      int(m.GetVbvMs()),
		Gop:        int(m.GetGop()),
		Bframes:    int(m.GetBframes()),
		Effort:     m.GetEffort(),
		Tune:       m.GetTune(),

		Capture:       m.GetCapture(),
		AudioSources:  toAudioSources(m.GetAudioSources()),
		AudioCodec:    m.GetAudioCodec(),
		DrmMap:        m.GetDrmMap(),
		Monitor:       int(m.GetMonitor()),
		CaptureMemory: m.GetCaptureMemory(),
		Cursor:        m.GetCursor(),

		SrtPublishLatencyMs: int(m.GetSrtPublishLatencyMs()),
		RtspPublishProtocol: m.GetRtspPublishProtocol(),

		UplinkMbps: int(m.GetUplinkMbps()),

		OutputResolution: m.GetOutputResolution(),
	}
}

func ToViewer(m *screensharev1.ViewerSettings) settings.Viewer {
	return settings.Viewer{
		TileWatchTransport: m.GetTileWatchTransport(),

		RtspWatchProtocol:  m.GetRtspWatchProtocol(),
		SrtWatchLatencyMs:  int(m.GetSrtWatchLatencyMs()),
		RtspWatchLatencyMs: int(m.GetRtspWatchLatencyMs()),

		RenderChain:  m.GetRenderChain(),
		PreviewRoute: m.GetPreviewRoute(),
	}
}

// Preset carries one saved preset across, name and settings whole.
//
// The name is asserted rather than tolerated, being this process's own:
// presets reach here from settings.LoadPresets, every row of that store written by SavePreset under
// a name the user typed, and a Preset has no inbound direction on the wire,
// so nothing another process wrote arrives at this parameter.
// A nameless row is a store that lost the identity a shell selects and deletes by,
// found out here rather than in a shell drawing a blank entry it can never act on again.
func Preset(p settings.Preset) *screensharev1.Preset {
	assert.Assert(p.Name != "", "a stored preset carries the name it is selected by")

	return &screensharev1.Preset{
		Name:     p.Name,
		Settings: PublishSettings(p.Settings),
	}
}

// Presets carries the whole store across in the order it holds them,
// the order the user saved them in and the order a shell lists them in.
//
// Nothing saved is an empty slice and not nil.
// Both encode to the same absent repeated field, so this buys nothing on the wire:
// it is for the Go side, where a reader most easily takes nil for "not loaded yet".
func Presets(ps []settings.Preset) []*screensharev1.Preset {
	out := make([]*screensharev1.Preset, 0, len(ps))
	for _, p := range ps {
		out = append(out, Preset(p))
	}

	assert.Assert(len(out) == len(ps), "a message per saved preset", len(out), len(ps))
	return out
}

// audioSources carries the list the second track is mixed from onto the contract,
// and toAudioSources reads it back.
// Both keep the order, the one a form draws the entries in and the one the indexed keys
// ("publish.audio_sources[2].gain") address them by.
//
// An empty list and an absent field are one statement here: a stream with no second track.
func audioSources(sources []settings.AudioSource) []*screensharev1.AudioSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]*screensharev1.AudioSource, 0, len(sources))
	for _, a := range sources {
		out = append(out, &screensharev1.AudioSource{
			Source: a.Source,
			Device: a.Device,
			Gain:   proto.Int32(int32(a.Gain)),
			Mute:   a.Mute,
		})
	}

	assert.Assert(len(out) == len(sources), "an entry per source the list holds", len(out), len(sources))
	return out
}

func toAudioSources(sources []*screensharev1.AudioSource) []settings.AudioSource {
	if len(sources) == 0 {
		// No list rather than an empty one:
		// that is what a settings object with no second track holds and what a repair leaves untouched.
		// A draft coming back with an empty slice where it sent none is one the repair changed without
		// naming a field.
		return nil
	}
	out := make([]settings.AudioSource, 0, len(sources))
	for _, a := range sources {
		out = append(out, settings.AudioSource{
			Source: a.GetSource(),
			Device: a.GetDevice(),
			Gain:   audioGain(a),
			Mute:   a.GetMute(),
		})
	}

	assert.Assert(len(out) == len(sources), "an entry per source the message carries", len(out), len(sources))
	return out
}

// audioGain is the level one entry contributes, and unity for an entry nobody has set one on.
//
// Zero is a level and not an absence, a source turned all the way down being silent,
// so the field carries presence and this reads it.
// An entry a reader creates by picking a kind on the growing row arrives with no gain at all,
// and arriving silent is the one answer it must not have.
func audioGain(a *screensharev1.AudioSource) int {
	if a.Gain == nil {
		return settings.GainUnity
	}
	return int(a.GetGain())
}
