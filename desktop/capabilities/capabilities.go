// Package capabilities is the single source of truth for the fixed facts about
// each video codec: whether it runs on NVENC, which pixel formats it may encode,
// which transports can carry it on the publish leg, and the scale its
// constant-quality knob counts on.
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
	// CqMax is the highest value this encoder's constant-quality knob accepts,
	// i.e. the scale the crf mode's quantizer target runs on: libvpx VP9 counts
	// to 63, the H.26x and AV1 encoders to 51. Zero means the scale is unknown,
	// which is the case for every family the argument builders do not map yet.
	CqMax int `json:"cqMax"`
	// Transports lists the transport registry keys that can carry this codec on
	// the publish leg. Empty means no registered transport publishes it (e.g. AV1
	// over MPEG-TS). It is not the list a viewer may receive over.
	Transports []string `json:"transports"`
}

// Validate rejects a codec/chroma/transport/quantizer combination this table
// forbids, so a settings object that no frontend normalized cannot reach an
// encoder. transportName is the publish leg, the only one an encoder sees. Both publish engines call it, the ffmpeg argument builder and the
// GStreamer pipeline builder, so neither path can accept what the other rejects.
// The values are taken apart rather than passed as a settings struct to keep this
// package free of dependencies.
func Validate(codec, chroma, transportName, mode string, cq int) error {
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
	// The quantizer target reaches the encoder in crf mode only, and each
	// encoder's knob has its own scale: 60 is a valid libvpx CQ and an error on
	// x264.
	if mode == "crf" && c.CqMax > 0 && (cq < 0 || cq > c.CqMax) {
		return fmt.Errorf("quantizer target %d is outside codec %s's 0-%d range", cq, c.Name, c.CqMax)
	}
	return nil
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

// IsNvenc reports whether name is an NVENC codec. Unknown codecs are not NVENC.
func IsNvenc(name string) bool {
	c, ok := Get(name)
	return ok && c.Nvenc
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
