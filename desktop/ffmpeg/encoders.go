package ffmpeg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

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

// encoderArgsFunc builds the codec-specific half of the command for one stream.
type encoderArgsFunc func(s settings.Stream, r rates) []string

// encoderMappings is the codec-specific half of the command, one entry per
// encoder whose knobs differ. The rate-control modes are the methods themselves:
// cbr and vbr and abr all target a bitrate and differ in the ceiling, crf targets
// a quality, lossless is bit-exact. The GStreamer publish engine expresses the same
// set in gstCodecs. A mode an encoder has no form of is declared as a Gap in
// capabilities.Codecs and rejected before a command is built, so the branch for it
// is absent here rather than approximated.
//
// A family whose codecs share one mapping outright belongs in familyMappings instead,
// which is what keeps a codec added to such a family from needing a row here.
var encoderMappings = map[string]encoderArgsFunc{
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
	// One QSV mapping serves all four codecs: the rate control is oneVPL's, so the
	// codec decides only which of its modes the generation implements.
	"h264_qsv": qsvArgs,
	"hevc_qsv": qsvArgs,
	"av1_qsv":  qsvArgs,
	"vp9_qsv":  qsvArgs,
	// One AMF mapping serves all three codecs, with what differs between them bound
	// here: the profile a chroma selects, and the options only some of the three own.
	"h264_amf": amfArgs(nil, amfH264Options),
	"hevc_amf": amfArgs(amfHevcProfiles, nil),
	"av1_amf":  amfArgs(nil, amfNoBPictures),
	// One Vulkan mapping serves all three codecs: the options are the encode
	// extension's own, so the codec decides only which of them the driver honours.
	"h264_vulkan": vulkanArgs,
	"hevc_vulkan": vulkanArgs,
	"av1_vulkan":  vulkanArgs,
}

// familyMappings is the same half of the command for the families whose codecs share
// one mapping outright, keyed as capabilities.Codecs names the family. The NVENC codecs
// differ only in the codec name the mapping already reads off the settings, so the
// family is the unit. VAAPI and AMF are keyed per codec instead, since what differs
// between their codecs (the option the quantizer travels in, the profile a chroma
// selects) is bound at the row.
var familyMappings = map[string]encoderArgsFunc{
	capabilities.FamilyNvenc: nvencArgs,
}

// familyLimits are the settings bounds an encoder family imposes beyond what
// capabilities.Validate checks, keyed as capabilities.Codecs names the family. It is
// the counterpart of gstFamilyLimits on the GStreamer builder. A family absent here
// takes whatever the capability table already approved.
//
// The bound lives here rather than in the capability table because it is a property
// of the option the family's mapping reads, not of a codec: every NVENC row takes
// the same preset ladder, and no other family reads the field at all.
var familyLimits = map[string]func(settings.Stream) error{
	capabilities.FamilyNvenc: nvencPresetLimit,
}

// encoderArgs returns the encoder arguments for the configured codec and rate
// control mode. gop is the resolved keyframe interval in frames, which the command
// also carries as -g. A codec's own row wins over its family's mapping.
//
// A codec neither table maps is refused rather than run on a bare -b:v guess. The
// families still declared Implemented:false (v4l2, rkmpp) each need a device and
// a load path of their own, as VAAPI, QSV and Vulkan do (hwsurface.go), and none of
// them encodes from a command that carries only a bitrate. capabilities.Validate
// rejects those codecs ahead of this either way.
func encoderArgs(s settings.Stream, gop int) ([]string, error) {
	c, ok := capabilities.Get(s.Codec)
	if !ok {
		return nil, fmt.Errorf("unknown codec %q", s.Codec)
	}
	// The family's bound is checked before either mapping runs, so a codec with a row
	// of its own is held to it as well.
	if limits, ok := familyLimits[c.Family]; ok {
		if err := limits(s); err != nil {
			return nil, err
		}
	}

	r := ratesFor(s, gop)
	if build, ok := encoderMappings[s.Codec]; ok {
		return build(s, r), nil
	}
	if build, ok := familyMappings[c.Family]; ok {
		return build(s, r), nil
	}
	return nil, fmt.Errorf("codec %q has no ffmpeg encoder mapping", s.Codec)
}

// softwareArgs is the rate-control mapping shared by the CPU H.26x encoders
// libx264 and libx265; the five modes match the software path of gstCodecs. Only
// the encoder name and the lossless knob differ between the two, so both are bound
// per codec in encoderMappings.
func softwareArgs(codec string, lossless []string) encoderArgsFunc {
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
		case "cbr":
			// maxrate = bitrate with a bounded bufsize is true CBR; without them
			// -b:v alone is one-pass ABR and bursts past a capped link.
			return []string{
				"-c:v", codec, "-preset", "veryfast", "-tune", "zerolatency",
				"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs),
			}
		default:
			assert.Never("unexpected rate-control mode", s.Mode)
			return nil
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
	case "cbr":
		// minrate = b:v = maxrate with a bounded buffer.
		return append(base,
			"-minrate", r.bitrate, "-b:v", r.bitrate, "-maxrate", r.bitrate,
			"-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
	default:
		assert.Never("unexpected rate-control mode", s.Mode)
		return nil
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
//   - abr is the library's VBR mode, which -b:v alone selects and which takes no
//     -maxrate. The vbr mode has no form on either engine and is declared as a gap:
//     SVT-AV1 accepts a rate ceiling in constant-quality mode only, and rejects the
//     whole encode when given one in VBR.
//   - cbr is rate-control mode 2, reachable only through -svtav1-params, and it
//     requires the low-delay prediction structure. buf-sz sizes its rate buffer in
//     milliseconds, which the element clamps to its own 20-10000 window.
func svtav1Args(s settings.Stream, r rates) []string {
	base := []string{"-c:v", "libsvtav1", "-preset", svtav1Preset}
	switch s.Mode {
	case "crf":
		return append(base, "-crf", r.cq)
	case "abr":
		return append(base, "-b:v", r.bitrate)
	case "cbr":
		params := "rc=2:pred-struct=1"
		if s.VbvMs > 0 {
			params += ":buf-sz=" + strconv.Itoa(s.VbvMs)
		}
		return append(base, "-b:v", r.bitrate, "-svtav1-params", params)
	default:
		assert.Never("unexpected rate-control mode", s.Mode)
		return nil
	}
}

// rav1eArgs maps the rate-control modes to rav1e. Its rate control is one
// bitrate target with no ceiling and no rate buffer, so vbr is declared as a gap on
// both engines and cbr and abr differ only in whether frame reordering is dropped
// for delay. speed 10 is the fastest
// of its eleven presets, and its quantizer counts to 255 rather than 63.
func rav1eArgs(s settings.Stream, r rates) []string {
	base := []string{"-c:v", "librav1e", "-speed", "10"}
	switch s.Mode {
	case "crf":
		return append(base, "-qp", r.cq)
	case "abr":
		return append(base, "-b:v", r.bitrate)
	case "cbr":
		return append(base, "-b:v", r.bitrate, "-rav1e-params", "low_latency=true")
	default:
		assert.Never("unexpected rate-control mode", s.Mode)
		return nil
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
func vaapiArgs(quantizer string) encoderArgsFunc {
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
		case "cbr":
			return []string{
				"-c:v", s.Codec, "-rc_mode", "CBR",
				"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs),
			}
		default:
			assert.Never("unexpected rate-control mode", s.Mode)
			return nil
		}
	}
}

// The two points on oneVPL's target-usage scale the modes encode at, spelled as ffmpeg
// names them: cbr trades quality for the encoder keeping up with a live capture, as the
// NVENC preset ladder and the AMF quality scale do at the same point, and the other three
// sit at the balanced point in the middle of the scale.
const (
	qsvLivePreset    = "veryfast"
	qsvQualityPreset = "medium"
)

// qsvAbrPeak is the factor the abr mapping places its ceiling at above the bitrate
// target, the same derivation the VAAPI, AMF and Vulkan mappings use.
const qsvAbrPeak = 2

// qsvLiveAsyncDepth is the number of frames the cbr mode lets the encoder keep in flight.
// oneVPL pipelines four by default, three frame periods of delay a live stream pays for
// throughput it does not need at a screen's frame rate.
const qsvLiveAsyncDepth = "1"

// qsvArgs maps the rate-control modes onto the QSV encoders' options, the Intel
// counterpart to vaapiArgs and amfArgs. It serves every QSV codec, the codec name itself
// being the only difference between them.
//
// This family has no -rc_mode to set: oneVPL's method is derived from which rate options
// carry a value, so each branch below is the shape that names one method.
//   - crf: -q:v carries the quantizer and sets ffmpeg's qscale flag with it, which is
//     what selects CQP. The value is the quantizer itself rather than a quality level,
//     and ffmpeg states it on the H.26x 0-51 scale for every codec but AV1, which
//     capabilities.Codecs carries as the CqMax of each row.
//   - cbr: a ceiling equal to the target selects CBR, the VBV window sizing its rate
//     buffer. async_depth drops the encoder's pipeline from four frames in flight to
//     one, which is what the qsv elements' low-latency property does on the other
//     engine, so live delay does not depend on the capture backend.
//   - vbr: a ceiling above the target selects VBR, bursting to the maxrate.
//   - abr: the same VBR with the ceiling at twice the target. A VBR encode given no
//     ceiling leaves oneVPL its own, which is not a rate the settings ever stated.
//
// ICQ and QVBR stay out, though a quantizer with a bitrate beside it would select them:
// both take a quality level on a scale of their own rather than a quantizer, and ICQ is
// absent from the AV1 and VP9 encoders. The lookahead methods and VCM stay out as the
// modes they refine are not reachable without them. lossless has no QSV form at all
// (qsvGaps).
//
// B-pictures are pinned off rather than taken from the settings, as on AMF: the B-frame
// count is NVENC's alone, and a live screen stream pays their reorder delay for a gain it
// cannot spend. The preset is the builder's choice for the same reason, the settings'
// EncPreset being the NVENC p1-p7 ladder, which the form greys for this family.
func qsvArgs(s settings.Stream, r rates) []string {
	preset := qsvQualityPreset
	if s.Mode == "cbr" {
		preset = qsvLivePreset
	}
	base := []string{"-c:v", s.Codec, "-preset", preset, "-bf", "0"}
	switch s.Mode {
	case "crf":
		return append(base, "-q:v", r.cq)
	case "abr":
		return append(base, "-b:v", r.bitrate,
			"-maxrate", fmt.Sprintf("%dM", s.BitrateM*qsvAbrPeak))
	case "vbr":
		return append(base, "-b:v", r.bitrate, "-maxrate", r.maxrate,
			"-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs))
	case "cbr":
		return append(base, "-async_depth", qsvLiveAsyncDepth,
			"-b:v", r.bitrate, "-maxrate", r.bitrate,
			"-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
	default:
		assert.Never("unexpected rate-control mode", s.Mode)
		return nil
	}
}

// vulkanAbrPeak is the factor the abr mapping places its ceiling at above the bitrate
// target, the same derivation the VAAPI and AMF mappings use: a fixed-function encoder
// codes against a maximum either way, so an unbounded average means the same thing on
// all three hardware families.
const vulkanAbrPeak = 2

// The Vulkan tuning modes the rate-control modes encode under. A tuning mode is a hint
// about what the driver should optimize for, not a coding mode, so it sets the
// encoder's character while -rc_mode still decides the rate. cbr trades quality for the
// encoder keeping up with a live capture, as the NVENC preset ladder does at the same
// point. The lossless tuning mode stays out: it is a hint like the other two and
// quantizes all the same, which capabilities.Codecs declares as a gap (vulkanGaps).
const (
	vulkanLiveTune    = "ll"
	vulkanQualityTune = "hq"
)

// vulkanArgs maps the rate-control modes onto the Vulkan encoders' -rc_mode knob, the
// cross-vendor counterpart to vaapiArgs and amfArgs. It serves every Vulkan codec, the
// codec name itself being the only difference between them.
//   - crf: cqp, one fixed quantizer and no rate bound.
//   - cbr: cbr at the target, the VBV window sizing its rate buffer.
//   - vbr: vbr targeting the bitrate and bursting to the maxrate ceiling.
//   - abr: the same vbr with the ceiling at twice the target, since the encoders take
//     no unbounded average.
//
// usage and content are the encode extension's declarations about the stream: a live
// one whose pictures are screen content, which is what this app captures either way.
// No preset or B-frame count, matching the other hardware families: the p1-p7 ladder is
// NVENC's, and the settings' B-frame count is NVENC's too, so the reorder delay a live
// screen stream cannot spend stays off.
func vulkanArgs(s settings.Stream, r rates) []string {
	base := []string{"-c:v", s.Codec, "-usage", "stream", "-content", "desktop"}
	switch s.Mode {
	case "crf":
		return append(base, "-tune", vulkanQualityTune, "-rc_mode", "cqp", "-qp", r.cq)
	case "abr":
		return append(base, "-tune", vulkanQualityTune, "-rc_mode", "vbr",
			"-b:v", r.bitrate, "-maxrate", fmt.Sprintf("%dM", s.BitrateM*vulkanAbrPeak))
	case "vbr":
		return append(base, "-tune", vulkanQualityTune, "-rc_mode", "vbr",
			"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.MaxrateM, s.VbvMs))
	case "cbr":
		return append(base, "-tune", vulkanLiveTune, "-rc_mode", "cbr",
			"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
	default:
		assert.Never("unexpected rate-control mode", s.Mode)
		return nil
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
func amfArgs(profiles map[string]string, options func(rates) []string) encoderArgsFunc {
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
		case "cbr":
			return append(base, "-rc", "cbr", "-b:v", r.bitrate, "-maxrate", r.bitrate,
				"-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
		default:
			assert.Never("unexpected rate-control mode", s.Mode)
			return nil
		}
	}
}

// nvencLivePreset is the ladder step cbr pins the preset to, where a live stream
// trades quality for the encoder keeping up. The frontend's MODE_META declares the
// same step as the cbr row's pinnedPreset, which is what the form's greyed preset
// control names, so the two have to spell it alike.
const nvencLivePreset = "p5"

// nvencArgs maps the rate-control modes onto the NVENC SDK's knobs: preset ladder,
// tune, and rc mode. It serves every nvenc codec, the codec name itself being the
// only difference between them.
func nvencArgs(s settings.Stream, r rates) []string {
	preset := s.EncPreset
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
	case "cbr":
		// CBR pins the preset rather than honouring the settings'. The form greys the
		// preset control in this mode and says which preset is in force, so running a
		// preset the user can no longer change would make that sentence false: a
		// settings file or a preset carrying p7 would run p7 while the form reads p5.
		// Pinning is also what the mode is for, since a low-latency preset is what lets
		// the encoder hold a constant rate.
		preset = nvencLivePreset
		args := []string{"-c:v", s.Codec, "-preset", preset, "-tune", "ll", "-rc", "cbr", "-b:v", r.bitrate, "-bf", "0"}
		if s.VbvMs > 0 {
			args = append(args, "-bufsize", bufsizeArg(s.BitrateM, s.VbvMs))
		}
		return args
	default:
		assert.Never("unexpected rate-control mode", s.Mode)
		return nil
	}
}

// nvencPresets is the p1-p7 preset ladder, the values the encoder preset setting may
// hold. The frontend's ENC_PRESETS carries the same seven with their tooltips, the
// way every other option list is spelled on both sides of the wire.
var nvencPresets = []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7"}

// nvencPresetLimit rejects a preset outside the ladder.
//
// The field is a free-form string in the settings, and nothing upstream of here bounds
// it: capabilities.Validate covers the codec, pixel format, rate-control mode and the
// two rate figures, none of which this is. Passing an unknown value through would put
// it behind -preset for ffmpeg to reject, in a message about an option the form never
// showed rather than about the control that holds it.
func nvencPresetLimit(s settings.Stream) error {
	if slices.Contains(nvencPresets, s.EncPreset) {
		return nil
	}
	return fmt.Errorf("encoder preset %q is not one of the NVENC ladder steps %s",
		s.EncPreset, strings.Join(nvencPresets, ", "))
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
