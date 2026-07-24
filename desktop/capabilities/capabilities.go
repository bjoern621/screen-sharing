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
	// Transports lists the transport registry keys that can carry this codec.
	// Empty means no registered transport carries it (e.g. AV1 over MPEG-TS).
	Transports []string `json:"transports"`
}

// Codecs is the capability table. Order is the UI display order: implemented
// backends first, then the not-yet-implemented hardware families.
//
// Only NVENC and software x264 are wired into the encoder argument builders. The
// non-NVIDIA hardware families (VAAPI for Intel/AMD, QSV for Intel, AMF for AMD,
// V4L2 M2M and Rockchip MPP for ARM SoCs, cross-vendor Vulkan Video) are declared
// with Implemented:false so the two-dropdown picker can show them as a roadmap
// without offering a codec that would only fail at launch. Their Chromas and
// Transports are the values that will apply once each is wired up, not a promise
// that they work today.
//
// Chroma note for the hardware families: consumer VAAPI/QSV/AMF encoders emit
// 4:2:0 (yuv420p), with 10-bit 4:2:0 (p010le) on the HEVC/AV1 Main-10 paths. The
// 4:4:4 and direct-RGB (gbrp) modes stay NVENC-only, so those chromas are absent
// here on purpose.
var Codecs = []Codec{
	{
		// No webrtc: ffmpeg's WHIP muxer carries H.264 and Opus only.
		Name:        "hevc_nvenc",
		Family:      "nvenc",
		Format:      "hevc",
		Nvenc:       true,
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		Transports:  []string{"srt", "rtsp"},
	},
	{
		Name:        "h264_nvenc",
		Family:      "nvenc",
		Format:      "h264",
		Nvenc:       true,
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p", "p010le"},
		Transports:  []string{"srt", "rtsp", "webrtc"},
	},
	{
		// No transport carries AV1: MediaMTX's SRT/MPEG-TS ingest takes
		// H.264/H.265 only, ffmpeg has no AV1 RTP payloader for RTSP, and the
		// WHIP muxer carries H.264 only.
		Name:        "av1_nvenc",
		Family:      "nvenc",
		Format:      "av1",
		Nvenc:       true,
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		Transports:  []string{},
	},
	{
		Name:        "libx264",
		Family:      "software",
		Format:      "h264",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p", "p010le"},
		Transports:  []string{"srt", "rtsp", "webrtc"},
	},

	// VAAPI (Intel + AMD). The single most useful addition for a non-NVIDIA
	// Linux desktop: one backend drives both vendors' iGPU/dGPU encoders.
	{Name: "h264_vaapi", Family: "vaapi", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_vaapi", Family: "vaapi", Format: "hevc", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"srt", "rtsp"}},
	{Name: "av1_vaapi", Family: "vaapi", Format: "av1", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{}},
	{Name: "vp9_vaapi", Family: "vaapi", Format: "vp9", Chromas: []string{"yuv420p"}, Transports: []string{}},
	{Name: "vp8_vaapi", Family: "vaapi", Format: "vp8", Chromas: []string{"yuv420p"}, Transports: []string{}},

	// QSV (Intel Quick Sync, via oneVPL). Intel-only; tends to beat generic
	// VAAPI on quality and rate control on the same Intel silicon.
	{Name: "h264_qsv", Family: "qsv", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_qsv", Family: "qsv", Format: "hevc", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"srt", "rtsp"}},
	{Name: "av1_qsv", Family: "qsv", Format: "av1", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{}},
	{Name: "vp9_qsv", Family: "qsv", Format: "vp9", Chromas: []string{"yuv420p"}, Transports: []string{}},

	// AMF (AMD Media Framework). AMD's own encoder path; Linux support is weaker
	// than VAAPI on the same cards, so VAAPI is usually the better AMD target.
	{Name: "h264_amf", Family: "amf", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_amf", Family: "amf", Format: "hevc", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"srt", "rtsp"}},
	{Name: "av1_amf", Family: "amf", Format: "av1", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{}},

	// V4L2 M2M (kernel memory-to-memory encoders: Raspberry Pi, some ARM SoCs).
	{Name: "h264_v4l2m2m", Family: "v4l2", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_v4l2m2m", Family: "v4l2", Format: "hevc", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp"}},

	// Rockchip MPP (RK35xx and similar SoC hardware encoders).
	{Name: "h264_rkmpp", Family: "rkmpp", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_rkmpp", Family: "rkmpp", Format: "hevc", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"srt", "rtsp"}},

	// Vulkan Video (cross-vendor, driver-provided). Newest and least mature path;
	// works anywhere the GPU driver exposes the Vulkan video-encode extensions.
	{Name: "h264_vulkan", Family: "vulkan", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_vulkan", Family: "vulkan", Format: "hevc", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"srt", "rtsp"}},
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
