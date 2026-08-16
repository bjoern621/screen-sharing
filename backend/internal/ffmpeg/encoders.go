package ffmpeg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// rates are one stream's rate-control figures, spelled as ffmpeg's command line takes them.
// The GOP length rides with them, in frames: amfArgs aligns a parameter-set repeat to it,
// and every other mapping leaves it to the command's own -g.
type rates struct {
	bitrate, maxrate, cq, bframes, gop string
}

func ratesFor(s settings.Settings, gop int) rates {
	return rates{
		bitrate: fmt.Sprintf("%dM", s.Publish.BitrateM),
		maxrate: fmt.Sprintf("%dM", s.Publish.MaxrateM),
		cq:      strconv.Itoa(s.Publish.Cq),
		bframes: strconv.Itoa(s.Publish.Bframes),
		gop:     strconv.Itoa(gop),
	}
}

// encoderArgsFunc builds the codec-specific half of one stream's command.
// l holds the two ladder steps resolved against the codec's own row, so a mapping names the option a
// step travels in and never the step itself.
type encoderArgsFunc func(s settings.Settings, r rates, l capabilities.Steps) []string

// tuneArgs is -tune for a resolved step, and nothing for a mode whose row declares no tune or whose
// step is the untuned one.
// ffmpeg spells "tune for nothing" as no -tune at all, which is how the encoders express it.
func tuneArgs(step string) []string {
	if step == "" || step == capabilities.TuneNone {
		return nil
	}
	return []string{"-tune", step}
}

// encoderMappings is the codec-specific half of the command, one entry per encoder whose knobs
// differ.
// The rate-control modes are the methods: cbr, vbr and abr target a bitrate and differ in the
// ceiling, crf targets a quality, lossless is bit-exact.
// gstCodecs expresses the same five on the GStreamer engine.
// A mode an encoder has no form of is a Gap in capabilities.Codecs and is rejected before a command
// is built, so no branch here approximates one.
//
// A family whose codecs share one mapping outright belongs in familyMappings, which is what keeps a
// codec added to such a family from needing a row here.
var encoderMappings = map[string]encoderArgsFunc{
	// x264 reaches bit-exact at -qp 0; x265 has no bit-exact qp and takes a parameter of its own.
	"libx264":    softwareArgs("libx264", []string{"-qp", "0"}),
	"libx265":    softwareArgs("libx265", []string{"-x265-params", "lossless=1"}),
	"libvpx-vp9": vp9Args,
	"libvpx":     vp8Args,
	"libaom-av1": aomArgs,
	"libsvtav1":  svtav1Args,
	"librav1e":   rav1eArgs,
	// One VAAPI mapping serves all five codecs, differing only in the option the quantizer travels in:
	// the H.26x encoders own a -qp, where the VP8, VP9 and AV1 ones ignore it and read ffmpeg's generic
	// -global_quality.
	"h264_vaapi": vaapiArgs("-qp"),
	"hevc_vaapi": vaapiArgs("-qp"),
	"av1_vaapi":  vaapiArgs("-global_quality"),
	"vp9_vaapi":  vaapiArgs("-global_quality"),
	"vp8_vaapi":  vaapiArgs("-global_quality"),
	// One QSV mapping serves all four codecs: the rate control is oneVPL's, so the codec decides only
	// which of its modes the silicon generation implements.
	"h264_qsv": qsvArgs,
	"hevc_qsv": qsvArgs,
	"av1_qsv":  qsvArgs,
	"vp9_qsv":  qsvArgs,
	// One AMF mapping serves all three codecs, with what differs bound here: the profile a chroma
	// selects, and the options only some of the three own.
	"h264_amf": amfArgs(nil, amfH264Options),
	"hevc_amf": amfArgs(amfHevcProfiles, nil),
	"av1_amf":  amfArgs(nil, amfNoBPictures),
	// One Vulkan mapping serves all three codecs: the options are the encode extension's, so the codec
	// decides only which of them the driver honours.
	"h264_vulkan": vulkanArgs,
	"hevc_vulkan": vulkanArgs,
	"av1_vulkan":  vulkanArgs,
}

// familyMappings is the same half of the command for the families whose codecs share one mapping
// outright, keyed as capabilities.Codecs names the family.
// The NVENC codecs differ only in the codec name the mapping reads off the settings, so the family
// is the unit.
// VAAPI and AMF are keyed per codec, what differs between their codecs (the option the quantizer
// travels in, the profile a chroma selects) being bound at the row.
var familyMappings = map[string]encoderArgsFunc{
	capabilities.FamilyNvenc:        nvencArgs,
	capabilities.FamilyVideoToolbox: videoToolboxArgs,
}

// encoderArgs returns the encoder half of the command for the configured codec and rate-control
// mode.
// gop is the resolved keyframe interval in frames, which the command also carries as -g.
// A codec's own row wins over its family's mapping.
//
// A codec neither table maps is refused rather than run on a bare -b:v guess.
// The families declared Implemented:false (v4l2, rkmpp) each need a device and a load path of their
// own, as VAAPI, QSV and Vulkan do (hwsurface.go), and none encodes from a command carrying only a
// bitrate.
// capabilities.Validate rejects those codecs ahead of this either way.
func encoderArgs(s settings.Settings, gop int) ([]string, error) {
	c, ok := capabilities.Get(s.Publish.Codec)
	if !ok {
		return nil, fmt.Errorf("unknown codec %q", s.Publish.Codec)
	}

	l, err := c.ResolveSteps(s.Publish.Mode, s.Publish.Effort, s.Publish.Tune)
	if err != nil {
		return nil, err
	}

	r := ratesFor(s, gop)
	if build, ok := encoderMappings[s.Publish.Codec]; ok {
		return build(s, r, l), nil
	}
	if build, ok := familyMappings[c.Family]; ok {
		return build(s, r, l), nil
	}
	return nil, fmt.Errorf("codec %q has no ffmpeg encoder mapping", s.Publish.Codec)
}

// softwareArgs is the rate-control mapping the CPU H.26x encoders libx264 and libx265 share,
// matching the software path of gstCodecs across all five modes.
// Only the encoder name and the lossless knob differ, and both are bound per codec in
// encoderMappings.
func softwareArgs(codec string, lossless []string) encoderArgsFunc {
	return func(s settings.Settings, r rates, l capabilities.Steps) []string {
		base := append([]string{"-c:v", codec, "-preset", l.Effort}, tuneArgs(l.Tune)...)
		switch s.Publish.Mode {
		case "crf":
			// A quality target held inside a ceiling where the settings state one: the rate factor drives
			// the rate until the VBV would overflow, and the picture softens from there rather than the
			// stream outgrowing the link.
			// Unbounded without one, which is what -crf alone is.
			return slices.Concat(base, []string{"-crf", r.cq}, qualityCeilingArgs(s, r))
		case "lossless":
			// No rate control: the frame costs what exactness costs, bursting to hundreds of Mbit/s.
			return append(base, lossless...)
		case "abr":
			// One-pass average with no VBV cap: quality holds and the rate bursts freely toward the target.
			return append(base, "-b:v", r.bitrate)
		case "vbr":
			// Constrained VBR: targets the bitrate, bursts to the maxrate ceiling on motion.
			// bufsize sizes that ceiling's VBV window.
			return append(base,
				"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs))
		case "cbr":
			// A maxrate at the bitrate over a bounded bufsize is true CBR.
			// -b:v alone is one-pass ABR and bursts past a capped link.
			return append(base,
				"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.Publish.BitrateM, s.Publish.VbvMs))
		default:
			assert.Never("unexpected rate-control mode", s.Publish.Mode)
			return nil
		}
	}
}

// qualityCeilingArgs is the ceiling a constant-quality encode is held to, and nothing where the
// encode is unbounded: no ceiling stated, or a codec whose constant-quality mode carries none on this
// engine (capabilities.QualityCeiling).
// One helper for every mapping that takes one, so the flag pair and the field it reads are one
// decision rather than one per codec.
func qualityCeilingArgs(s settings.Settings, r rates) []string {
	if s.Publish.MaxrateM <= 0 || !capabilities.QualityCeiling(s.Publish.Codec, capabilities.EngineFfmpeg) {
		return nil
	}
	return []string{"-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs)}
}

// aomRates appends the rate-control options the encoders built on Google's aom rate control share:
// libvpx (VP8 and VP9) and libaom (AV1).
// All three read the same generic ffmpeg knobs, so the base command and the lossless branch stay
// with the caller.
func aomRates(base []string, s settings.Settings, r rates) []string {
	switch s.Publish.Mode {
	case "crf":
		// These libraries bound a quality target with -b:v rather than with a VBV pair: zero leaves the
		// rate factor alone to drive the rate, and a figure there is libvpx's constrained quality, the
		// rate it codes toward and stays under.
		// -maxrate without a target is refused outright, so the ceiling travels here or nowhere.
		ceiling := "0"
		if s.Publish.MaxrateM > 0 && capabilities.QualityCeiling(s.Publish.Codec, capabilities.EngineFfmpeg) {
			ceiling = r.maxrate
		}
		return append(base, "-crf", r.cq, "-b:v", ceiling)
	case "abr":
		return append(base, "-b:v", r.bitrate)
	case "vbr":
		return append(base,
			"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs))
	case "cbr":
		// minrate, b:v and maxrate at one figure over a bounded buffer.
		return append(base,
			"-minrate", r.bitrate, "-b:v", r.bitrate, "-maxrate", r.bitrate,
			"-bufsize", bufsizeArg(s.Publish.BitrateM, s.Publish.VbvMs))
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// vp9Profiles is the VP9 profile that codes each configured chroma.
// VP9 splits its profiles by subsampling and bit depth and libvpx refuses a pixel format the
// selected profile cannot carry, which is what lets one codec row offer all four chromas.
// gbrp rides profile 1 through VP9's identity matrix, keeping RGB as RGB.
var vp9Profiles = map[string]string{
	"yuv420p": "0",
	"yuv444p": "1",
	"gbrp":    "1",
	"p010le":  "2",
}

// vp9Args maps the rate-control modes onto libvpx VP9.
// realtime, row-mt, cpu-used and tune-content=screen keep a live screen-content encode within a few
// cores.
// libvpx is the one software encoder here whose lossless mode is a dedicated flag rather than a
// quantizer of zero.
func vp9Args(s settings.Settings, r rates, l capabilities.Steps) []string {
	profile, known := vp9Profiles[s.Publish.Chroma]
	assert.Assert(known, "a VP9 chroma names the profile it encodes in", s.Publish.Chroma)

	base := slices.Concat([]string{
		"-c:v", "libvpx-vp9", "-profile:v", profile,
		"-deadline", "realtime", "-row-mt", "1", "-cpu-used", l.Effort,
		"-tune-content", "screen",
	}, tuneArgs(l.Tune))
	if s.Publish.Mode == "lossless" {
		return append(base, "-lossless", "1")
	}
	return aomRates(base, s, r)
}

// vp8Args maps the rate-control modes onto libvpx VP8.
// VP8 has one profile and no lossless mode, and libvpx's VP9-only threading and content tuning do
// not apply to it.
// screen-content-mode is VP8's own equivalent, turning on the coding tools for text and sharp edges.
// Its ladder runs further than VP9's and its row starts higher on it, VP8 having less to trade away.
func vp8Args(s settings.Settings, r rates, l capabilities.Steps) []string {
	return aomRates(slices.Concat([]string{
		"-c:v", "libvpx", "-deadline", "realtime", "-cpu-used", l.Effort,
		"-screen-content-mode", "1",
	}, tuneArgs(l.Tune)), s, r)
}

// aomArgs maps the rate-control modes onto libaom AV1.
// The realtime usage profile switches libaom off its two-pass defaults; without it a live encode
// falls minutes behind.
// row-mt and the tile split spread the frame over cores, and lag-in-frames 0 drops the lookahead
// that would hold frames back.
//
// tune-content=screen turns on the coding tools for text and sharp edges, as -screen-content-mode
// does on VP8 and -tune-content on VP9.
// It reaches libaom through -aom-params, ffmpeg's wrapper putting no option of its own on it.
func aomArgs(s settings.Settings, r rates, l capabilities.Steps) []string {
	return aomRates(slices.Concat([]string{
		"-c:v", "libaom-av1", "-usage", "realtime", "-cpu-used", l.Effort,
		"-row-mt", "1", "-tiles", "2x1", "-lag-in-frames", "0",
		"-aom-params", "tune-content=screen",
	}, tuneArgs(l.Tune)), s, r)
}

// svtav1Args maps the rate-control modes onto SVT-AV1, whose rate control does not take the aom
// shape:
//   - crf targets a quality with -crf and takes no bitrate.
//   - abr is the library's VBR mode, selected by -b:v alone and taking no -maxrate.
//     vbr has no form on either engine and is declared as a gap: SVT-AV1 accepts a rate
//     ceiling in constant-quality mode alone and rejects the whole encode when given one
//     in VBR.
//   - cbr is rate-control mode 2, reachable only through -svtav1-params, and it needs the
//     low-delay prediction structure. buf-sz sizes its rate buffer in ms, which the
//     element clamps to 20..10000.
//
// Everything the library takes rather than the wrapper travels in that one parameter string, so the
// rate-control half and the picture half are assembled together (svtav1Params).
func svtav1Args(s settings.Settings, r rates, l capabilities.Steps) []string {
	base := []string{"-c:v", "libsvtav1", "-preset", l.Effort}
	switch s.Publish.Mode {
	case "crf":
		return slices.Concat(base, []string{"-crf", r.cq}, svtav1Params(l))
	case "abr":
		return slices.Concat(base, []string{"-b:v", r.bitrate}, svtav1Params(l))
	case "cbr":
		rate := []string{"rc=2", "pred-struct=1"}
		if s.Publish.VbvMs > 0 {
			rate = append(rate, "buf-sz="+strconv.Itoa(s.Publish.VbvMs))
		}
		return slices.Concat(base, []string{"-b:v", r.bitrate}, svtav1Params(l, rate...))
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// svtav1Params assembles one -svtav1-params string out of the caller's rate-control keys and the
// picture keys every SVT-AV1 encode carries, and nothing at all where there is none to state.
//
// scm=1 turns on the screen-content tools for text and sharp edges, as -tune-content does on VP9.
// The library's own default detects content instead, which is a guess about a desktop this app
// already knows the answer for.
// The image-quality tune overrides it, and says so on a warning line of its own.
//
// The library refuses a key or a value it does not know rather than dropping it, so a step off the
// row's ladder ends the encode at launch instead of coding at a setting nobody asked for.
func svtav1Params(l capabilities.Steps, rate ...string) []string {
	params := append([]string{}, rate...)
	params = append(params, "scm=1")
	if tune, named := capabilities.Svtav1TuneValue(l.Tune); named {
		params = append(params, "tune="+tune)
	}
	return []string{"-svtav1-params", strings.Join(params, ":")}
}

// rav1eArgs maps the rate-control modes onto rav1e.
// Its rate control is one bitrate target with no ceiling and no rate buffer, so vbr is a gap on both
// engines, and cbr and abr differ only in whether frame reordering is dropped for delay.
// Its quantizer counts to 255 rather than 63.
func rav1eArgs(s settings.Settings, r rates, l capabilities.Steps) []string {
	base := []string{"-c:v", "librav1e", "-speed", l.Effort}
	switch s.Publish.Mode {
	case "crf":
		return slices.Concat(base, []string{"-qp", r.cq}, rav1eParams(l))
	case "abr":
		return slices.Concat(base, []string{"-b:v", r.bitrate}, rav1eParams(l))
	case "cbr":
		return slices.Concat(base, []string{"-b:v", r.bitrate}, rav1eParams(l, "low_latency=true"))
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// rav1eParams assembles one -rav1e-params string, the option carrying every rav1e setting ffmpeg's
// wrapper puts no flag of its own on.
// The rav1enc element takes the same two tunes on a property instead (publish/gstencoders.go), the
// step being rav1e's own either way.
func rav1eParams(l capabilities.Steps, rate ...string) []string {
	params := append([]string{}, rate...)
	if l.Tune != "" && l.Tune != capabilities.TuneNone {
		params = append(params, "tune="+l.Tune)
	}
	if len(params) == 0 {
		return nil
	}
	return []string{"-rav1e-params", strings.Join(params, ":")}
}

// videoToolboxArgs maps the one rate-control mode Apple's framework reaches onto its encoders, the
// counterpart to vtEncoder on the GStreamer path.
// It serves both codecs, the codec name being the only difference between them.
//
// abr is all there is, which is what the rows' gaps state: -b:v is the average the framework codes
// towards, and the three modes that would bound or replace it have no form here.
//
// realtime is the framework's own answer to an effort ladder, and it is set rather than offered: it
// is what stops the encoder buffering frames to spend more time on them, which a live stream cannot
// pay whatever else it asks for.
// B-pictures are pinned off beside it, as on QSV and AMF: the framework turns its frame reordering
// off with the count, and the reorder delay is one a screen share cannot spend.
func videoToolboxArgs(s settings.Settings, r rates, _ capabilities.Steps) []string {
	assert.Assert(s.Publish.Mode == capabilities.ModeAbr,
		"a VideoToolbox encode runs the one rate control its rows declare", s.Publish.Mode)

	return []string{"-c:v", s.Publish.Codec, "-realtime", "1", "-bf", "0", "-b:v", r.bitrate}
}

// vaapiArgs maps the rate-control modes onto the VAAPI encoders' shared -rc_mode knob, the
// counterpart to vaEncoder on the GStreamer path.
// quantizer is the option the constant-quality target travels in, bound per codec in
// encoderMappings.
//   - crf: CQP, one fixed quantizer and no rate bound.
//   - cbr: CBR at the target, the VBV window sizing its coded-picture buffer.
//   - vbr: VBR targeting the bitrate and bursting to the maxrate ceiling.
//   - abr: VBR with no ceiling given, which ffmpeg turns into a ceiling of twice the
//     target. VAAPI rate control always codes against a maximum, so that is the closest
//     it comes to an unbounded average.
//
// CQP, CBR and VBR are the three modes both vendors' drivers implement.
// ffmpeg also offers ICQ, QVBR and AVBR, which are Intel-only or absent, and a driver handed a mode
// it lacks refuses to open the encoder rather than falling back.
// lossless has no VAAPI form at all (vaapiGaps).
//
// No effort step and no B-frame count: the family's rows declare no ladder, and VAAPI B-frame
// support varies per driver and hardware generation, so the form greys both fields.
func vaapiArgs(quantizer string) encoderArgsFunc {
	return func(s settings.Settings, r rates, _ capabilities.Steps) []string {
		switch s.Publish.Mode {
		case "crf":
			return []string{"-c:v", s.Publish.Codec, "-rc_mode", "CQP", quantizer, r.cq}
		case "abr":
			return []string{"-c:v", s.Publish.Codec, "-rc_mode", "VBR", "-b:v", r.bitrate}
		case "vbr":
			return []string{
				"-c:v", s.Publish.Codec, "-rc_mode", "VBR",
				"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs),
			}
		case "cbr":
			return []string{
				"-c:v", s.Publish.Codec, "-rc_mode", "CBR",
				"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.Publish.BitrateM, s.Publish.VbvMs),
			}
		default:
			assert.Never("unexpected rate-control mode", s.Publish.Mode)
			return nil
		}
	}
}

// The two ends of oneVPL's target-usage scale the ladder's defaults sit at, spelled as ffmpeg names
// them: the balanced point mid-scale, and the speed end a live rate is held at.
const (
	qsvLivePreset    = "veryfast"
	qsvQualityPreset = "medium"
)

// qsvPresets is this engine's spelling of oneVPL's target-usage scale.
//
// The ladder is the scale's own numbers, which is what oneVPL defines and what the GStreamer
// elements take on their target-usage property; ffmpeg names the same seven points, and this is
// where the one becomes the other.
// A step with no name here is one this engine cannot ask for, and it passes no preset at all rather
// than a nearby one: the encoder's own default is an honest answer where a substituted step is a
// silent one.
var qsvPresets = map[string]string{
	"1": "veryslow",
	"2": "slower",
	"3": "slow",
	"4": qsvQualityPreset,
	"5": "fast",
	"6": "faster",
	"7": qsvLivePreset,
}

// qsvAbrPeak is where the abr mapping places its ceiling, as a multiple of the bitrate target.
// The VAAPI, AMF and Vulkan mappings derive theirs the same way.
const qsvAbrPeak = 2

// qsvLiveAsyncDepth is how many frames cbr lets the encoder keep in flight.
// oneVPL pipelines four by default, three frame periods of delay a live stream pays for throughput
// it does not need at a screen's frame rate.
const qsvLiveAsyncDepth = "1"

// qsvArgs maps the rate-control modes onto the QSV encoders' options, the Intel counterpart to
// vaapiArgs and amfArgs.
// It serves every QSV codec, the codec name being the only difference between them.
//
// This family has no -rc_mode to set: oneVPL derives the method from which rate options carry a
// value, so each branch below is the shape that names one method.
//   - crf: -q:v carries the quantizer and sets ffmpeg's qscale flag with it, which selects
//     CQP. The value is the quantizer itself rather than a quality level, and ffmpeg states
//     it on the H.26x 0..51 scale for every codec but AV1, which capabilities.Codecs carries
//     as each row's CqMax.
//   - cbr: a ceiling equal to the target selects CBR, the VBV window sizing its rate buffer.
//     async_depth drops the encoder's pipeline from four frames in flight to one, which is
//     what the qsv elements' low-latency property does on the other engine, so live delay
//     does not follow from the capture backend.
//   - vbr: a ceiling above the target selects VBR, bursting to the maxrate.
//   - abr: the same VBR with the ceiling at twice the target. A VBR encode given no ceiling
//     leaves oneVPL its own, which is not a rate the settings ever stated.
//
// ICQ and QVBR stay out, though a quantizer with a bitrate beside it would select them: both take a
// quality level on a scale of their own rather than a quantizer, and ICQ is absent from the AV1 and
// VP9 encoders.
// The lookahead methods and VCM stay out, the modes they refine not being reachable without them.
// lossless has no QSV form at all (qsvGaps).
//
// B-pictures are pinned off rather than taken from the settings, as on AMF: the B-frame count is
// NVENC's alone, and a live screen stream pays their reorder delay for a gain it cannot spend.
//
// The tune step is oneVPL's scenario hint, spelled on -scenario rather than on -tune.
// It reaches the H.264 and HEVC encoders, the two rows declaring the ladder, so the AV1 and VP9 ones
// resolve an empty step and state nothing.
func qsvArgs(s settings.Settings, r rates, l capabilities.Steps) []string {
	base := []string{"-c:v", s.Publish.Codec, "-bf", "0"}
	if preset, named := qsvPresets[l.Effort]; named {
		base = append(base, "-preset", preset)
	}
	if l.Tune != "" && l.Tune != capabilities.TuneNone {
		base = append(base, "-scenario", l.Tune)
	}
	switch s.Publish.Mode {
	case "crf":
		return append(base, "-q:v", r.cq)
	case "abr":
		return append(base, "-b:v", r.bitrate,
			"-maxrate", fmt.Sprintf("%dM", s.Publish.BitrateM*qsvAbrPeak))
	case "vbr":
		return append(base, "-b:v", r.bitrate, "-maxrate", r.maxrate,
			"-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs))
	case "cbr":
		return append(base, "-async_depth", qsvLiveAsyncDepth,
			"-b:v", r.bitrate, "-maxrate", r.bitrate,
			"-bufsize", bufsizeArg(s.Publish.BitrateM, s.Publish.VbvMs))
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// vulkanAbrPeak is where the abr mapping places its ceiling, as a multiple of the bitrate target,
// as on VAAPI and AMF: a fixed-function encoder codes against a maximum either way, so an unbounded
// average means the same thing on all three hardware families.
const vulkanAbrPeak = 2

// vulkanArgs maps the rate-control modes onto the Vulkan encoders' -rc_mode knob, the cross-vendor
// counterpart to vaapiArgs and amfArgs.
// It serves every Vulkan codec, the codec name being the only difference between them.
//   - crf: cqp, one fixed quantizer and no rate bound.
//   - cbr: cbr at the target, the VBV window sizing its rate buffer.
//   - vbr: vbr targeting the bitrate and bursting to the maxrate ceiling.
//   - abr: the same vbr with the ceiling at twice the target, the encoders taking no
//     unbounded average.
//
// usage and content are the encode extension's declarations about the stream: a live one whose
// pictures are screen content, which is what this app captures either way.
// The tune comes off the row's ladder.
// No effort step and no B-frame count, matching the other hardware families: the rows declare no
// effort ladder, and the settings' B-frame count is NVENC's, so the reorder delay a live screen
// stream cannot spend stays off.
func vulkanArgs(s settings.Settings, r rates, l capabilities.Steps) []string {
	base := append([]string{"-c:v", s.Publish.Codec, "-usage", "stream", "-content", "desktop"},
		tuneArgs(l.Tune)...)
	switch s.Publish.Mode {
	case "crf":
		return append(base, "-rc_mode", "cqp", "-qp", r.cq)
	case "abr":
		return append(base, "-rc_mode", "vbr",
			"-b:v", r.bitrate, "-maxrate", fmt.Sprintf("%dM", s.Publish.BitrateM*vulkanAbrPeak))
	case "vbr":
		return append(base, "-rc_mode", "vbr",
			"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs))
	case "cbr":
		return append(base, "-rc_mode", "cbr",
			"-b:v", r.bitrate, "-maxrate", r.bitrate, "-bufsize", bufsizeArg(s.Publish.BitrateM, s.Publish.VbvMs))
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// amfHevcProfiles is the HEVC profile indicating each configured chroma's bit depth.
// AMF leaves the profile indication at Main whatever the input surface carries, so a p010le encode
// comes out as a 10-bit bitstream announcing an 8-bit profile, which a decoder gating on the profile
// is entitled to refuse.
// AMF's H.264 encoder needs no such map, its row being 8-bit, and neither does its AV1 one, where
// profile 0 carries both depths.
var amfHevcProfiles = map[string]string{
	"yuv420p": "main",
	"p010le":  "main10",
}

// amfUsage is the AMF usage preset every mode encodes under.
// A usage preset is a bundle of AMD defaults applied ahead of the rate-control properties, so it
// sets the encoder's character while -rc decides the rate.
//
// The same preset in all four modes, cbr included, because AMF's low-latency presets are unusable
// for a stream a viewer joins late: under lowlatency and ultralowlatency its H.264 encoder drops the
// IDR period and codes one IDR for the whole stream, leaving a later subscriber no recovery point
// and a decoder that never starts.
// The keyframe interval the settings ask for is the stronger claim, so the preset gives way, and cbr
// states its low-delay character through the speed end of the quality scale and the B-pictures the
// mappings pin off.
// transcoding is also the one preset every VCN generation implements; high_quality is refused by the
// older ones.
const amfUsage = "transcoding"

// amfAbrPeak is where the abr mapping places its peak ceiling, as a multiple of the bitrate target,
// ffmpeg's own derivation for a hardware VBR encode given none.
const amfAbrPeak = 2

// amfH264Options are the options AMD's H.264 encoder needs beyond the shared mapping.
// header_spacing repeats SPS and PPS every GOP, so they arrive with each IDR; written once at stream
// start otherwise, they leave a viewer who subscribes later with no parameter sets to configure its
// decoder from.
// AMD's HEVC encoder repeats VPS, SPS and PPS per IDR on its own, and AV1 carries no out-of-band
// parameter sets at all, so this is the one row that needs telling.
func amfH264Options(r rates) []string {
	return append(amfNoBPictures(r), "-header_spacing", r.gop)
}

// amfNoBPictures switches off the B-picture pattern of the two AMF encoders that have one, rather
// than leaving it to the usage preset: the settings' B-frame count is NVENC's alone, and a live
// screen stream pays their reorder delay for a gain it cannot spend.
// AMD's HEVC encoder codes no B-frames at all and has no such option.
func amfNoBPictures(rates) []string {
	return []string{"-bf", "0"}
}

// amfArgs maps the rate-control modes onto the AMF encoders' -rc knob, the AMD counterpart to
// vaapiArgs.
// profiles selects the profile a chroma needs, nil where the codec has one profile for every chroma
// it codes; options adds what only some of the three encoders own.
//   - crf: cqp, a fixed quantizer per frame type and no rate bound.
//   - cbr: cbr at the target, the VBV window sizing the rate buffer.
//   - vbr: peak-constrained VBR, targeting the bitrate and bursting to the maxrate ceiling.
//   - abr: the same peak-constrained VBR with the ceiling at twice the target, which is what
//     ffmpeg derives for a VAAPI encode given no ceiling, so an unbounded average means the
//     same thing on both hardware families. AMF codes against a peak either way, and left
//     unset it keeps the usage preset's own, which is not a rate the settings ever stated.
//
// AMF's remaining rate-control modes stay out: qvbr targets a quality level on a scale of its own,
// and hqvbr and hqcbr are the pre-analysis variants the older VCN generations do not implement.
// lossless has no AMF form at all (amfGaps).
func amfArgs(profiles map[string]string, options func(rates) []string) encoderArgsFunc {
	return func(s settings.Settings, r rates, l capabilities.Steps) []string {
		base := []string{"-c:v", s.Publish.Codec}
		if profile, ok := profiles[s.Publish.Chroma]; ok {
			base = append(base, "-profile", profile)
		}
		if options != nil {
			base = append(base, options(r)...)
		}
		base = append(base, "-usage", amfUsage)
		// The step reaches the encoder verbatim: all three AMF encoders spell the scale alike, so there is
		// nothing to map here the way oneVPL's numbers need mapping (qsvPresets).
		if l.Effort != "" {
			base = append(base, "-quality", l.Effort)
		}
		switch s.Publish.Mode {
		case "crf":
			return append(base, "-rc", "cqp", "-qp_i", r.cq, "-qp_p", r.cq)
		case "abr":
			return append(base, "-rc", "vbr_peak", "-b:v", r.bitrate,
				"-maxrate", fmt.Sprintf("%dM", s.Publish.BitrateM*amfAbrPeak))
		case "vbr":
			return append(base, "-rc", "vbr_peak", "-b:v", r.bitrate, "-maxrate", r.maxrate,
				"-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs))
		case "cbr":
			return append(base, "-rc", "cbr", "-b:v", r.bitrate, "-maxrate", r.bitrate,
				"-bufsize", bufsizeArg(s.Publish.BitrateM, s.Publish.VbvMs))
		default:
			assert.Never("unexpected rate-control mode", s.Publish.Mode)
			return nil
		}
	}
}

// nvencArgs maps the rate-control modes onto the NVENC SDK's knobs: the preset ladder, the tune and
// the rc mode.
// It serves every nvenc codec, the codec name being the only difference between them.
//
// Both ladder steps come off the row in every mode, the pinned one included.
// cbr runs the low-latency preset the row pins rather than the settings' step, which is what lets
// the encoder hold a constant rate; the pin is also what greys the control and names the step in
// force, so a settings file carrying p7 cannot run p7 while the form reads what the row declares.
func nvencArgs(s settings.Settings, r rates, l capabilities.Steps) []string {
	preset := []string{"-preset", l.Effort}
	tune := tuneArgs(l.Tune)
	switch s.Publish.Mode {
	case "lossless":
		// True nvenc lossless: no rate control, the frame costs what exactness costs and can burst well
		// past 1 Gbps.
		// B-frames pinned off rather than taken from the settings, bit-exact coding gaining nothing from
		// them, which is what the UI greys the field for.
		return slices.Concat([]string{"-c:v", s.Publish.Codec}, preset, tune, []string{"-bf", "0"})
	case "crf":
		// VBR against a constant quantizer: cq drives the look and the ceiling only caps bursts.
		// -b:v 0 is what keeps the target out of it, the rate belonging to the quantizer.
		// multipass fullres spends the most effort per bit.
		return slices.Concat([]string{"-c:v", s.Publish.Codec}, preset, tune, []string{
			"-multipass", "fullres",
			"-rc", "vbr", "-cq", r.cq, "-b:v", "0",
		}, qualityCeilingArgs(s, r), []string{"-bf", r.bframes})
	case "abr":
		// VBR toward an average, no ceiling.
		return slices.Concat([]string{"-c:v", s.Publish.Codec}, preset, tune,
			[]string{"-rc", "vbr", "-b:v", r.bitrate, "-bf", r.bframes})
	case "vbr":
		return slices.Concat([]string{"-c:v", s.Publish.Codec}, preset, tune, []string{
			"-rc", "vbr",
			"-b:v", r.bitrate, "-maxrate", r.maxrate, "-bufsize", bufsizeArg(s.Publish.MaxrateM, s.Publish.VbvMs),
			"-bf", r.bframes,
		})
	case "cbr":
		args := slices.Concat([]string{"-c:v", s.Publish.Codec}, preset, tune,
			[]string{"-rc", "cbr", "-b:v", r.bitrate, "-bf", "0"})
		if s.Publish.VbvMs > 0 {
			args = append(args, "-bufsize", bufsizeArg(s.Publish.BitrateM, s.Publish.VbvMs))
		}
		return args
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// bufsizeArg is the ffmpeg -bufsize value in kbit for a rate in Mbit/s held over a VBV window.
// rateM Mbit/s over ms milliseconds is rateM*ms kbit.
// A window of zero takes one second, the conventional CBR buffer.
func bufsizeArg(rateM, vbvMs int) string {
	ms := vbvMs
	if ms <= 0 {
		ms = 1000
	}
	return strconv.Itoa(rateM*ms) + "k"
}
