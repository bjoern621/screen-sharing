package capabilities

import screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

// Codecs is the capability table.
// Order is the UI display order: the wired backends first, then the hardware families no builder
// maps.
// An unwired row's Chromas are the formats its builder is to reach, and its absent CqMax is the
// scale that builder will set, so the picker can show the family without offering a codec that only
// fails at launch.
//
// A row's Chromas states every format either engine codes, and a format only one of them codes
// carries a Gap naming the other.
// The pair is read per engine (EngineChromas), so the wider list is the base and the gaps are the
// deltas: the option stays selectable on the engine that codes it and is greyed with the element's
// own limit on the engine that does not.
// Every chroma gap names the GStreamer engine, the narrower half of the two, since the ffmpeg engine
// reaches every format listed here.
//
// A chroma counts as reached where the encoder codes that subsampling and bit depth, not that memory
// layout.
// The software encoders take planar 10-bit (yuv420p10le) where the hardware ones take semi-planar
// p010le, so ffmpeg converts the layout on the way in and the GStreamer pipeline pins I420_10LE,
// for the same coded picture either way.
//
// 4:2:2 lives on the two software H.26x rows alone, through x264's High 4:2:2 and x265's Main 4:2:2
// 10.
// No hardware encoder here has an entrypoint for it, VP8 and VP9 have no 4:2:2 profile,
// and AV1's is the professional profile 2, which none of the three AV1 encoders here codes.
// A row states the subsampling its encoder codes, not the one the format defines.
//
// 10-bit H.264 is the High 10 profile and full chroma the High 4:4:4 Predictive one.
// x264 codes both and NVENC the second.
// No other H.264 encoder here codes either.
//
// The hardware families code 4:2:0, and 10-bit where the format has a profile for it: HEVC's Main 10
// and AV1's profile 0, which carries both bit depths.
// Full chroma and direct RGB (gbrp) are NVENC's alone among them: VAAPI reaches 4:4:4 only through
// the HEVC Range Extensions profile and QSV through that plus VP9's profile 1,
// and VP9's 10-bit profile 2 is the same case, each implemented by too few generations to declare
// per format.
var Codecs = []Codec{
	{
		// No planar-RGB gap, unlike the other RGB-coding rows: the nvcodec HEVC elements take a GBR sink
		// format and code it in the Range Extensions profile, so both engines reach gbrp.
		Name:        "hevc_nvenc",
		Effort:      Ladder{Steps: nvencPresets, Defaults: nvencPresetDefaults, Pins: []string{ModeCbr}},
		Tune:        Ladder{Steps: nvencTunes, Defaults: nvencTuneDefaults},
		Family:      FamilyNvenc,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
	},
	{
		Name:        "h264_nvenc",
		Effort:      Ladder{Steps: nvencPresets, Defaults: nvencPresetDefaults, Pins: []string{ModeCbr}},
		Tune:        Ladder{Steps: nvencTunes, Defaults: nvencTuneDefaults},
		Family:      FamilyNvenc,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p"},
		CqMax:       EveryEngine(51),
	},
	{
		// NVENC's lossless tune is an H.264 and HEVC one.
		// Its AV1 encoder reports no lossless capability.
		Name:        "av1_nvenc",
		Effort:      Ladder{Steps: nvencPresets, Defaults: nvencPresetDefaults, Pins: []string{ModeCbr}},
		Tune:        Ladder{Steps: nvencTunes, Defaults: nvencTuneDefaults},
		Family:      FamilyNvenc,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps: []Gap{{
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_NVENC_AV1_NO_LOSSLESS_TUNE,
		}},
	},
	{
		Name:        "libx264",
		Effort:      Ladder{Steps: x264Presets, Defaults: x264PresetDefaults},
		Tune:        Ladder{Steps: x264Tunes, Defaults: h26xTuneDefaults},
		Family:      FamilySoftware,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv422p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        []Gap{gstNoRateCeiling},
	},
	{
		Name:        "libx265",
		Effort:      Ladder{Steps: x264Presets, Defaults: x264PresetDefaults},
		Tune:        Ladder{Steps: x265Tunes, Defaults: h26xTuneDefaults},
		Family:      FamilySoftware,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        []Gap{gstNoPlanarRGB, gstNoRateCeiling},
	},
	{
		// Each chroma selects the VP9 profile that codes it on the ffmpeg engine (vp9Profiles):
		// 0 for 8-bit 4:2:0, 1 for 4:4:4 and for gbrp, which rides VP9's identity matrix so RGB stays
		// RGB, and 2 for 10-bit 4:2:0.
		Name:        "libvpx-vp9",
		Effort:      Ladder{Steps: vp9Speeds, Defaults: vp9Default},
		Family:      FamilySoftware,
		Format:      "vp9",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(63),
		Gaps: []Gap{gstNoPlanarRGB, gstNoRateCeiling, {
			Engine: EngineGst,
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_VP9ENC_NO_LOSSLESS,
		}},
	},
	{
		// One profile, one chroma, one bit depth: 8-bit 4:2:0 is the whole of VP8.
		Name:        "libvpx",
		Effort:      Ladder{Steps: vp8Speeds, Defaults: vp8Default},
		Family:      FamilySoftware,
		Format:      "vp8",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(63),
		Gaps: []Gap{vp8NoFullRange, gstNoRateCeiling, {
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_VP8_HAS_NO_LOSSLESS,
		}},
	},
	{
		// 10-bit is the ffmpeg engine's alone: libaom codes it, and av1enc's sink pad lists 8-bit formats
		// only.
		// The other two software AV1 encoders carry 10-bit on both engines.
		Name:        "libaom-av1",
		Effort:      Ladder{Steps: aomSpeeds, Defaults: aomDefault},
		Family:      FamilySoftware,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(63),
		Gaps: []Gap{gstNoPlanarRGB, gstNoRateCeiling, {
			Engine: EngineGst,
			Option: OptionChroma,
			Value:  "p010le",
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_AV1ENC_EIGHT_BIT_ONLY,
		}, {
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_LIBAOM_NO_LOSSLESS_SWITCH,
		}, {
			// Measured: the element's streams are byte-identical for a full-range and a limited-range
			// encode, and a decoder produces no colorimetry at all from either.
			Engine: EngineGst,
			Option: OptionColorRange,
			Value:  "pc",
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_AV1ENC_NO_COLOUR_DESCRIPTION,
		}},
	},
	{
		Name:        "libsvtav1",
		Effort:      Ladder{Steps: svtav1Steps, Defaults: svtav1Preset},
		Family:      FamilySoftware,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(63),
		// SVT-AV1 refuses a target above 100 Mbit/s outright, where every other encoder here takes what
		// it is given.
		// The refusal is the library's, so both engines meet it whichever property carries the rate.
		// The publish default target sits above it.
		BitrateLimitM: EveryEngine(100),
		Gaps: []Gap{
			{
				Option: OptionMode,
				Value:  ModeLossless,
				Reason: screensharev1.TextCode_TEXT_CODE_GAP_SVTAV1_NO_LOSSLESS,
			},
			{
				// Both engines, unlike the software rows around it: the refusal is the library's own,
				// so neither wrapper has a property to put a ceiling in.
				Option: OptionMode,
				Value:  ModeVbr,
				Reason: screensharev1.TextCode_TEXT_CODE_GAP_SVTAV1_NO_CONSTRAINED_VBR,
			},
			{
				// SVT-AV1 takes CBR in its low-delay prediction structure alone, and svtav1enc deadlocks the
				// moment that structure is selected: it takes the frames and produces nothing,
				// so the pipeline stalls instead of failing.
				// The ffmpeg wrapper drives the same library into CBR without it, which is what makes this one
				// engine's gap and not the codec's.
				Engine: EngineGst,
				Option: OptionMode,
				Value:  ModeCbr,
				Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_SVTAV1ENC_NO_CBR,
			},
		},
	},
	{
		// No gbrp: rav1e codes no RGB matrix.
		// Its rate control is a single one-pass bitrate target, which is also why the form greys the
		// rate-buffer control on this codec (form.availabilityEngineRules).
		Name:        "librav1e",
		Effort:      Ladder{Steps: rav1eSpeeds, Defaults: rav1eDefault},
		Family:      FamilySoftware,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv444p", "yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps: []Gap{{
			Option: OptionMode,
			Value:  ModeLossless,
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_RAV1E_NO_LOSSLESS,
		}, {
			// Both engines, as on libsvtav1: rav1e's rate control is one target and nothing above it,
			// whichever wrapper sets it.
			Option: OptionMode,
			Value:  ModeVbr,
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_RAV1E_NO_CONSTRAINED_VBR,
		}},
	},

	// VAAPI (Intel and AMD): one backend over both vendors' iGPU and dGPU encoder blocks,
	// and the hardware path on a non-NVIDIA Linux desktop.
	//
	// Which of the five rows a machine runs is the driver's answer rather than the table's:
	// an AMD card exposes no VP8 or VP9 encode entrypoint, an Intel one before Arc no AV1.
	// encoders.Detect test-encodes each and the UI greys away what this GPU refuses,
	// which is why a row declares the format's capability rather than one generation's.
	{
		Name:        "h264_vaapi",
		Family:      FamilyVaapi,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(51),
		Gaps:        vaapiGaps,
	},
	{
		Name:        "hevc_vaapi",
		Family:      FamilyVaapi,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        vaapiGaps,
	},
	{
		Name:        "av1_vaapi",
		Family:      FamilyVaapi,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps:        vaapiGaps,
	},
	{
		Name:        "vp9_vaapi",
		Family:      FamilyVaapi,
		Format:      "vp9",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(255),
		Gaps:        vaapiGaps,
	},
	{
		// The format's own colour-range gap leads the VAAPI ones: it holds on both engines where the
		// shared one holds on the GStreamer engine alone, and the first match is the reason reported.
		Name:        "vp8_vaapi",
		Family:      FamilyVaapi,
		Format:      "vp8",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(127),
		Gaps:        append([]Gap{vp8NoFullRange}, vaapiGaps...),
	},

	// QSV (Intel Quick Sync, through oneVPL): the second runtime onto an Intel GPU's encoder block,
	// where VAAPI is the first.
	// Intel implements this one itself, so it carries rate-control modes the VAAPI drivers leave out
	// and reaches a generation's formats before Mesa exposes them.
	//
	// Which of the four rows a machine runs is the generation's answer rather than the table's,
	// as on VAAPI: VP9 encode arrives with Ice Lake and AV1 encode with Arc, so encoders.Detect
	// test-encodes each and the UI greys away what this GPU refuses.
	{
		Name:        "h264_qsv",
		Effort:      Ladder{Steps: qsvTargetUsages, Defaults: qsvTargetUsageDefaults},
		Family:      FamilyQsv,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(51),
		Gaps:        qsvGaps,
	},
	{
		Name:        "hevc_qsv",
		Effort:      Ladder{Steps: qsvTargetUsages, Defaults: qsvTargetUsageDefaults},
		Family:      FamilyQsv,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        qsvGaps,
	},
	{
		Name:        "av1_qsv",
		Effort:      Ladder{Steps: qsvTargetUsages, Defaults: qsvTargetUsageDefaults},
		Family:      FamilyQsv,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps:        qsvGaps,
	},
	{
		// The one row whose quantizer scale differs per engine, and it is the publish path's rather than
		// the format's: ffmpeg's QSV encoders state a CQP quantizer on the H.26x 0-51 scale for every
		// codec but AV1, so a higher target is clamped on the way to oneVPL, where the qsvvp9enc element
		// passes VP9's own 0-255 q_idx straight through.
		// One number for both engines would take a quality step away from one or promise one the other
		// silently clamps.
		Name:        "vp9_qsv",
		Effort:      Ladder{Steps: qsvTargetUsages, Defaults: qsvTargetUsageDefaults},
		Family:      FamilyQsv,
		Format:      "vp9",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       map[string]int{EngineFfmpeg: 51, EngineGst: 255},
		Gaps:        qsvGaps,
	},

	// AMF (AMD Advanced Media Framework): AMD's own runtime onto the same VCN silicon VAAPI reaches on
	// an AMD card, through AMD's closed-source stack instead of Mesa.
	// The two are alternatives rather than layers.
	// VAAPI adds VP8 and VP9, which AMF has no encoder for.
	// AMF brings peak-constrained VBR, a burst ceiling a bitrate mode can target.
	//
	// The rows code 4:2:0: the RGB and 4:4:4 input formats the encoders accept are converted before
	// coding.
	// AMD ships the runtime for x86_64 alone, which is the encoder probe's answer rather than a table
	// fact: where it is missing the encoder refuses to open, exactly as on a machine with no AMD card.
	{
		Name:        "h264_amf",
		Effort:      Ladder{Steps: amfPresets, Defaults: amfPresetDefaults},
		Family:      FamilyAmf,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(51),
		Gaps:        amfGaps,
	},
	{
		// The builder selects the profile from the chroma (amfHevcProfiles): left to its default,
		// AMF writes a Main profile indication over a 10-bit bitstream.
		Name:        "hevc_amf",
		Effort:      Ladder{Steps: amfPresets, Defaults: amfPresetDefaults},
		Family:      FamilyAmf,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        amfGaps,
	},
	{
		// Full range is this row's gap alone in the family: measured, an encode asked for full range
		// signals limited in the sequence header, where the H.264 and HEVC rows on the same runtime signal
		// the range they are given.
		Name:        "av1_amf",
		Effort:      Ladder{Steps: amfPresets, Defaults: amfPresetDefaults},
		Family:      FamilyAmf,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps: append([]Gap{{
			Engine: EngineFfmpeg,
			Option: OptionColorRange,
			Value:  "pc",
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_AMF_AV1_LIMITED_RANGE_ONLY,
		}}, amfGaps...),
	},

	// Vulkan Video (cross-vendor): the video-encode extensions a GPU driver implements itself,
	// so one backend reaches NVIDIA, AMD and Intel silicon through the same API and on every platform
	// Vulkan runs on.
	// On an AMD or Intel card it drives the encoder block VAAPI reaches, through the vendor's Vulkan
	// driver instead of Mesa's VA layer.
	//
	// A driver implements the encode extension per format, so which of the three rows a machine runs is
	// the driver's answer rather than the table's, and encoders.Detect test-encodes each as on VAAPI.
	{
		Name:        "h264_vulkan",
		Tune:        Ladder{Steps: vulkanTunes, Defaults: vulkanTuneDefaults},
		Family:      FamilyVulkan,
		Format:      "h264",
		Implemented: true,
		Chromas:     []string{"yuv420p"},
		CqMax:       EveryEngine(51),
		Gaps:        vulkanGaps,
	},
	{
		Name:        "hevc_vulkan",
		Tune:        Ladder{Steps: vulkanTunes, Defaults: vulkanTuneDefaults},
		Family:      FamilyVulkan,
		Format:      "hevc",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(51),
		Gaps:        vulkanGaps,
	},
	{
		// Full range is this row's gap alone in the family, as on AMF: measured, an encode asked for full
		// range signals limited in the sequence header, where the H.264 and HEVC rows on the same driver
		// signal the range they are given.
		Name:        "av1_vulkan",
		Tune:        Ladder{Steps: vulkanTunes, Defaults: vulkanTuneDefaults},
		Family:      FamilyVulkan,
		Format:      "av1",
		Implemented: true,
		Chromas:     []string{"yuv420p", "p010le"},
		CqMax:       EveryEngine(255),
		Gaps: append([]Gap{{
			Engine: EngineFfmpeg,
			Option: OptionColorRange,
			Value:  "pc",
			Reason: screensharev1.TextCode_TEXT_CODE_GAP_VULKAN_AV1_LIMITED_RANGE_ONLY,
		}}, vulkanGaps...),
	},

	// V4L2 M2M: the kernel's memory-to-memory encoders, on a Raspberry Pi and some ARM SoCs.
	{Name: "h264_v4l2m2m", Family: FamilyV4l2, Format: "h264", Chromas: []string{"yuv420p"}},
	{Name: "hevc_v4l2m2m", Family: FamilyV4l2, Format: "hevc", Chromas: []string{"yuv420p"}},

	// Rockchip MPP: the RK35xx family's SoC encoders.
	{Name: "h264_rkmpp", Family: FamilyRkmpp, Format: "h264", Chromas: []string{"yuv420p"}},
	{Name: "hevc_rkmpp", Family: FamilyRkmpp, Format: "hevc", Chromas: []string{"yuv420p", "p010le"}},
}

// vaapiGaps are the gaps every VAAPI row carries.
//
// A VA encoder quantizes every frame and no VA profile exposes a transform-bypass path,
// so lossless has no VA form on either publish engine.
// The other four modes reach the drivers' CQP, CBR and VBR rate control.
//
// Full range is the colour range the GStreamer engine has no honest form of.
// Measured on all five formats, the va elements write no colour description into the bitstream and
// expose no property that would, so what a decoder produces from their streams carries no
// colorimetry at all.
// A viewer reads an unsignalled stream as limited-range BT.709.
// A full-range publish therefore arrives expanded a second time, with crushed blacks and clipped
// whites, while the form says the stream is full range.
// The ffmpeg engine tags the frames with the whole description (ffmpeg.colourFilter) and reaches
// both ranges on the same hardware.
var vaapiGaps = []Gap{
	{
		Option: OptionMode,
		Value:  ModeLossless,
		Reason: screensharev1.TextCode_TEXT_CODE_GAP_VAAPI_NO_LOSSLESS,
	},
	{
		Engine: EngineGst,
		Option: OptionColorRange,
		Value:  "pc",
		Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_VA_NO_COLOUR_DESCRIPTION,
	},
}

// qsvGaps is the rate-control gap every QSV row carries.
// Intel's encoders quantize every frame, and neither ffmpeg's QSV encoders nor the qsv elements
// expose the transform bypass a bit-exact stream needs.
// The other four modes map onto oneVPL's CQP, CBR and VBR rate control, which both engines reach.
var qsvGaps = []Gap{{
	Option: OptionMode,
	Value:  ModeLossless,
	Reason: screensharev1.TextCode_TEXT_CODE_GAP_QSV_NO_LOSSLESS,
}}

// amfGaps are the gaps every AMF row carries.
//
// The engine gap is a platform one rather than a codec one: GStreamer's amfcodec plugin builds its
// device layer on D3D11 and its meson configuration refuses any other host system,
// so the engine the portal capture backend runs on has no AMF element to name at all.
// That leaves the ffmpeg engine, whose encoders load AMD's libamfrt64 runtime directly.
//
// Lossless is the mode AMD's encoders have no form of, as on VAAPI: the quantizer options bottom out
// at a quantized picture, and no AMF property bypasses the transform the way x264's qp 0 or NVENC's
// lossless tune do.
// The other four modes map onto AMF's CQP, CBR and peak-constrained VBR rate control.
var amfGaps = []Gap{
	{
		Engine: EngineGst,
		Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_AMFCODEC_WINDOWS_ONLY,
	},
	{
		Option: OptionMode,
		Value:  ModeLossless,
		Reason: screensharev1.TextCode_TEXT_CODE_GAP_AMF_NO_LOSSLESS,
	},
}

// vulkanGaps are the gaps every Vulkan Video row carries.
//
// The engine gap is a memory one: the GStreamer vulkan encoder takes images on a Vulkan device,
// a memory the capture chain would have to upload to, and the plugin carries no HEVC or AV1 encoder
// to take them at all.
// That leaves the ffmpeg engine, whose encoders create the Vulkan device themselves and upload each
// frame through the filter graph.
//
// Vulkan's lossless tuning mode is the one place the options read as if the family coded bit-exact.
// It is a hint about what to optimize for rather than a coding mode, and nothing in the path pins
// the transform bypass such a stream needs.
// A picture encoded under it decodes back different from the source.
var vulkanGaps = []Gap{
	{
		Engine: EngineGst,
		Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_VULKAN_NO_CAPTURE_MEMORY,
	},
	{
		Option: OptionMode,
		Value:  ModeLossless,
		Reason: screensharev1.TextCode_TEXT_CODE_GAP_VULKAN_NO_LOSSLESS,
	},
}

// vp8NoFullRange is the colour-range gap both VP8 rows carry, on both publish engines.
// It belongs to the format rather than to an encoder: a VP8 keyframe header codes a single
// colour_space bit, which has one defined value, and no colour range field at all.
// Measured on libvpx through both engines, there is nothing for an encoder to write the range into
// and no property that would.
// A full-range publish would arrive at every viewer expanded as limited range, with crushed blacks
// and clipped whites, while the form says the stream is full range.
var vp8NoFullRange = Gap{
	Option: OptionColorRange,
	Value:  "pc",
	Reason: screensharev1.TextCode_TEXT_CODE_GAP_VP8_HAS_NO_COLOUR_RANGE_FIELD,
}

// gstNoRateCeiling is the constrained-VBR gap every software row carries on the GStreamer publish
// engine.
// None of those elements takes a ceiling above the target: x264enc's pass=cbr locks the VBV maxrate
// to the bitrate, and x265enc, vp8enc, vp9enc and av1enc expose a target and no maximum at all.
// Given a ceiling they run the uncapped average abr already names.
//
// It gaps the mode rather than greying the ceiling field, because the mode is what the element
// cannot do: a field greyed under a mode that silently becomes another mode still reports the wrong
// rate control in the command and in every verdict derived from it.
var gstNoRateCeiling = Gap{
	Engine: EngineGst,
	Option: OptionMode,
	Value:  ModeVbr,
	Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_SOFTWARE_NO_RATE_CEILING,
}

// gstNoPlanarRGB is the planar-RGB gap the software RGB-coding rows carry on the GStreamer publish
// engine.
// x265enc, vp9enc and av1enc take YUV only, so the pipeline could only convert RGB to 4:4:4 YUV and
// spend RGB's bitrate without its exactness.
// The ffmpeg engine codes each of those formats as RGB, which is what makes this a gap per engine
// rather than a pixel format the rows leave out.
//
// It is not every RGB-coding row: the nvcodec HEVC elements do take a GBR sink format and code it in
// the Range Extensions profile, so hevc_nvenc reaches planar RGB on both engines.
// Which elements take which layout is publish.gstFamilyChromaFormats, and this gap states only where
// none of them does.
var gstNoPlanarRGB = Gap{
	Engine: EngineGst,
	Option: OptionChroma,
	Value:  "gbrp",
	Reason: screensharev1.TextCode_TEXT_CODE_GAP_GST_ELEMENTS_NO_PLANAR_RGB,
}
