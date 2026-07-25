package ffmpeg

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// rates are the rate-control figures a stream's settings yield, rendered as the
// values ffmpeg takes on the command line. The GOP length travels with them, in
// frames, because one encoder aligns a parameter-set repeat to it (amfArgs) and the
// keyframe interval is otherwise the command's own -g.
type rates struct {
	bitrate, maxrate, cq, bframes, gop string
}

func ratesFor(s settings.Stream, gop int) rates {
	return rates{
		bitrate: fmt.Sprintf("%dM", s.BitrateM),
		maxrate: fmt.Sprintf("%dM", s.MaxrateM),
		cq:      strconv.Itoa(s.Cq),
		bframes: strconv.Itoa(s.Bframes),
		gop:     strconv.Itoa(gop),
	}
}

// encoderMappings is the codec-specific half of the command, one entry per
// encoder whose knobs differ. The rate-control modes are the methods themselves:
// cbr and vbr and abr all target a bitrate and differ in the ceiling, crf targets
// a quality, lossless is bit-exact. The GStreamer publish engine expresses the same
// set in gstCodecs. A mode an encoder has no form of is declared as a Gap in
// capabilities.Codecs and rejected before a command is built, so the branch for it
// is absent here rather than approximated.
//
// The NVENC family is matched by capability rather than by name (encoderArgs),
// because one mapping serves every nvenc codec.
var encoderMappings = map[string]func(s settings.Stream, r rates) []string{
	// x264 reaches bit-exact at -qp 0; x265 has no bit-exact qp, so lossless is
	// its own param.
	"libx264":    softwareArgs("libx264", []string{"-qp", "0"}),
	"libx265":    softwareArgs("libx265", []string{"-x265-params", "lossless=1"}),
	"libvpx-vp9": vp9Args,
	"libvpx":     vp8Args,
	"libaom-av1": aomArgs,
	"libsvtav1":  svtav1Args,
	"librav1e":   rav1eArgs,
	// One VAAPI mapping serves all five codecs; only the option the quantizer
	// travels in differs. The H.26x encoders own a -qp option, while the VP8, VP9
	// and AV1 ones ignore it and read ffmpeg's generic -global_quality.
	"h264_vaapi": vaapiArgs("-qp"),
	"hevc_vaapi": vaapiArgs("-qp"),
	"av1_vaapi":  vaapiArgs("-global_quality"),
	"vp9_vaapi":  vaapiArgs("-global_quality"),
	"vp8_vaapi":  vaapiArgs("-global_quality"),
	// One AMF mapping serves all three codecs, with what differs between them bound
	// here: the profile a chroma selects, and the options only some of the three own.
	"h264_amf": amfArgs(nil, amfH264Options),
	"hevc_amf": amfArgs(amfHevcProfiles, nil),
	"av1_amf":  amfArgs(nil, amfNoBPictures),
}

// encoderArgs returns the encoder arguments for the configured codec and rate
// control mode. gop is the resolved keyframe interval in frames, which the command
// also carries as -g.
func encoderArgs(s settings.Stream, gop int) ([]string, error) {
	r := ratesFor(s, gop)
	if build, ok := encoderMappings[s.Codec]; ok {
		return build(s, r), nil
	}
	if capabilities.IsNvenc(s.Codec) {
		return nvencArgs(s, r), nil
	}
	// Generic bitrate-targeted path. capabilities.Validate rejects every codec whose
	// entry has Implemented:false, so the not-yet-wired hardware families (qsv, v4l2,
	// rkmpp, vulkan) never reach here. When one is implemented, give it its own
	// mapping: QSV needs its own device and load path, as VAAPI needs a device and an
	// upload filter chain (vaapi.go), so none of them fit this bare -b:v fallback.
	return []string{"-c:v", s.Codec, "-b:v", r.bitrate}, nil
}

// softwareArgs is the rate-control mapping shared by the CPU H.26x encoders
// libx264 and libx265; the five modes match the software path of gstCodecs. Only
// the encoder name and the lossless knob differ between the two, so both are bound
// per codec in encoderMappings.
func softwareArgs(codec string, lossless []string) func(settings.Stream, rates) []string {
	return func(s settings.Stream, r rates) []string {
		switch s.Mode {
		case "crf":
			return []string{"-c:v", codec, "-preset", "slow", "-crf", r.cq}
		case "lossless":
			// No rate control, bursts to hundreds of Mbit/s. zerolatency keeps live
			// delay by dropping the B-frames and lookahead lossless gains little from.
			return append([]string{"-c:v", codec, "-preset", "veryfast", "-tune", "zerolatency"}, lossless...)
		case "abr":
			// One-pass average bitrate, no VBV cap: quality holds and bitrate bursts
			// freely toward the target average.
			return []string{"-c:v", codec, "-preset", "medium", "-b:v", r.bitrate}
		case "vbr":
			// Constrained VBR: targets the bitrate but bursts up to the maxrate
			// ceiling on motion. bufsize sizes the ceiling's VBV window.
			return []string{
				"-c:v", codec, "-preset", "medium",
				"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs),
			}
		default: // cbr
			// maxrate = bitrate with a bounded bufsize is true CBR; without them
			// -b:v alone is one-pass ABR and bursts past a capped link.
			return []string{
				"-c:v", codec, "-preset", "veryfast", "-tune", "zerolatency",
				"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs),
			}
		}
	}
}

// aomRates appends the rate-control options shared by the encoders built on
// Google's aom-family rate control: libvpx (VP8 and VP9) and libaom (AV1). All
// three read the same generic ffmpeg knobs, so only the base command and the
// lossless branch differ per codec, and those stay with the caller.
func aomRates(base []string, s settings.Stream, r rates) []string {
	switch s.Mode {
	case "crf":
		// Constant quality: -b:v 0 lets the crf target alone drive the rate.
		return append(base, "-crf", r.cq, "-b:v", "0")
	case "abr":
		return append(base, "-b:v", r.bitrate)
	case "vbr":
		return append(base,
			"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs))
	default: // cbr: minrate = b:v = maxrate with a bounded buffer.
		return append(base,
			"-minrate", r.bitrate, "-b:v", r.bitrate, "-maxrate", r.bitrate,
			"-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
	}
}

// vp9Profiles maps the configured chroma to the VP9 profile that codes it. VP9
// splits its profiles by subsampling and bit depth, and libvpx refuses a pixel
// format the selected profile cannot carry, so this is what lets one codec row
// offer all four chromas. gbrp travels in profile 1 through VP9's identity matrix,
// which keeps RGB as RGB.
var vp9Profiles = map[string]string{
	"yuv420p": "0",
	"yuv444p": "1",
	"gbrp":    "1",
	"p010le":  "2",
}

// vp9Args maps the rate-control modes to libvpx VP9. The realtime, row-mt,
// cpu-used and tune-content=screen knobs keep a live screen-content encode within
// a few cores. libvpx is the one software encoder here whose lossless mode is
// bit-exact by a dedicated flag rather than a quantizer of zero.
func vp9Args(s settings.Stream, r rates) []string {
	base := []string{
		"-c:v", "libvpx-vp9", "-profile:v", vp9Profiles[s.Chroma],
		"-deadline", "realtime", "-row-mt", "1", "-cpu-used", "6",
		"-tune-content", "screen",
	}
	if s.Mode == "lossless" {
		return append(base, "-lossless", "1")
	}
	return aomRates(base, s, r)
}

// vp8Args maps the rate-control modes to libvpx VP8. VP8 has a single profile and
// no lossless mode, and libvpx's VP9-only threading and content tuning do not
// apply; screen-content-mode is VP8's own equivalent, turning on the coding tools
// for text and sharp edges. cpu-used runs higher than on VP9 because VP8 has less
// to trade away.
func vp8Args(s settings.Stream, r rates) []string {
	return aomRates([]string{
		"-c:v", "libvpx", "-deadline", "realtime", "-cpu-used", "8",
		"-screen-content-mode", "1",
	}, s, r)
}

// aomArgs maps the rate-control modes to libaom AV1. The realtime usage profile
// switches libaom off its two-pass defaults; without it a live encode falls
// minutes behind. cpu-used 8 is the fastest point the realtime profile offers,
// row-mt and the tile split spread the frame over cores, and lag-in-frames 0 drops
// the lookahead that would otherwise hold frames back.
func aomArgs(s settings.Stream, r rates) []string {
	return aomRates([]string{
		"-c:v", "libaom-av1", "-usage", "realtime", "-cpu-used", "8",
		"-row-mt", "1", "-tiles", "2x1", "-lag-in-frames", "0",
	}, s, r)
}

// svtav1Preset is the point on SVT-AV1's 0-13 quality/speed ladder the modes
// encode at. 9 is the fastest preset the encoder still calls a quality preset;
// 10 and above are documented as automation targets and show visual artifacts.
const svtav1Preset = "9"

// svtav1Args maps the rate-control modes to SVT-AV1, whose rate control does not
// follow the aom shape:
//   - crf targets a quality with -crf and takes no bitrate.
//   - abr and vbr are the library's VBR mode, which -b:v alone selects. Neither
//     takes -maxrate: SVT-AV1 accepts a rate ceiling in constant-quality mode
//     only, and rejects the whole encode when given one in VBR.
//   - cbr is rate-control mode 2, reachable only through -svtav1-params, and it
//     requires the low-delay prediction structure. buf-sz sizes its rate buffer in
//     milliseconds, which the element clamps to its own 20-10000 window.
func svtav1Args(s settings.Stream, r rates) []string {
	base := []string{"-c:v", "libsvtav1", "-preset", svtav1Preset}
	switch s.Mode {
	case "crf":
		return append(base, "-crf", r.cq)
	case "abr", "vbr":
		return append(base, "-b:v", r.bitrate)
	default: // cbr
		params := "rc=2:pred-struct=1"
		if s.VbvMs > 0 {
			params += ":buf-sz=" + strconv.Itoa(s.VbvMs)
		}
		return append(base, "-b:v", r.bitrate, "-svtav1-params", params)
	}
}

// rav1eArgs maps the rate-control modes to rav1e. Its rate control is one
// bitrate target with no ceiling and no rate buffer, so cbr, vbr and abr differ
// only in whether frame reordering is dropped for delay. speed 10 is the fastest
// of its eleven presets, and its quantizer counts to 255 rather than 63.
func rav1eArgs(s settings.Stream, r rates) []string {
	base := []string{"-c:v", "librav1e", "-speed", "10"}
	switch s.Mode {
	case "crf":
		return append(base, "-qp", r.cq)
	case "abr", "vbr":
		return append(base, "-b:v", r.bitrate)
	default: // cbr
		return append(base, "-b:v", r.bitrate, "-rav1e-params", "low_latency=true")
	}
}

// vaapiArgs maps the rate-control modes onto the VAAPI encoders' shared -rc_mode
// knob, the counterpart to vaEncoder on the GStreamer path. quantizer is the option
// the constant-quality target travels in, bound per codec in encoderMappings.
//   - crf: CQP, one fixed quantizer and no rate bound.
//   - cbr: CBR at the target, the VBV window sizing its coded-picture buffer.
//   - vbr: VBR targeting the bitrate and bursting to the maxrate ceiling.
//   - abr: VBR with no ceiling given, which ffmpeg turns into a ceiling of twice
//     the target: the closest VAAPI comes to an unbounded average, since its rate
//     control always codes against a maximum.
//
// CQP, CBR and VBR are the three modes both vendors' drivers implement. ffmpeg also
// offers ICQ, QVBR and AVBR, which are Intel-only or absent, and a driver handed a
// mode it lacks refuses to open the encoder rather than falling back. lossless has
// no VAAPI form at all (vaapiGaps).
//
// No preset or B-frame count: the p1-p7 ladder is NVENC's, and VAAPI B-frame
// support varies per driver and hardware generation, so the form greys both fields
// for this family.
func vaapiArgs(quantizer string) func(settings.Stream, rates) []string {
	return func(s settings.Stream, r rates) []string {
		switch s.Mode {
		case "crf":
			return []string{"-c:v", s.Codec, "-rc_mode", "CQP", quantizer, r.cq}
		case "abr":
			return []string{"-c:v", s.Codec, "-rc_mode", "VBR", "-b:v", r.bitrate}
		case "vbr":
			return []string{
				"-c:v", s.Codec, "-rc_mode", "VBR",
				"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs),
			}
		default: // cbr
			return []string{
				"-c:v", s.Codec, "-rc_mode", "CBR",
				"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs),
			}
		}
	}
}

// amfHevcProfiles maps the configured chroma to the HEVC profile that indicates its
// bit depth. AMF leaves the profile indication at Main whatever the input surface
// carries, so a p010le encode comes out as a 10-bit bitstream announcing an 8-bit
// profile, which a decoder gating on the profile is entitled to refuse. AMF's H.264
// encoder needs no such map, its row being 8-bit, and neither does its AV1 one,
// where profile 0 carries both depths.
var amfHevcProfiles = map[string]string{
	"yuv420p": "main",
	"p010le":  "main10",
}

// amfUsage is the AMF usage preset every mode encodes under. A usage preset is a
// bundle of AMD defaults applied ahead of the rate-control properties, so it sets the
// encoder's character while -rc still decides the rate.
//
// It is the same preset in all four modes, including cbr, because AMF's low-latency
// presets are unusable for a stream a viewer joins late: under lowlatency and
// ultralowlatency its H.264 encoder drops the IDR period and codes one IDR for the
// whole stream, so a viewer who subscribes after it finds no recovery point and its
// decoder never starts. The keyframe interval the settings ask for is the stronger
// claim, so the preset gives way, and cbr states its low-delay character through the
// speed end of the quality scale and the B-pictures the mappings pin off.
// transcoding is also the one preset every VCN generation implements; high_quality
// is refused by the older ones.
const amfUsage = "transcoding"

// The two points on AMF's speed/quality scale the modes take. All three encoders
// spell them the same way, though the numbers behind the names differ per codec: cbr
// trades quality for the encoder keeping up with a live capture, as the NVENC preset
// ladder does at the same point.
const (
	amfLivePreset    = "speed"
	amfQualityPreset = "quality"
)

// amfAbrPeak is the factor the abr mapping places its peak ceiling at above the
// bitrate target, ffmpeg's own derivation for a hardware VBR encode given none.
const amfAbrPeak = 2

// amfH264Options are the options AMD's H.264 encoder needs beyond the shared mapping.
// header_spacing repeats SPS and PPS every GOP, so they arrive with each IDR: the
// encoder writes them once at stream start otherwise, and a viewer who subscribes
// later then has no parameter sets to configure its decoder with. Its HEVC encoder
// repeats VPS/SPS/PPS per IDR on its own, and AV1 carries no out-of-band parameter
// sets at all, so this is the one row that needs telling.
func amfH264Options(r rates) []string {
	return append(amfNoBPictures(r), "-header_spacing", r.gop)
}

// amfNoBPictures switches off the B-picture pattern of the two AMF encoders that have
// one, rather than leaving it to the usage preset: the settings' B-frame count is
// NVENC's alone, and a live screen stream pays their reorder delay for a gain it
// cannot spend. AMD's HEVC encoder codes no B-frames at all and has no such option.
func amfNoBPictures(rates) []string {
	return []string{"-bf", "0"}
}

// amfArgs maps the rate-control modes onto the AMF encoders' -rc knob, the AMD
// counterpart to vaapiArgs. profiles selects the profile a chroma needs, nil where the
// codec has one profile for every chroma it codes, and options adds what only some of
// the three encoders own.
//   - crf: cqp, a fixed quantizer per frame type and no rate bound.
//   - cbr: cbr at the target, the VBV window sizing the rate buffer.
//   - vbr: peak-constrained VBR, targeting the bitrate and bursting to the maxrate
//     ceiling.
//   - abr: the same peak-constrained VBR with the ceiling at twice the target, which
//     is what ffmpeg derives for a VAAPI encode given no ceiling, so an unbounded
//     average means the same thing on both hardware families. AMF codes against a
//     peak either way, and left unset it keeps the usage preset's own, which is not a
//     rate the settings ever stated.
//
// AMF's remaining rate-control modes stay out: qvbr targets a quality level on a scale
// of its own, and hqvbr and hqcbr are the pre-analysis variants, which the older VCN
// generations do not implement. lossless has no AMF form at all (amfGaps).
//
// The speed/quality preset is the builder's choice rather than the settings'
// EncPreset: that field is NVENC's p1-p7 ladder, which has no AMF equivalent, and the
// form greys it for this family.
func amfArgs(profiles map[string]string, options func(rates) []string) func(settings.Stream, rates) []string {
	return func(s settings.Stream, r rates) []string {
		base := []string{"-c:v", s.Codec}
		if profile, ok := profiles[s.Chroma]; ok {
			base = append(base, "-profile", profile)
		}
		if options != nil {
			base = append(base, options(r)...)
		}
		preset := amfQualityPreset
		if s.Mode == "cbr" {
			preset = amfLivePreset
		}
		base = append(base, "-usage", amfUsage, "-quality", preset)
		switch s.Mode {
		case "crf":
			return append(base, "-rc", "cqp", "-qp_i", r.cq, "-qp_p", r.cq)
		case "abr":
			return append(base, "-rc", "vbr_peak", "-b:v", r.bitrate,
				"-maxrate", fmt.Sprintf("%dM", s.BitrateM*amfAbrPeak))
		case "vbr":
			return append(base, "-rc", "vbr_peak", "-b:v", r.bitrate, "-maxrate", r.maxrate,
				"-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs))
		default: // cbr
			return append(base, "-rc", "cbr", "-b:v", r.bitrate, "-maxrate", r.bitrate,
				"-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
		}
	}
}

// The NVENC preset the modes default to when the settings carry none: the quality
// end of the p1-p7 ladder, except for cbr, where a live stream trades quality for
// the encoder keeping up.
const (
	nvencQualityPreset = "p7"
	nvencLivePreset    = "p5"
)

// nvencArgs maps the rate-control modes onto the NVENC SDK's knobs: preset ladder,
// tune, and rc mode. It serves every nvenc codec, the codec name itself being the
// only difference between them.
func nvencArgs(s settings.Stream, r rates) []string {
	preset := s.EncPreset
	if preset == "" {
		preset = nvencQualityPreset
	}
	switch s.Mode {
	case "lossless":
		// True nvenc lossless: no rate control, the frame costs whatever exactness
		// costs and can burst well past 1 Gbps. B-frames are pinned off, not taken
		// from the settings: bit-exact coding gains nothing from them and the UI greys
		// the field for that reason.
		return []string{"-c:v", s.Codec, "-preset", preset, "-tune", "lossless", "-bf", "0"}
	case "crf":
		// VBR targeting a constant quantizer: cq drives the look, the bitrate only
		// caps bursts. multipass fullres spends the most effort per bit.
		return []string{
			"-c:v", s.Codec, "-preset", preset, "-tune", "hq", "-multipass", "fullres",
			"-rc", "vbr", "-cq", r.cq, "-b:v", "0", "-maxrate", r.bitrate, "-bufsize", r.bitrate,
			"-bf", r.bframes,
		}
	case "abr":
		// VBR toward an average with no ceiling.
		return []string{"-c:v", s.Codec, "-preset", preset, "-tune", "hq", "-rc", "vbr", "-b:v", r.bitrate, "-bf", r.bframes}
	case "vbr":
		return []string{
			"-c:v", s.Codec, "-preset", preset, "-tune", "hq", "-rc", "vbr",
			"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs),
			"-bf", r.bframes,
		}
	default: // cbr
		if s.EncPreset == "" {
			preset = nvencLivePreset
		}
		args := []string{"-c:v", s.Codec, "-preset", preset, "-tune", "ll", "-rc", "cbr", "-b:v", r.bitrate, "-bf", "0"}
		if s.VbvMs > 0 {
			args = append(args, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
		}
		return args
	}
}

// bufsizeArg returns the ffmpeg -bufsize value in kbit for a rate (Mbit/s) held
// over a VBV window. A zero window defaults to one second, the conventional CBR
// buffer. rateM Mbit/s over ms milliseconds is rateM*ms kbit.
func bufsizeArg(rateM, vbvMs int) string {
	ms := vbvMs
	if ms <= 0 {
		ms = 1000
	}
	return strconv.Itoa(rateM*ms) + "k"
}
