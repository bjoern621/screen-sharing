// Package capabilities is the single source of truth for the fixed facts about
// each video codec: whether it runs on NVENC, which pixel formats it may encode,
// and which transports can carry it.
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

// Codec describes one video codec's fixed capabilities.
type Codec struct {
	// Name is the ffmpeg encoder name, e.g. "hevc_nvenc".
	Name string `json:"name"`
	// Nvenc is true for codecs that run on NVIDIA's encoder ASIC.
	Nvenc bool `json:"nvenc"`
	// Chromas lists the pixel formats this codec may encode.
	Chromas []string `json:"chromas"`
	// Transports lists the transport registry keys that can carry this codec.
	// Empty means no registered transport carries it (e.g. AV1 over MPEG-TS).
	Transports []string `json:"transports"`
}

// Codecs is the capability table. Order is the UI display order.
var Codecs = []Codec{
	{
		// No webrtc: ffmpeg's WHIP muxer carries H.264 and Opus only.
		Name:       "hevc_nvenc",
		Nvenc:      true,
		Chromas:    []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		Transports: []string{"srt", "rtsp"},
	},
	{
		Name:       "h264_nvenc",
		Nvenc:      true,
		Chromas:    []string{"yuv444p", "yuv420p", "p010le"},
		Transports: []string{"srt", "rtsp", "webrtc"},
	},
	{
		// No transport carries AV1: MediaMTX's SRT/MPEG-TS ingest takes
		// H.264/H.265 only, ffmpeg has no AV1 RTP payloader for RTSP, and the
		// WHIP muxer carries H.264 only.
		Name:       "av1_nvenc",
		Nvenc:      true,
		Chromas:    []string{"yuv420p", "p010le"},
		Transports: []string{},
	},
	{
		Name:       "libx264",
		Nvenc:      false,
		Chromas:    []string{"yuv444p", "yuv420p", "p010le"},
		Transports: []string{"srt", "rtsp", "webrtc"},
	},
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

// CarriedBy reports whether transport can carry codec name.
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
