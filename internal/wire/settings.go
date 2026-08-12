package wire

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Settings carries a settings.Settings out onto the contract, group by group.
//
// The assignments are written in the order both sides declare their fields, because
// the only defect these functions can have is a field that quietly never crosses, and a
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
func Settings(s settings.Settings) *screensharev1.Settings {
	return &screensharev1.Settings{
		Relay:   RelaySettings(s.Relay),
		Publish: PublishSettings(s.Publish),
		Viewer:  ViewerSettings(s.Viewer),
	}
}

// RelaySettings carries the relay's address and listeners out.
func RelaySettings(r settings.Relay) *screensharev1.RelaySettings {
	return &screensharev1.RelaySettings{
		Host:       r.Host,
		SrtPort:    int32(r.SrtPort),
		ApiPort:    int32(r.ApiPort),
		RtspPort:   int32(r.RtspPort),
		WebrtcPort: int32(r.WebrtcPort),
		RtmpPort:   int32(r.RtmpPort),
		HlsPort:    int32(r.HlsPort),
	}
}

// PublishSettings carries one way of publishing out, which is also what a preset is.
func PublishSettings(p settings.Publish) *screensharev1.PublishSettings {
	return &screensharev1.PublishSettings{
		Name: p.Name,

		PublishTransport: p.Transport,
		Codec:            p.Codec,
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
		Audio:         p.Audio,
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

// ViewerSettings carries how this machine watches out.
func ViewerSettings(v settings.Viewer) *screensharev1.ViewerSettings {
	return &screensharev1.ViewerSettings{
		PlayerWatchTransport: v.PlayerWatchTransport,
		TileWatchTransport:   v.TileWatchTransport,

		RtspWatchProtocol:  v.RtspWatchProtocol,
		SrtWatchLatencyMs:  int32(v.SrtWatchLatencyMs),
		RtspWatchLatencyMs: int32(v.RtspWatchLatencyMs),

		RenderChain: v.RenderChain,
	}
}

// ToSettings reads a draft back off the contract.
//
// Every value is read through a generated GetX accessor rather than off the field,
// which is what makes a nil message convert to the zero settings.Settings instead of
// panicking. That case is not a curiosity worth guarding against for its own sake: a
// request that arrives with no settings set was written by another process, so it is
// an environment condition, and answering it is the caller's job - the control service
// rejects it with INVALID_ARGUMENT. An assert here would instead turn every malformed
// request into a crash of the process holding the encoder, which is the one thing the
// error model forbids (docs/ipc-api.md, "Errors").
//
// The contract carries every settings field now, so a draft read off it is whole. That
// was not always so: two knobs of the obsolete GTK4 grid had no message to arrive on,
// and an inbound draft had to be merged onto what the backend held so a shell that
// could not see them did not clear them. Both are ViewerSettings fields now, and the
// merge went with the window.
func ToSettings(m *screensharev1.Settings) settings.Settings {
	return settings.Settings{
		Relay:   ToRelay(m.GetRelay()),
		Publish: ToPublish(m.GetPublish()),
		Viewer:  ToViewer(m.GetViewer()),
	}
}

// ToRelay reads the relay group back off the contract.
func ToRelay(m *screensharev1.RelaySettings) settings.Relay {
	return settings.Relay{
		Host:       m.GetHost(),
		SrtPort:    int(m.GetSrtPort()),
		ApiPort:    int(m.GetApiPort()),
		RtspPort:   int(m.GetRtspPort()),
		WebrtcPort: int(m.GetWebrtcPort()),
		RtmpPort:   int(m.GetRtmpPort()),
		HlsPort:    int(m.GetHlsPort()),
	}
}

// ToPublish reads the publish group back off the contract, which is also how a preset
// arrives.
func ToPublish(m *screensharev1.PublishSettings) settings.Publish {
	return settings.Publish{
		Name: m.GetName(),

		Transport:  m.GetPublishTransport(),
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
		Effort:     m.GetEffort(),
		Tune:       m.GetTune(),

		Capture:       m.GetCapture(),
		Audio:         m.GetAudio(),
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

// ToViewer reads the viewer group back off the contract.
func ToViewer(m *screensharev1.ViewerSettings) settings.Viewer {
	return settings.Viewer{
		PlayerWatchTransport: m.GetPlayerWatchTransport(),
		TileWatchTransport:   m.GetTileWatchTransport(),

		RtspWatchProtocol:  m.GetRtspWatchProtocol(),
		SrtWatchLatencyMs:  int(m.GetSrtWatchLatencyMs()),
		RtspWatchLatencyMs: int(m.GetRtspWatchLatencyMs()),

		RenderChain: m.GetRenderChain(),
	}
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
		Settings: PublishSettings(p.Settings),
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
