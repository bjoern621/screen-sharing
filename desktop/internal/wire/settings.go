package wire

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Settings carries a settings.Stream out onto the contract.
//
// The assignments are written in the order both sides declare their fields, because
// the only defect this function can have is a field that quietly never crosses, and a
// list in the schema's own order is one a reader can walk against settings.proto line
// for line. The spellings diverge wherever the two sides named the same value
// differently - BitrateM against bitrate_mbps, MaxrateM against maxrate_mbps, VbvMs
// against vbv_ms - and each of those pairs is a place where the wrong twin reads as
// perfectly plausible code while changing what the user's encoder is told. The
// round-trip test is what actually holds the twins together; the ordering is only what
// makes the mistake visible to a person.
//
// The numbers narrow from int to int32 here and nothing asserts their range on the
// way. Every one of them is a port, a frame rate, a megabit figure or a millisecond
// window, all bounded by what the form offers; a value big enough to lose bits reached
// this struct through a hand-edited settings file, which is an environment condition
// the app survives rather than a broken internal contract worth panicking on.
func Settings(s settings.Stream) *screensharev1.StreamSettings {
	return &screensharev1.StreamSettings{
		Name:       s.Name,
		RelayHost:  s.RelayHost,
		RelayPort:  int32(s.RelayPort),
		ApiPort:    int32(s.ApiPort),
		RtspPort:   int32(s.RtspPort),
		WebrtcPort: int32(s.WebrtcPort),
		RtmpPort:   int32(s.RtmpPort),
		HlsPort:    int32(s.HlsPort),
		MoqPort:    int32(s.MoqPort),

		Transport:   s.Transport,
		Codec:       s.Codec,
		Mode:        s.Mode,
		Chroma:      s.Chroma,
		ColorRange:  s.ColorRange,
		Fps:         int32(s.Fps),
		Cq:          int32(s.Cq),
		BitrateMbps: int32(s.BitrateM),
		MaxrateMbps: int32(s.MaxrateM),
		VbvMs:       int32(s.VbvMs),
		Gop:         int32(s.Gop),
		Bframes:     int32(s.Bframes),
		EncPreset:   s.EncPreset,

		Capture:       s.Capture,
		Audio:         s.Audio,
		AudioCodec:    s.AudioCodec,
		DrmMap:        s.DrmMap,
		Monitor:       int32(s.Monitor),
		CaptureMemory: s.CaptureMemory,

		SrtPublishLatencyMs: int32(s.SrtPublishLatencyMs),
		SrtWatchLatencyMs:   int32(s.SrtWatchLatencyMs),

		RtspPublishProtocol: s.RtspPublishProtocol,
		RtspWatchProtocol:   s.RtspWatchProtocol,

		UplinkMbps: int32(s.UplinkMbps),

		WatchTransport: s.WatchTransport,

		OutputResolution: s.OutputResolution,
	}
}

// StreamSettings reads a draft back off the contract.
//
// Every value is read through a generated GetX accessor rather than off the field,
// which is what makes a nil message convert to the zero settings.Stream instead of
// panicking. That case is not a curiosity worth guarding against for its own sake: a
// request that arrives with no settings set was written by another process, so it is
// an environment condition, and answering it is the caller's job - the control service
// rejects it with INVALID_ARGUMENT. An assert here would instead turn every malformed
// request into a crash of the process holding the encoder, which is the one thing the
// error model forbids (docs/ipc-api.md, "Errors").
//
// GridTransport and RtspWatchLatencyMs are the settings fields the contract does not
// carry, so nothing here can set them and this function alone would clear them. Callers
// that are writing the backend's held settings use StreamSettingsOnto instead, which is
// where that is handled and why.
func StreamSettings(m *screensharev1.StreamSettings) settings.Stream {
	return settings.Stream{
		Name:       m.GetName(),
		RelayHost:  m.GetRelayHost(),
		RelayPort:  int(m.GetRelayPort()),
		ApiPort:    int(m.GetApiPort()),
		RtspPort:   int(m.GetRtspPort()),
		WebrtcPort: int(m.GetWebrtcPort()),
		RtmpPort:   int(m.GetRtmpPort()),
		HlsPort:    int(m.GetHlsPort()),
		MoqPort:    int(m.GetMoqPort()),

		Transport:  m.GetTransport(),
		Codec:      m.GetCodec(),
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
		EncPreset:  m.GetEncPreset(),

		Capture:       m.GetCapture(),
		Audio:         m.GetAudio(),
		AudioCodec:    m.GetAudioCodec(),
		DrmMap:        m.GetDrmMap(),
		Monitor:       int(m.GetMonitor()),
		CaptureMemory: m.GetCaptureMemory(),

		SrtPublishLatencyMs: int(m.GetSrtPublishLatencyMs()),
		SrtWatchLatencyMs:   int(m.GetSrtWatchLatencyMs()),

		RtspPublishProtocol: m.GetRtspPublishProtocol(),
		RtspWatchProtocol:   m.GetRtspWatchProtocol(),

		UplinkMbps: int(m.GetUplinkMbps()),

		WatchTransport:   m.GetWatchTransport(),
		OutputResolution: m.GetOutputResolution(),
	}
}

// StreamSettingsOnto reads a draft off the contract onto the settings the backend
// already holds, and is what every inbound settings path uses.
//
// Every field the contract carries comes from the message; every field it does not
// comes from base. A shell sends the whole settings message rather than a diff, so a
// field the contract dropped would otherwise be cleared by the next save made from a
// screen that never showed it - a shell cannot be asked to preserve a value it has no
// way to see.
//
// There are two such fields, and both are leftovers rather than a pattern: the watch
// leg the obsolete GTK4 grid window received over, and the jitter buffer its receiving
// pipeline sized itself by. Nothing on the contract describes that window, so nothing
// on the contract carries its knobs. When the binary goes, so does this function's
// body; until then the values survive a shell that knows nothing about them.
func StreamSettingsOnto(base settings.Stream, m *screensharev1.StreamSettings) settings.Stream {
	next := StreamSettings(m)
	next.GridTransport = base.GridTransport
	next.RtspWatchLatencyMs = base.RtspWatchLatencyMs
	return next
}

// Preset carries one saved preset across, name and settings whole.
//
// The name is asserted rather than tolerated because it is this process's own. Presets
// reach here from settings.LoadPresets, and every row of that store was written by
// SavePreset under a name the user typed; the wire has no inbound direction for a
// Preset at all, so nothing another process wrote ever arrives at this parameter. A
// nameless row is therefore a store that lost the identity a shell selects and deletes
// by, and here is where that should be found out rather than in the shell drawing a
// blank entry it can never act on again.
func Preset(p settings.Preset) *screensharev1.Preset {
	assert.Assert(p.Name != "", "a stored preset carries the name it is selected by")

	return &screensharev1.Preset{
		Name:     p.Name,
		Settings: Settings(p.Settings),
	}
}

// Presets carries the whole store across in the order it holds them, which is the order
// the user saved them in and the order a shell lists them in.
//
// The result is an empty slice and not nil where nothing is saved. Both encode to the
// same absent repeated field, so this buys nothing on the wire; it is for the Go side,
// where nil is the value a reader most easily reads as "not loaded yet".
func Presets(ps []settings.Preset) []*screensharev1.Preset {
	out := make([]*screensharev1.Preset, 0, len(ps))
	for _, p := range ps {
		out = append(out, Preset(p))
	}

	assert.Assert(len(out) == len(ps), "a message per saved preset", len(out), len(ps))
	return out
}
