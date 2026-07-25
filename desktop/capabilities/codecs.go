package capabilities

// Codecs is the capability table. Order is the UI display order: implemented
// backends first, then the not-yet-implemented hardware families.
//
// The software encoders, NVENC and VAAPI are wired into the encoder argument
// builders. The remaining hardware families (QSV for Intel, AMF for AMD, V4L2 M2M
// and Rockchip MPP for ARM SoCs, cross-vendor Vulkan Video) are declared with
// Implemented:false so the two-dropdown picker can show them as a roadmap without
// offering a codec that would only fail at launch. Their Chromas and Transports
// are the values that will apply once each is wired up, not a promise that they
// work today.
//
// A row's Chromas and ModeGaps hold for both publish engines. Where the ffmpeg
// encoder and the GStreamer element disagree, the row states the narrower fact and
// says which side is the limit, so a combination the UI offers cannot fail on one
// capture backend and work on the other.
//
// Chroma note for the hardware families: consumer VAAPI/QSV/AMF encoders emit
// 4:2:0 (yuv420p), with 10-bit 4:2:0 (p010le) on the HEVC/AV1 Main-10 paths. Among
// the hardware families the 4:4:4 and direct-RGB (gbrp) modes are NVENC's alone,
// so those chromas are absent here on purpose: VAAPI reaches 4:4:4 only through the
// HEVC Range Extensions profile, which few drivers implement for encoding. CqMax
// stays zero on the unwired families as well: their quantizer scale is whatever
// their builder will set, so guessing one would be a fact the code does not honor.
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
		// RTSP alone carries AV1, as for every AV1 row: RTP has a payload mapping
		// for it, MediaMTX's SRT/MPEG-TS ingest takes H.264/H.265 only, and the WHIP
		// muxer carries H.264 only.
		Name:        "av1_nvenc",
		Family:      "nvenc",
		Format:      "av1",
		Nvenc:       true,
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       51,
		Transports:  []string{"rtsp"},
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
		// libvpx VP9: the one 4:4:4 codec a browser decodes in software
		// (WebCodecs), so it carries the lossless 4:4:4 modes to the web viewer.
		// RTSP only: MPEG-TS (SRT) has no VP9 mapping. Each chroma selects the VP9
		// profile that codes it, so all four reach the encoder: 0 for 8-bit 4:2:0,
		// 1 for 4:4:4 and for gbrp, which uses VP9's identity matrix so RGB stays
		// RGB, 2 for 10-bit 4:2:0.
		//
		// libvpx counts its quantizer to 63, not 51 like the H.26x encoders, so
		// the same CQ number means a different quality here.
		Name:        "libvpx-vp9",
		Family:      "software",
		Format:      "vp9",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       63,
		Transports:  []string{"rtsp"},
		ModeGaps: []ModeGap{{
			Engine: "gstreamer",
			Mode:   "lossless",
			Reason: "vp9enc exposes no lossless property, so libvpx's lossless coding is reachable through the ffmpeg capture backends only",
		}},
	},
	{
		// libvpx VP8. Every WebRTC stack decodes it, and it is the cheapest
		// royalty-free encode here, which makes it the fallback for a machine with
		// no GPU encoder and cores to spare. 8-bit 4:2:0 is the whole format: VP8
		// has one profile and no 4:4:4 or high bit depth. RTSP only, the same
		// MPEG-TS gap as VP9.
		Name:        "libvpx",
		Family:      "software",
		Format:      "vp8",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       63,
		Transports:  []string{"rtsp"},
		ModeGaps: []ModeGap{{
			Mode:   "lossless",
			Reason: "VP8 has no lossless coding mode; libvpx added that with VP9",
		}},
	},
	{
		// libaom, the AV1 reference encoder, and the only software AV1 here that
		// codes 4:4:4 and RGB. Its realtime usage profile keeps a screen encode
		// within reach on many cores, but it stays the slowest of the three.
		//
		// No p010le: the ffmpeg encoder codes 10-bit, the GStreamer element av1enc
		// negotiates 8-bit input only, and a chroma the table permits has to reach
		// both engines. 10-bit AV1 is libsvtav1's and librav1e's column.
		//
		// AV1 rides RTSP alone: MediaMTX's SRT/MPEG-TS ingest takes H.264/H.265
		// only, and the WHIP muxer takes H.264. RTP carries it either way, which is
		// what the relay needs to re-serve it.
		Name:        "libaom-av1",
		Family:      "software",
		Format:      "av1",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p"},
		CqMax:       63,
		Transports:  []string{"rtsp"},
		ModeGaps: []ModeGap{{
			Mode:   "lossless",
			Reason: "neither ffmpeg's libaom wrapper nor av1enc exposes libaom's lossless switch",
		}},
	},
	{
		// SVT-AV1: the fastest realtime AV1 encoder of the three, at 4:2:0 and
		// 10-bit 4:2:0 only. Its preset ladder runs to 13, far past what libaom or
		// rav1e reach, which is what makes AV1 practical at desktop resolutions.
		Name:        "libsvtav1",
		Family:      "software",
		Format:      "av1",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       63,
		// SVT-AV1 refuses a target above 100 Mbit/s outright, where every other
		// encoder here takes whatever it is given. AV1 at this preset has no use for
		// that rate anyway, but the settings default sits above it.
		BitrateLimitM: 100,
		Transports:    []string{"rtsp"},
		ModeGaps: []ModeGap{
			{
				Mode:   "lossless",
				Reason: "SVT-AV1 has no lossless coding mode",
			},
			{
				// SVT-AV1 takes CBR in its low-delay prediction structure only, and
				// svtav1enc deadlocks the moment that structure is selected: it takes
				// the frames and produces nothing, so the pipeline stalls instead of
				// failing. The ffmpeg wrapper drives the same library into CBR without
				// it, which is why this gap is one engine's and not the codec's.
				Engine: "gstreamer",
				Mode:   "cbr",
				Reason: "svtav1enc stalls in the low-delay prediction structure SVT-AV1's CBR requires, so constant bitrate is reachable through the ffmpeg capture backends only",
			},
		},
	},
	{
		// rav1e: AV1 at 4:4:4 and 10-bit, faster than libaom and slower than
		// SVT-AV1. Its rate control is a single one-pass bitrate target, so the
		// ceiling and rate-buffer fields do not bind (the frontend's engine rules
		// carry that). No gbrp: rav1e codes no RGB matrix.
		//
		// rav1e counts its quantizer to 255, four times the H.26x scale and the
		// widest here, so a CQ carried over from another codec means a very
		// different quality.
		Name:        "librav1e",
		Family:      "software",
		Format:      "av1",
		Nvenc:       false,
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p", "p010le"},
		CqMax:       255,
		Transports:  []string{"rtsp"},
		ModeGaps: []ModeGap{{
			Mode:   "lossless",
			Reason: "rav1e has no lossless coding mode",
		}},
	},

	// VAAPI (Intel + AMD): one backend drives both vendors' iGPU and dGPU
	// encoders, so it is the hardware path on a non-NVIDIA Linux desktop. All five
	// rows are 4:2:0, and none of them codes bit-exact (vaapiModeGaps).
	//
	// Which of the five a machine can run is a driver question, not a table one: an
	// AMD card exposes no VP8/VP9 encode entrypoint, an Intel one before Arc no AV1.
	// encoders.Detect test-encodes each and the UI greys away what this GPU refuses,
	// which is why the table declares the format's capability rather than one
	// generation's.
	{
		// 4:2:0 8-bit only. H.264 10-bit needs the High 10 profile and 4:4:4 the
		// High 4:4:4 Predictive one; no VAAPI driver offers either for encoding.
		Name:        "h264_vaapi",
		Family:      "vaapi",
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       51,
		Transports:  []string{"srt", "rtsp", "webrtc"},
		ModeGaps:    vaapiModeGaps,
	},
	{
		// Main and Main 10, the two HEVC profiles VAAPI drivers implement for
		// encoding. No webrtc, as on every HEVC row: WHIP is H.264 + Opus.
		Name:        "hevc_vaapi",
		Family:      "vaapi",
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       51,
		Transports:  []string{"srt", "rtsp"},
		ModeGaps:    vaapiModeGaps,
	},
	{
		// AV1 profile 0 carries both bit depths, so 10-bit needs no second profile.
		// The quantizer knob is AV1's base_q_idx, a 0-255 scale rather than the
		// H.26x 0-51 one, so the same CQ number means a different quality here.
		Name:        "av1_vaapi",
		Family:      "vaapi",
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       255,
		Transports:  []string{"rtsp"},
		ModeGaps:    vaapiModeGaps,
	},
	{
		// VP9 profile 0. Profile 2 (10-bit) stays out for the same reason as the
		// HEVC Range Extensions: too few drivers expose it for encoding. RTSP only,
		// as on the libvpx row: MPEG-TS has no VP9 mapping. The quantizer is VP9's
		// 0-255 q_idx.
		Name:        "vp9_vaapi",
		Family:      "vaapi",
		Format:      "vp9",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       255,
		Transports:  []string{"rtsp"},
		ModeGaps:    vaapiModeGaps,
	},
	{
		// VP8 has one profile, one chroma and one bit depth, so this row is the
		// whole format. Its quantizer index counts to 127.
		Name:        "vp8_vaapi",
		Family:      "vaapi",
		Format:      "vp8",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       127,
		Transports:  []string{"rtsp"},
		ModeGaps:    vaapiModeGaps,
	},

	// QSV (Intel Quick Sync, via oneVPL). Intel-only; tends to beat generic
	// VAAPI on quality and rate control on the same Intel silicon.
	{Name: "h264_qsv", Family: "qsv", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_qsv", Family: "qsv", Format: "hevc", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"srt", "rtsp"}},
	{Name: "av1_qsv", Family: "qsv", Format: "av1", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"rtsp"}},
	{Name: "vp9_qsv", Family: "qsv", Format: "vp9", Chromas: []string{"yuv420p"}, Transports: []string{"rtsp"}},

	// AMF (AMD Media Framework). AMD's own encoder path; Linux support is weaker
	// than VAAPI on the same cards, so VAAPI is usually the better AMD target.
	{Name: "h264_amf", Family: "amf", Format: "h264", Chromas: []string{"yuv420p"}, Transports: []string{"srt", "rtsp", "webrtc"}},
	{Name: "hevc_amf", Family: "amf", Format: "hevc", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"srt", "rtsp"}},
	{Name: "av1_amf", Family: "amf", Format: "av1", Chromas: []string{"yuv420p", "p010le"}, Transports: []string{"rtsp"}},

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

// vaapiModeGaps is the rate-control gap every VAAPI row carries. A VA encoder
// quantizes every frame and no VA profile exposes a transform-bypass path, so
// lossless has no VAAPI form on either publish engine. The other four modes reach
// the drivers' CQP, CBR and VBR rate control.
var vaapiModeGaps = []ModeGap{{
	Mode:   "lossless",
	Reason: "VAAPI's fixed-function encoders quantize every frame, and no VA profile codes bit-exact",
}}
