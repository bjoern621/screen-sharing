package capabilities

import "strconv"

// The effort and tune ladders, per encoder family.
//
// Every step here was read off the encoder rather than remembered: the named ladders were
// checked by encoding one frame at each step, and the numeric ranges come from the
// encoders' own option help. A step the table declares and the encoder refuses is a
// publish that dies at launch, which is exactly what a table exists to prevent.
//
// The values each mode starts on are the constants the argument builders spend today.
// They are stated here so the two builders stop being the place a speed decision lives:
// what a mode is worth running at is a fact about the encoder, and a fact written into
// two switch statements is one that drifts between engines.
//
// VAAPI declares no ladder, and its absence is stated rather than implied: neither engine's
// VAAPI path has such a knob at all, so the form greys the control there and names the codec.
//
// The QSV and AMF ladders below are the two encoders' own documented scales rather than a
// reading taken off silicon that is not here. Each is stated as the range its API defines and
// each engine's spelling of it is the builder's, which is what the equivalence test holds: a
// step this table declares and an element refuses is a publish that dies at launch, so the
// ladders are the ranges the vendors publish and nothing wider.

// x264Presets is the ladder libx264 and libx265 share, most effort first.
//
// The same ten names reach both engines: ffmpeg spells the knob -preset and the GStreamer
// elements spell it speed-preset, and the values are x264's own either way.
var x264Presets = []string{
	"placebo", "veryslow", "slower", "slow", "medium",
	"fast", "faster", "veryfast", "superfast", "ultrafast",
}

// x264PresetDefaults is where each mode starts on that ladder.
//
// The three points are the trade each mode is already making. Constant quality holds
// quality and can afford to work at it; the two bursting modes sit in the middle; and the
// two that hold a live delay take the fast step, because the effort a slower preset spends
// goes into lookahead and reordering that a live encode cannot wait for.
var x264PresetDefaults = map[string]string{
	ModeCrf:      "slow",
	ModeAbr:      "medium",
	ModeVbr:      "medium",
	ModeCbr:      "veryfast",
	ModeLossless: "veryfast",
}

// x264Tunes is what libx264 optimizes for. "none" is the settings value for leaving the
// knob unset, which is how x264 itself expresses no tune, and it leads the ladder because
// it is what most encodes want.
var x264Tunes = []string{
	TuneNone, "film", "animation", "grain", "stillimage", "psnr", "ssim", "fastdecode", "zerolatency",
}

// x265Tunes is the same knob on libx265, which implements a shorter list: it has no film
// and no stillimage tune.
var x265Tunes = []string{TuneNone, "psnr", "ssim", "grain", "fastdecode", "animation", "zerolatency"}

// h26xTuneDefaults is the tune each mode starts on for the two software H.26x encoders.
// The two modes that hold a live delay ask for zerolatency, which drops the B-frames and
// the lookahead; the rest tune for nothing.
//
// The three that tune for nothing say so with the ladder's own untuned step rather than by
// leaving the mode out. A mode left out means the encoder's default stands, and the two
// engines have different ones here: x265's own default is no tune where the x265enc
// element's property starts at ssim, so an unstated mode would make a stream's look follow
// the capture backend that produced it.
var h26xTuneDefaults = map[string]string{
	ModeCrf:      TuneNone,
	ModeAbr:      TuneNone,
	ModeVbr:      TuneNone,
	ModeCbr:      "zerolatency",
	ModeLossless: "zerolatency",
}

// nvencPresets is NVIDIA's p1-p7 ladder, most effort first. p1 is the fastest step and p7
// the slowest, so the list counts down.
var nvencPresets = []string{"p7", "p6", "p5", "p4", "p3", "p2", "p1"}

// nvencPresetDefaults is where each mode starts on that ladder.
//
// Four of the five start on the slowest step, which is the settings default and what the
// builder forwards. CBR does not: the mode pins the preset to a low-latency step, because
// holding a constant rate is what a low-latency preset is for, and the form greys the
// control there and says which step is in force. A row claiming the mode starts on p7
// would make that sentence false the moment anything read this table instead of the
// builder.
var nvencPresetDefaults = map[string]string{
	ModeCrf:      "p7",
	ModeAbr:      "p7",
	ModeVbr:      "p7",
	ModeLossless: "p7",
	ModeCbr:      "p5",
}

// nvencTunes are the encoder's tuning info values. Lossless is a tune rather than a step,
// which is why the mode that codes bit-exact takes it here instead of on the ladder above.
var nvencTunes = []string{"hq", "ll", "ull", "lossless"}

// nvencTuneDefaults is what each mode tunes for: quality where the encode is aiming at
// one, low latency where it is holding a rate, and the lossless tune where the mode is
// bit-exact.
var nvencTuneDefaults = map[string]string{
	ModeCrf:      "hq",
	ModeAbr:      "hq",
	ModeVbr:      "hq",
	ModeCbr:      "ll",
	ModeLossless: "lossless",
}

// vulkanTunes are the two tuning modes ffmpeg's Vulkan encoders take. The API's lossless
// tuning mode is deliberately absent: it is a hint about what to optimize for rather than
// a coding mode, and the row's own gap says the family codes nothing bit-exact.
var vulkanTunes = []string{"hq", "ll"}

// vulkanTuneDefaults follows the same split the NVENC one does.
var vulkanTuneDefaults = map[string]string{
	ModeCrf: "hq", ModeAbr: "hq", ModeVbr: "hq", ModeCbr: "ll",
}

// steps is a numeric ladder from most effort to least, both ends included.
//
// Every numeric ladder here counts the same way round: a lower number means the encoder
// works harder. That is the aom family's convention and SVT-AV1's and rav1e's, so the
// ladders are generated from it rather than each being spelled out.
func steps(hardest, easiest int) []string {
	out := make([]string, 0, easiest-hardest+1)
	for n := hardest; n <= easiest; n++ {
		out = append(out, strconv.Itoa(n))
	}
	return out
}

// The numeric ladders, with what each encoder's option help states as its range.
//
// Each is narrowed to the steps a live screen encode can reach, and the narrowing is
// stated rather than assumed. libvpx counts its cpu-used from -16 (VP8) or -8 (VP9), and
// libaom from 0; the negative half belongs to the deadlines this pipeline does not use,
// since both capture chains pin the realtime deadline. SVT-AV1's own range starts at -2,
// which are its tuning presets rather than quality ones.
var (
	vp8Speeds    = steps(0, 16)
	vp9Speeds    = steps(0, 8)
	aomSpeeds    = steps(0, 8)
	svtav1Steps  = steps(0, 13)
	rav1eSpeeds  = steps(0, 10)
	vp8Default   = everyMode("8")
	vp9Default   = everyMode("6")
	aomDefault   = everyMode("8")
	svtav1Preset = everyMode("9")
	rav1eDefault = everyMode("10")
)

// qsvTargetUsages is oneVPL's target-usage scale, most effort first.
//
// The scale is the API's own and runs 1 to 7, where 1 is the quality end and 7 the speed one,
// which is the same direction every numeric ladder here counts in. It is stated as the
// numbers rather than as ffmpeg's names for them, because the numbers are what oneVPL
// defines and the names are one engine's spelling: the GStreamer elements take the figure
// directly on their target-usage property, and the ffmpeg builder maps it onto -preset
// (ffmpeg/encoders.go, qsvPresets).
var qsvTargetUsages = steps(1, 7)

// qsvTargetUsageDefaults is where each mode starts on that scale.
//
// The two points are the trade the builders already made: the middle of the scale for the
// modes aiming at a quality or a rate, and the speed end for the one holding a live rate,
// which is the same split NVENC's preset ladder makes at the same place.
var qsvTargetUsageDefaults = map[string]string{
	ModeCrf:      "4",
	ModeAbr:      "4",
	ModeVbr:      "4",
	ModeCbr:      "7",
	ModeLossless: "4",
}

// amfPresets is AMD's quality preset, most effort first.
//
// Three steps and not more: quality, balanced and speed are what every AMF encoder in the
// table implements and what both this app's encoders spell alike. The higher preset newer
// VCN generations add is deliberately absent, for the reason amfUsage names in the builder -
// the older generations refuse it, and a ladder step that fails on the hardware half the
// users have is a publish that dies at launch.
var amfPresets = []string{"quality", "balanced", "speed"}

// amfPresetDefaults is where each mode starts, the same split every hardware family here
// makes: the quality end where the encode is aiming at one, the speed end where it is
// holding a live rate.
var amfPresetDefaults = map[string]string{
	ModeCrf: "quality",
	ModeAbr: "quality",
	ModeVbr: "quality",
	ModeCbr: "speed",
}

// TuneNone is the settings value for an encode that is not tuned.
//
// It is a step of the tune ladders rather than an absence, for the reason
// platform.AudioSourceNone is a row of the source table: a control needs something to
// hold, and a value the table declares is one every consumer can name. What the builders
// do with it is pass no tune flag at all, which is how the encoders themselves express it.
const TuneNone = "none"
