package capabilities

// Codecs is the capability table. Order is the UI display order: implemented
// backends first, then the not-yet-implemented hardware families.
//
// Only NVENC and the software x264/x265/libvpx-vp9 encoders are wired into the
// encoder argument builders. The
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
// here on purpose. CqMax stays zero there as well: an unwired family's quantizer
// scale is whatever its builder will set, so guessing one would be a fact the
// code does not yet honor.
var Codecs = []Codec{
	{
		// No webrtc: ffmpeg's WHIP muxer carries H.264 and Opus only.
		Name:        "hevc_nvenc",
		Family:      "nvenc",
		Format:      "hevc",
		Nvenc:       true,
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       51,
		Transports:  []string{"srt", "rtsp"},
	},
	{
		Name:        "h264_nvenc",
		Family:      "nvenc",
		Format:      "h264",
		Nvenc:       true,
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p", "p010le"},
		CqMax:       51,
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
		CqMax:       51,
		Transports:  []string{},
	},
	{
		Name:        "libx264",
		Family:      "software",
		Format:      "h264",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p", "p010le"},
		CqMax:       51,
		Transports:  []string{"srt", "rtsp", "webrtc"},
	},
	{
		// Same format facts as hevc_nvenc: HEVC codes RGB via the Range
		// Extensions (gbrp), and no registered transport carries it over webrtc
		// (ffmpeg's WHIP muxer is H.264 + Opus only).
		Name:        "libx265",
		Family:      "software",
		Format:      "hevc",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       51,
		Transports:  []string{"srt", "rtsp"},
	},
	{
		// libvpx VP9 profile 1: the one 4:4:4 codec a browser decodes in software
		// (WebCodecs), so it carries the lossless 4:4:4 modes to the web viewer.
		// RTSP only: MPEG-TS (SRT) has no VP9 mapping. gbrp uses VP9's identity
		// matrix so RGB stays RGB; yuv444p is the Y'CbCr 4:4:4 form.
		//
		// libvpx counts its quantizer to 63, not 51 like the H.26x encoders, so
		// the same CQ number means a different quality here.
		Name:        "libvpx-vp9",
		Family:      "software",
		Format:      "vp9",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p"},
		CqMax:       63,
		Transports:  []string{"rtsp"},
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
