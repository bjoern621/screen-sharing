package capabilities

// Codecs is the capability table. Order is the UI display order: implemented
// backends first, then the not-yet-implemented hardware families.
//
// The software encoders, NVENC, VAAPI, QSV, AMF and Vulkan Video are wired into the
// encoder argument builders. The remaining hardware families (V4L2 M2M and Rockchip
// MPP for ARM SoCs) are declared with Implemented:false so the two-dropdown picker can
// show them as a roadmap without offering a codec that would only fail at launch.
// Their Chromas are the formats that will apply once each is wired up, not a promise
// that they work today.
//
// A row's Chromas states every format either engine codes, and each format the
// ffmpeg encoder and the GStreamer element disagree on carries a Gap naming the
// engine that lacks it. The pair is read per engine (EngineChromas), so the wider
// list is a base and the gaps are the deltas rather than an answer either engine
// has to live with: the option stays selectable on the engine that codes it and is
// greyed with the element's own limit on the engine that does not. Every chroma gap
// runs the same way round, since the GStreamer elements are the narrower half: the
// ffmpeg engine reaches every format the table lists.
//
// A chroma counts as reached when the encoder codes that subsampling and bit depth,
// not that exact memory layout. The software encoders take planar 10-bit
// (yuv420p10le) where the hardware ones take semi-planar p010le, so ffmpeg converts
// the layout on the way in and the GStreamer pipeline pins I420_10LE; the coded
// picture is the same 10-bit 4:2:0 either way.
//
// Chroma note for 4:2:2: it is the two software H.26x rows' alone. x264 codes it
// through High 4:2:2 and x265 through Main 4:2:2 10, both engines reaching the
// same profiles. No hardware encoder here has an entrypoint for it, VP8 and VP9
// have no 4:2:2 profile, and AV1's is the professional profile 2, which libaom
// codes and neither of the two fast AV1 encoders does. A row is added where the
// encoder codes the subsampling, not where the format defines it.
//
// Chroma note for the hardware families: consumer VAAPI/QSV/AMF encoders emit
// 4:2:0 (yuv420p), with 10-bit 4:2:0 (p010le) on the HEVC/AV1 Main-10 paths. Among
// the hardware families the 4:4:4 and direct-RGB (gbrp) modes are NVENC's alone,
// so those chromas are absent here on purpose: VAAPI reaches 4:4:4 only through the
// HEVC Range Extensions profile, and QSV through the same profile plus VP9's profile 1,
// which too few generations implement for encoding to declare per format. The
// unwired families state no CqMax entry for either engine: their quantizer scale is
// whatever their builder will set, so guessing one would be a fact the code does not
// honor.
var Codecs = []Codec{
	{
		Name:        "hevc_nvenc",
		Family:      FamilyNvenc,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        []Gap{gstNoPlanarRGB},
	},
	{
		// 8-bit only, the same limit every other H.264 row states: 10-bit H.264 is
		// the High 10 profile, which NVENC's H.264 encoder does not implement. Its
		// 4:4:4 support is the High 4:4:4 Predictive profile, which NVENC does.
		Name:        "h264_nvenc",
		Family:      FamilyNvenc,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p"},
		CqMax:       EveryEngine(51),
	},
	{
		// NVENC's lossless tune is an H.264 and HEVC one. The AV1 encoder reports no
		// lossless capability, so this row carries the gap every other AV1 row does.
		Name:        "av1_nvenc",
		Family:      FamilyNvenc,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps: []Gap{{
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: "NVENC codes bit-exact through its lossless tune, which its AV1 encoder does not implement",
		}},
	},
	{
		// 4:2:2 is the High 4:2:2 profile, which x264 codes and no hardware H.264
		// encoder here does, so this row and libx265 are where the middle
		// subsampling lives.
		Name:        "libx264",
		Family:      FamilySoftware,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv422p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        []Gap{gstNoRateCeiling},
	},
	{
		// Same format facts as hevc_nvenc: HEVC codes RGB through its Range
		// Extensions profile, which is what puts gbrp in the list. 4:2:2 is in the
		// same profile family (Main 4:2:2 10), which x265 codes.
		Name:        "libx265",
		Family:      FamilySoftware,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        []Gap{gstNoPlanarRGB, gstNoRateCeiling},
	},
	{
		// libvpx VP9: the one 4:4:4 codec a browser decodes in software
		// (WebCodecs), so it carries the lossless 4:4:4 modes to the web viewer.
		// On the ffmpeg engine each chroma selects the VP9 profile that codes it
		// (vp9Profiles), so all four reach the encoder: 0 for 8-bit 4:2:0, 1 for
		// 4:4:4 and for gbrp, which uses VP9's identity matrix so RGB stays RGB,
		// 2 for 10-bit 4:2:0.
		//
		// libvpx counts its quantizer to 63, not 51 like the H.26x encoders, so
		// the same CQ number means a different quality here.
		Name:        "libvpx-vp9",
		Family:      FamilySoftware,
		Format:      "vp9",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(63),
		Gaps: []Gap{gstNoPlanarRGB, gstNoRateCeiling, {
			Engine: EngineGst,
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: "the vp9enc element exposes no lossless property, so libvpx's lossless coding is reachable on the ffmpeg publish engine only",
		}},
	},
	{
		// libvpx VP8. Every WebRTC stack decodes it, and it is the cheapest
		// royalty-free encode here, which makes it the fallback for a machine with
		// no GPU encoder and cores to spare. 8-bit 4:2:0 is the whole format: VP8
		// has one profile and no 4:4:4 or high bit depth.
		Name:        "libvpx",
		Family:      FamilySoftware,
		Format:      "vp8",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(63),
		Gaps: []Gap{vp8NoFullRange, gstNoRateCeiling, {
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: "VP8 has no lossless coding mode; libvpx added that with VP9",
		}},
	},
	{
		// libaom, the AV1 reference encoder, and the only software AV1 here that
		// codes 4:4:4 and RGB. Its realtime usage profile keeps a screen encode
		// within reach on many cores, but it stays the slowest of the three.
		//
		// 10-bit is the ffmpeg engine's alone: libaom codes it, and av1enc's sink pad
		// lists 8-bit formats only. The other two software AV1 encoders carry 10-bit
		// on both engines.
		Name:        "libaom-av1",
		Family:      FamilySoftware,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(63),
		Gaps: []Gap{gstNoPlanarRGB, gstNoRateCeiling, {
			Engine: EngineGst,
			Option: OptionChroma,
			Value:  "p010le",
			Reason: "the av1enc element takes 8-bit input only, so 10-bit libaom AV1 is reachable on the ffmpeg publish engine only",
		}, {
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: "neither the ffmpeg libaom encoder nor the av1enc element exposes libaom's lossless switch",
		}, {
			// Measured: the element's streams are byte-identical for a full-range and a
			// limited-range encode, and what a decoder produces from either carries no
			// colorimetry at all.
			Engine: EngineGst,
			Option: OptionColorRange,
			Value:  "pc",
			Reason: "the av1enc element writes no colour description into the sequence header, so a full-range stream arrives at every viewer expanded as limited range; full range is reachable on the ffmpeg publish engine, whose libaom encoder tags what it codes",
		}},
	},
	{
		// SVT-AV1: the fastest realtime AV1 encoder of the three, at 4:2:0 and
		// 10-bit 4:2:0 only. Its preset ladder runs to 13, far past what libaom or
		// rav1e reach, which is what makes AV1 practical at desktop resolutions.
		Name:        "libsvtav1",
		Family:      FamilySoftware,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(63),
		// SVT-AV1 refuses a target above 100 Mbit/s outright, where every other
		// encoder here takes whatever it is given. The refusal is the library's, so
		// both engines meet it whichever property they set the rate through. AV1 at
		// this preset has no use for that rate anyway, but the settings default sits
		// above it.
		BitrateLimitM: EveryEngine(100),
		Gaps: []Gap{
			{
				Option: OptionMode,
				Value:  ModeLossless,
				Reason: "SVT-AV1 has no lossless coding mode",
			},
			{
				// Both engines, unlike the software rows around it: the refusal is the
				// library's own rather than one wrapper's, so neither engine has a
				// property to put the ceiling in.
				Option: OptionMode,
				Value:  ModeVbr,
				Reason: "SVT-AV1 accepts a rate ceiling in constant-quality mode only and rejects a VBR encode given one, so no publish engine can constrain the burst; abr is the same encode named for what it does",
			},
			{
				// SVT-AV1 takes CBR in its low-delay prediction structure only, and
				// svtav1enc deadlocks the moment that structure is selected: it takes
				// the frames and produces nothing, so the pipeline stalls instead of
				// failing. The ffmpeg wrapper drives the same library into CBR without
				// it, which is why this gap is one engine's and not the codec's.
				Engine: EngineGst,
				Option: OptionMode,
				Value:  ModeCbr,
				Reason: "the svtav1enc element stalls in the low-delay prediction structure SVT-AV1's CBR requires, so constant bitrate is reachable on the ffmpeg publish engine only",
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
		Family:      FamilySoftware,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps: []Gap{{
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: "rav1e has no lossless coding mode",
		}, {
			// Both engines, as on libsvtav1: rav1e's rate control is one target and
			// nothing above it, whichever wrapper sets it.
			Option: OptionMode,
			Value:  ModeVbr,
			Reason: "rav1e's one-pass rate control takes a bitrate target and nothing above it, so no publish engine can constrain the burst; abr is the same encode named for what it does",
		}},
	},

	// VAAPI (Intel + AMD): one backend drives both vendors' iGPU and dGPU
	// encoders, so it is the hardware path on a non-NVIDIA Linux desktop. All five
	// rows are 4:2:0, none of them codes bit-exact, and none signals a colour
	// description on the GStreamer engine (vaapiGaps).
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
		Family:      FamilyVaapi,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(51),
		Gaps:        vaapiGaps,
	},
	{
		// Main and Main 10, the two HEVC profiles VAAPI drivers implement for
		// encoding.
		Name:        "hevc_vaapi",
		Family:      FamilyVaapi,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        vaapiGaps,
	},
	{
		// AV1 profile 0 carries both bit depths, so 10-bit needs no second profile.
		// The quantizer knob is AV1's base_q_idx, a 0-255 scale rather than the
		// H.26x 0-51 one, so the same CQ number means a different quality here.
		Name:        "av1_vaapi",
		Family:      FamilyVaapi,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps:        vaapiGaps,
	},
	{
		// VP9 profile 0. Profile 2 (10-bit) stays out for the same reason as the
		// HEVC Range Extensions: too few drivers expose it for encoding. The
		// quantizer is VP9's 0-255 q_idx.
		Name:        "vp9_vaapi",
		Family:      FamilyVaapi,
		Format:      "vp9",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(255),
		Gaps:        vaapiGaps,
	},
	{
		// VP8 has one profile, one chroma and one bit depth, so this row is the
		// whole format. Its quantizer index counts to 127.
		//
		// The format's own colour-range gap comes first: it holds on both engines,
		// where the VAAPI one the row shares holds on the GStreamer engine alone.
		Name:        "vp8_vaapi",
		Family:      FamilyVaapi,
		Format:      "vp8",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(127),
		Gaps:        append([]Gap{vp8NoFullRange}, vaapiGaps...),
	},

	// QSV (Intel Quick Sync, via oneVPL): the second way to an Intel GPU's encoder
	// block, where VAAPI is the first. The two drive the same silicon through
	// different runtimes, and QSV is the one Intel implements itself, so it carries
	// the rate-control modes VAAPI drivers leave out and reaches formats a
	// generation supports before Mesa exposes them.
	//
	// Which of the four rows a machine runs is the generation's answer rather than the
	// table's, as on VAAPI: VP9 encode arrives with Ice Lake and AV1 encode with Arc,
	// so encoders.Detect test-encodes each and the UI greys away what this GPU refuses.
	// All four rows are 4:2:0, the subsampling every generation encodes, with 10-bit on
	// the HEVC and AV1 rows; qsvGaps carries what none of them does.
	{
		// 4:2:0 8-bit only, the limit every H.264 row states: 10-bit is the High 10
		// profile and full chroma the High 4:4:4 Predictive one, and Intel's H.264
		// encoder implements neither.
		Name:        "h264_qsv",
		Family:      FamilyQsv,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(51),
		Gaps:        qsvGaps,
	},
	{
		// Main and Main 10. The profile needs no selecting: oneVPL writes the
		// indication from the bit depth of the surfaces the encoder is handed.
		Name:        "hevc_qsv",
		Family:      FamilyQsv,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        qsvGaps,
	},
	{
		// AV1 profile 0 carries both bit depths, so 10-bit needs no second profile, and
		// the quantizer is AV1's base_q_idx on its 0-255 scale rather than the H.26x
		// 0-51 one.
		Name:        "av1_qsv",
		Family:      FamilyQsv,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps:        qsvGaps,
	},
	{
		// VP9 profile 0, as on VAAPI: profile 2 (10-bit) and profile 1 (4:4:4) stay out
		// for the same reason as the HEVC Range Extensions.
		//
		// The quantizer scale is the one row where the two engines differ, and it is
		// the publish path's rather than the format's: ffmpeg's QSV encoders state a
		// CQP quantizer on the H.26x 0-51 scale for every codec but AV1, so a target
		// above 51 is clamped on the way to oneVPL instead of reaching it, while the
		// qsvvp9enc element passes VP9's own 0-255 q_idx straight through. Stating
		// either number for both would take a quality step away from one engine or
		// promise one the other silently clamps.
		Name:        "vp9_qsv",
		Family:      FamilyQsv,
		Format:      "vp9",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       map[string]int{EngineFfmpeg: 51, EngineGst: 255},
		Gaps:        qsvGaps,
	},

	// AMF (AMD Advanced Media Framework): AMD's own encoder API, driving the same
	// VCN silicon VAAPI reaches on an AMD card through AMD's closed-source runtime
	// instead of Mesa. The two are alternatives, not layers, and each has what the
	// other lacks. VAAPI is the wider of the two here, adding VP8 and VP9, which AMF
	// has no encoder for; AMF brings AMD's own rate control, whose peak-constrained
	// VBR gives a burst ceiling a bitrate mode can actually target.
	//
	// All three rows are 4:2:0 (the RGB and 4:4:4 input formats the encoders accept
	// are converted to 4:2:0 before coding), and none codes bit-exact. amfGaps carries
	// that and the family's absence from the GStreamer engine. AMD ships the runtime
	// for x86_64 alone, which is the encoder probe's answer rather than a table fact:
	// where it is missing the encoder refuses to open, exactly as on a machine with no
	// AMD card.
	{
		Name:        "h264_amf",
		Family:      FamilyAmf,
		Format:      "h264",
		Implemented: true,
		// 8-bit only: 10-bit H.264 is the High 10 profile, which no VCN encoder
		// implements. The AMF quantizer options count on the H.26x 0-51 scale.
		Chromas: []string{"yuv420p"},
		CqMax:   EveryEngine(51),
		Gaps:    amfGaps,
	},
	{
		// Main and Main 10. The builder selects the profile from the chroma
		// (amfHevcProfiles), since AMF writes a Main profile indication over a 10-bit
		// bitstream when left to its default.
		Name:        "hevc_amf",
		Family:      FamilyAmf,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        amfGaps,
	},
	{
		// AV1 profile 0 carries both bit depths, so 10-bit needs no second profile,
		// and the quantizer is AV1's base_q_idx on its 0-255 scale rather than the
		// H.26x 0-51 one.
		//
		// Full range is this row's alone in the family: measured, an encode at full
		// range signals limited in the sequence header, where the H.264 and HEVC rows
		// on the same runtime signal the range they are given.
		Name:        "av1_amf",
		Family:      FamilyAmf,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps: append([]Gap{{
			Engine: EngineFfmpeg,
			Option: OptionColorRange,
			Value:  "pc",
			Reason: "the AMF AV1 encoder writes limited range into the sequence header whatever colour range it is given, so a full-range stream arrives at every viewer expanded as limited range",
		}}, amfGaps...),
	},

	// Vulkan Video (cross-vendor): the video-encode extensions a GPU driver implements
	// itself, so one backend reaches NVIDIA, AMD and Intel silicon through the same API
	// and on every platform Vulkan runs on. On an AMD or Intel card it drives the same
	// encoder block VAAPI does, through the vendor's Vulkan driver instead of Mesa's VA
	// layer, which makes it the second way to that hardware where AMF is the third.
	//
	// All three rows are 4:2:0, and vulkanGaps carries what none of them does. Which of
	// the three a machine runs is the driver's answer rather than the table's, as on
	// VAAPI: a driver implements the encode extension per format, and encoders.Detect
	// test-encodes each so the UI greys away what this GPU refuses.
	{
		// 8-bit only, the limit every H.264 row states: 10-bit H.264 is the High 10
		// profile, which no Vulkan encode profile covers. The quantizer counts on the
		// H.26x 0-51 scale.
		Name:        "h264_vulkan",
		Family:      FamilyVulkan,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(51),
		Gaps:        vulkanGaps,
	},
	{
		// Main and Main 10. The profile needs no selecting, unlike on AMF: ffmpeg
		// writes the indication from the bit depth of the surfaces it is handed, so a
		// p010le encode announces Main 10 on its own.
		Name:        "hevc_vulkan",
		Family:      FamilyVulkan,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        vulkanGaps,
	},
	{
		// AV1 profile 0 carries both bit depths, so 10-bit needs no second profile, and
		// the quantizer is AV1's base_q_idx on its 0-255 scale rather than the H.26x
		// 0-51 one. RTSP alone, as on every AV1 row.
		//
		// Full range is this row's alone in the family, as on AMF: measured, an encode
		// at full range signals limited in the sequence header, where the H.264 and
		// HEVC rows on the same driver signal the range they are given.
		Name:        "av1_vulkan",
		Family:      FamilyVulkan,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps: append([]Gap{{
			Engine: EngineFfmpeg,
			Option: OptionColorRange,
			Value:  "pc",
			Reason: "the Vulkan AV1 encoder writes limited range into the sequence header whatever colour range it is given, so a full-range stream arrives at every viewer expanded as limited range",
		}}, vulkanGaps...),
	},

	// V4L2 M2M (kernel memory-to-memory encoders: Raspberry Pi, some ARM SoCs).
	{Name: "h264_v4l2m2m", Family: FamilyV4l2, Format: "h264", Chromas: []string{"yuv420p"}},
	{Name: "hevc_v4l2m2m", Family: FamilyV4l2, Format: "hevc", Chromas: []string{"yuv420p"}},

	// Rockchip MPP (RK35xx and similar SoC hardware encoders).
	{Name: "h264_rkmpp", Family: FamilyRkmpp, Format: "h264", Chromas: []string{"yuv420p"}},
	{Name: "hevc_rkmpp", Family: FamilyRkmpp, Format: "hevc", Chromas: []string{"yuv420p", "p010le"}},
}

// vaapiGaps are the gaps every VAAPI row carries.
//
// Lossless is the mode VA has no form of on either publish engine: a VA encoder
// quantizes every frame and no VA profile exposes a transform-bypass path. The other
// four modes reach the drivers' CQP, CBR and VBR rate control.
//
// Full range is the colour range the GStreamer engine has no honest form of. The va
// elements write no colour description into the bitstream and expose no property that
// would, measured on all five formats: what a decoder produces from their streams
// carries no colorimetry at all. A viewer reads an unsignalled stream as limited-range
// BT.709, so a full-range publish arrives expanded a second time, with crushed blacks
// and clipped whites, while the form says the stream is full range. Limited range is
// the one the same viewer assumes, so the picture holds. The ffmpeg engine tags the
// frames with the whole description (ffmpeg.colourFilter) and reaches both ranges on
// the same hardware.
var vaapiGaps = []Gap{
	{
		Option: OptionMode,
		Value:  ModeLossless,
		Reason: "VAAPI's fixed-function encoders quantize every frame, and no VA profile codes bit-exact",
	},
	{
		Engine: EngineGst,
		Option: OptionColorRange,
		Value:  "pc",
		Reason: "the va encoder elements signal no colour description, so a full-range stream arrives at every viewer expanded as limited range; full range is reachable on the ffmpeg publish engine, which tags the frames it encodes",
	},
}

// qsvGaps is the rate-control gap every QSV row carries. Intel's encoders quantize
// every frame, and neither ffmpeg's QSV encoders nor the qsv elements expose the
// transform bypass a bit-exact stream needs, so lossless has no QSV form on either
// publish engine. The other four modes map onto oneVPL's CQP, CBR and VBR rate control,
// which both engines reach.
var qsvGaps = []Gap{{
	Option: OptionMode,
	Value:  ModeLossless,
	Reason: "Intel's fixed-function encoders quantize every frame, and oneVPL exposes no transform-bypass path",
}}

// amfGaps are the gaps every AMF row carries.
//
// The engine gap is a platform one rather than a codec one: GStreamer's amfcodec
// plugin builds its device layer on D3D11 and its meson configuration refuses any
// other host system, so the engine the portal capture backend runs has no AMF
// element to name at all. That leaves the ffmpeg engine, whose encoders load AMD's
// libamfrt64 runtime directly, as the only way to reach the family.
//
// Lossless is the mode AMD's encoders have no form of, as on VAAPI: the quantizer
// options bottom out at a quantized picture, and no AMF property bypasses the
// transform the way x264's qp 0 or NVENC's lossless tune do. The other four modes
// map onto AMF's CQP, CBR and peak-constrained VBR rate control.
var amfGaps = []Gap{
	{
		Engine: EngineGst,
		Reason: "the GStreamer amfcodec plugin builds for Windows only, so AMD AMF is reachable on the ffmpeg publish engine alone",
	},
	{
		Option: OptionMode,
		Value:  ModeLossless,
		Reason: "AMF's fixed-function encoders quantize every frame, and no AMF profile codes bit-exact",
	},
}

// vulkanGaps are the gaps every Vulkan Video row carries.
//
// The engine gap is a memory one: the GStreamer vulkan encoder takes images on a
// Vulkan device, a memory the capture chain would have to upload to, and the plugin
// carries no HEVC or AV1 encoder to take them at all. That leaves the ffmpeg engine,
// whose encoders create the Vulkan device themselves and upload each frame through
// the filter graph, as the way to reach the family.
//
// Lossless is the mode Vulkan has no form of, as on VAAPI and AMF, and the one place
// its options read as if it did: the API's lossless tuning mode is a hint about what
// to optimize for, not a coding mode, and nothing in the path pins the transform
// bypass a bit-exact stream needs. A picture encoded under it decodes back different
// from the source.
var vulkanGaps = []Gap{
	{
		Engine: EngineGst,
		Reason: "the GStreamer vulkan plugin encodes from Vulkan device memory, which no capture backend on this engine produces, so Vulkan Video is reachable on the ffmpeg publish engine alone",
	},
	{
		Option: OptionMode,
		Value:  ModeLossless,
		Reason: "Vulkan's lossless tuning mode is a hint rather than a coding mode, and its encoders quantize under it all the same",
	},
}

// vp8NoFullRange is the colour-range gap both VP8 rows carry, on both publish engines.
// It belongs to the format rather than to an encoder: a VP8 keyframe header codes a
// single colour_space bit, which has one defined value, and no colour range field at
// all. There is nothing for an encoder to write the range into and no property that
// would, measured on libvpx through both engines, so a full-range publish would arrive
// at every viewer expanded as limited range, with crushed blacks and clipped whites,
// while the form says the stream is full range. Limited range is the one the same
// viewer assumes, so the picture holds.
var vp8NoFullRange = Gap{
	Option: OptionColorRange,
	Value:  "pc",
	Reason: "the VP8 bitstream has no colour range field, so a full-range stream arrives at every viewer expanded as limited range; the other formats here carry the range and reach both",
}

// gstNoRateCeiling is the constrained-VBR gap every software row carries on the
// GStreamer publish engine. None of those elements takes a ceiling above the target:
// x264enc's pass=cbr locks the VBV maxrate to the bitrate, and x265enc, vp8enc,
// vp9enc and av1enc expose a target and no maximum at all. What they run given a
// ceiling is the uncapped average abr already names, so offering VBR here would put
// two names on one encode and leave the ceiling field standing over a value nothing
// reads.
//
// It is a gap on the mode rather than a note on the ceiling field because the mode
// is what the element cannot do. A field greyed under a mode that silently becomes
// another mode still reports the wrong rate control in the command and in every
// verdict derived from it.
var gstNoRateCeiling = Gap{
	Engine: EngineGst,
	Option: OptionMode,
	Value:  ModeVbr,
	Reason: "no GStreamer software encoder element takes a rate ceiling above the target, so constrained VBR is reachable on the ffmpeg publish engine only; abr is the same encode named for what it does",
}

// gstNoPlanarRGB is the planar-RGB gap every RGB-coding row carries on the GStreamer
// publish engine. No encoder element there takes a GBR sink format: x265enc, vp9enc
// and av1enc take YUV only, as do the nvcodec elements, so the pipeline could only
// convert RGB to 4:4:4 YUV and spend RGB's bitrate without its exactness. The ffmpeg
// publish engine codes each of those formats as RGB, which is why this is a gap per
// engine and not a pixel format the rows leave out.
var gstNoPlanarRGB = Gap{
	Engine: EngineGst,
	Option: OptionChroma,
	Value:  "gbrp",
	Reason: "no GStreamer encoder element takes planar-RGB input, so direct RGB coding is reachable on the ffmpeg publish engine only",
}
