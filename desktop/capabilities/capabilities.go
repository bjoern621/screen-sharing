// Package capabilities is the single source of truth for the fixed facts about
// each video codec: whether it runs on NVENC, which pixel formats it may encode,
// which transports can carry it on the publish leg, which rate-control modes its
// encoder implements, and the scale its constant-quality knob counts on.
//
// Every transport fact here is publish-side (publisher to relay). What a viewer
// can receive the stream over is a separate question, answered by the watch-side
// helpers in the transport package, since the relay re-serves each ingested
// stream on all its listeners.
//
// These facts are consumed on both sides of the wire. The ffmpeg argument builder
// reads them to branch and to reject an impossible combination, and the frontend
// fetches them (App.Capabilities) to grey out options the user cannot pick. One
// definition keeps the two in agreement: a codec's constraints cannot say one
// thing to the encoder and another to the UI.
//
// Presentation (labels, tooltips) and bitrate heuristics are the frontend's
// concern and are not modeled here.
package capabilities

import "fmt"

// ModeGap is one rate-control mode a codec cannot be driven in, with the reason
// the UI shows in place of the option. Engine names the publish engine the gap
// applies to; empty means every engine, i.e. the format or the library has no
// such mode rather than one builder failing to reach it.
type ModeGap struct {
	// Engine is "ffmpeg", "gstreamer", or empty for both.
	Engine string `json:"engine"`
	// Mode is the rate-control mode: cbr, vbr, abr, crf or lossless.
	Mode string `json:"mode"`
	// Reason states which library or element lacks the mode.
	Reason string `json:"reason"`
}

// Codec describes one video codec's fixed capabilities.
type Codec struct {
	// Name is the ffmpeg encoder name, e.g. "hevc_nvenc".
	Name string `json:"name"`
	// Family groups codecs that share an encoder backend, so the UI can offer the
	// backend and the format as two separate choices. One of: "software",
	// "nvenc", "vaapi", "qsv", "amf", "v4l2", "rkmpp", "vulkan".
	Family string `json:"family"`
	// Format is the video coding format independent of the backend: "h264",
	// "hevc", "av1", "vp9", "vp8". Facts that follow the format rather than the
	// backend (coding efficiency, browser decodability) key off this on the
	// frontend.
	Format string `json:"format"`
	// Nvenc is true for codecs that run on NVIDIA's encoder ASIC.
	Nvenc bool `json:"nvenc"`
	// Implemented is true once the argument builders (encoderArgs, gstEncoder)
	// actually map this codec to a working command. A false entry appears in the
	// UI greyed out so the roadmap is visible, but BuildPublishArgs rejects it.
	Implemented bool `json:"implemented"`
	// Chromas lists the pixel formats this codec may encode.
	Chromas []string `json:"chromas"`
	// CqMax is the highest value this encoder's constant-quality knob accepts, i.e.
	// the scale the crf mode's quantizer target runs on. It follows the encoder and
	// not the format: the H.26x encoders reach 51, libvpx and the software AV1 ones
	// 63, and an encoder whose knob is a raw quantizer index counts to 127 or 255.
	// Zero means the scale is unknown, which is the case for every family the
	// argument builders do not map yet.
	CqMax int `json:"cqMax"`
	// BitrateLimitM is the highest bitrate target the encoder accepts, in Mbit/s,
	// for the modes that aim at one. Zero means the encoder takes any rate the
	// machine can produce, which is the usual case; SVT-AV1 is the exception, and
	// it refuses the whole encode rather than clamping. This is a ceiling on the
	// target, not the VBR burst ceiling the user sets above it.
	BitrateLimitM int `json:"bitrateLimitM"`
	// Transports lists the transport registry keys that can carry this codec on
	// the publish leg. Empty means no registered transport publishes it (e.g. AV1
	// over MPEG-TS). It is not the list a viewer may receive over.
	Transports []string `json:"transports"`
	// ModeGaps lists the rate-control modes this codec cannot be driven in.
	// Empty means all five modes reach the encoder on both publish engines.
	ModeGaps []ModeGap `json:"modeGaps"`
}

// Validate rejects a codec/chroma/transport/mode/quantizer combination this table
// forbids, so a settings object that no frontend normalized cannot reach an
// encoder. transportName is the publish leg, the only one an encoder sees. engine
// is the caller's own publish engine ("ffmpeg" or "gstreamer"), which decides the
// rate-control modes available: both engines call this, so neither path can accept
// what the other rejects, and a mode only one of them reaches is refused for the
// other. The values are taken apart rather than passed as a settings struct to
// keep this package free of dependencies.
func Validate(engine, codec, chroma, transportName, mode string, cq, bitrateM int) error {
	c, ok := Get(codec)
	if !ok {
		return fmt.Errorf("unknown codec %q", codec)
	}
	if !c.Implemented {
		return fmt.Errorf("codec %s is listed but not implemented yet", c.Name)
	}
	if !contains(c.Chromas, chroma) {
		return fmt.Errorf("codec %s cannot encode pixel format %s", c.Name, chroma)
	}
	if !contains(c.Transports, transportName) {
		return fmt.Errorf("transport %s cannot carry codec %s", transportName, c.Name)
	}
	if gap, ok := ModeGapFor(codec, engine, mode); ok {
		return fmt.Errorf("codec %s has no %s mode: %s", c.Name, mode, gap.Reason)
	}
	// The quantizer target reaches the encoder in crf mode only, and each
	// encoder's knob has its own scale: 60 is a valid libvpx CQ and an error on
	// x264.
	if mode == "crf" && c.CqMax > 0 && (cq < 0 || cq > c.CqMax) {
		return fmt.Errorf("quantizer target %d is outside codec %s's 0-%d range", cq, c.Name, c.CqMax)
	}
	// The bitrate target reaches the encoder in the three bitrate modes only, so a
	// value left over from a lossless preset must not block a constant-quality
	// encode that never sends it.
	if targetsBitrate(mode) && c.BitrateLimitM > 0 && bitrateM > c.BitrateLimitM {
		return fmt.Errorf("bitrate target %d Mbit/s is above codec %s's %d Mbit/s ceiling", bitrateM, c.Name, c.BitrateLimitM)
	}
	return nil
}

// targetsBitrate reports whether a rate-control mode aims at a bitrate the user
// sets. Constant quality and lossless spend whatever the picture costs, so the
// bitrate field means nothing to them.
func targetsBitrate(mode string) bool {
	return mode == "cbr" || mode == "vbr" || mode == "abr"
}

// Get returns the capabilities for name, or false if the codec is unknown.
func Get(name string) (Codec, bool) {
	for _, c := range Codecs {
		if c.Name == name {
			return c, true
		}
	}
	return Codec{}, false
}

// ModeGapFor returns the gap that keeps codec name out of the given rate-control
// mode on the named publish engine, and false when the mode reaches the encoder.
// An unknown codec has no gaps: Validate rejects it on the name instead.
func ModeGapFor(name, engine, mode string) (ModeGap, bool) {
	c, ok := Get(name)
	if !ok {
		return ModeGap{}, false
	}
	for _, g := range c.ModeGaps {
		if g.Mode == mode && (g.Engine == "" || g.Engine == engine) {
			return g, true
		}
	}
	return ModeGap{}, false
}

// IsNvenc reports whether name is an NVENC codec. Unknown codecs are not NVENC.
func IsNvenc(name string) bool {
	c, ok := Get(name)
	return ok && c.Nvenc
}

// FamilyVaapi is the encoder family whose codecs encode from VAAPI surfaces rather
// than from system memory, which both publish engines build differently: the ffmpeg
// command opens a device and uploads each frame, and the GStreamer pipeline pins the
// va plugin's own raw formats.
const FamilyVaapi = "vaapi"

// IsVaapi reports whether name is a VAAPI codec. Unknown codecs are not VAAPI.
func IsVaapi(name string) bool {
	c, ok := Get(name)
	return ok && c.Family == FamilyVaapi
}

// SupportsChroma reports whether codec name may encode the given pixel format.
func SupportsChroma(name, chroma string) bool {
	c, ok := Get(name)
	if !ok {
		return false
	}
	return contains(c.Chromas, chroma)
}

// CarriedBy reports whether transport can carry codec name on the publish leg.
func CarriedBy(name, transport string) bool {
	c, ok := Get(name)
	if !ok {
		return false
	}
	return contains(c.Transports, transport)
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
