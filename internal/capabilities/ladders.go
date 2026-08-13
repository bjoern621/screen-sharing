package capabilities

import (
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"
)

// The effort and tune ladders, per encoder family.
//
// Every step was read off the encoder: the named ladders by encoding one frame at each step,
// the numeric ranges out of the encoders' own option help.
// A step this table declares and the encoder refuses is a publish that dies at launch.
//
// The per-mode defaults are the constants both argument builders spend.
// They live here because a speed decision written into two switch statements drifts between the
// engines.
//
// VAAPI declares no ladder, and the absence is stated rather than implied: neither engine's VAAPI
// path has such a knob, so the form greys the control and names the codec.
// The QSV and AMF ladders are the ranges those two APIs document rather than a reading taken off
// silicon that is not here, and the ladder tests hold both builders to them.

// x264Presets is the ladder libx264 and libx265 share, most effort first.
// ffmpeg spells the knob -preset and the GStreamer elements speed-preset, with x264's own values on
// both.
var x264Presets = []string{
	"placebo", "veryslow", "slower", "slow", "medium",
	"fast", "faster", "veryfast", "superfast", "ultrafast",
}

// x264PresetDefaults is where each mode starts on that ladder.
// The two modes holding a live delay take a fast step, because the effort a slower preset spends
// goes into lookahead and frame reordering a live encode cannot wait for.
var x264PresetDefaults = map[string]string{
	ModeCrf:      "slow",
	ModeAbr:      "medium",
	ModeVbr:      "medium",
	ModeCbr:      "veryfast",
	ModeLossless: "veryfast",
}

// x264Tunes is what libx264 optimizes for, untuned step first.
var x264Tunes = []string{
	TuneNone, "film", "animation", "grain", "stillimage", "psnr", "ssim", "fastdecode", "zerolatency",
}

// x265Tunes is the same knob on libx265, which has no film and no stillimage tune.
var x265Tunes = []string{TuneNone, "psnr", "ssim", "grain", "fastdecode", "animation", "zerolatency"}

// h26xTuneDefaults is the tune each mode starts on for the two software H.26x encoders.
//
// The untuned modes name TuneNone rather than being left out of the map.
// A mode left out leaves the encoder's own default standing, and the two engines disagree on it:
// x265 defaults to no tune where the x265enc element's property starts at ssim,
// so an unstated mode would make a stream's look follow the capture backend that produced it.
var h26xTuneDefaults = map[string]string{
	ModeCrf:      TuneNone,
	ModeAbr:      TuneNone,
	ModeVbr:      TuneNone,
	ModeCbr:      "zerolatency",
	ModeLossless: "zerolatency",
}

// nvencPresets is NVIDIA's preset ladder, most effort first: p7 is the slowest step and p1 the
// fastest, so the list counts down.
var nvencPresets = []string{"p7", "p6", "p5", "p4", "p3", "p2", "p1"}

// nvencPresetDefaults is where each mode starts on that ladder.
//
// CBR pins its step instead of starting on it, since a low-latency preset is what lets NVENC hold a
// constant rate, and the form greys the control and names the step in force.
// A row claiming p7 there would make that sentence name a step the encode never spends.
var nvencPresetDefaults = map[string]string{
	ModeCrf:      "p7",
	ModeAbr:      "p7",
	ModeVbr:      "p7",
	ModeLossless: "p7",
	ModeCbr:      "p5",
}

// nvencTunes are the encoder's tuning info values.
// Lossless is a tune on NVENC and not an effort step, which is why the bit-exact mode is served off
// this ladder.
var nvencTunes = []string{"hq", "ll", "ull", "lossless"}

var nvencTuneDefaults = map[string]string{
	ModeCrf:      "hq",
	ModeAbr:      "hq",
	ModeVbr:      "hq",
	ModeCbr:      "ll",
	ModeLossless: "lossless",
}

// vulkanTunes are the two tuning modes ffmpeg's Vulkan encoders take.
// The API's lossless tuning mode is left out: it is a hint about what to optimize for and not a
// coding mode (vulkanGaps).
var vulkanTunes = []string{"hq", "ll"}

var vulkanTuneDefaults = map[string]string{
	ModeCrf: "hq", ModeAbr: "hq", ModeVbr: "hq", ModeCbr: "ll",
}

// steps builds a numeric ladder from most effort to least, both ends included.
// Every numeric ladder here counts the same way round, a lower number meaning more work,
// which is the convention aom, SVT-AV1 and rav1e share.
func steps(hardest, easiest int) []string {
	assert.Assert(hardest <= easiest, "a ladder runs from its hardest step to its easiest", hardest, easiest)

	out := make([]string, 0, easiest-hardest+1)
	for n := hardest; n <= easiest; n++ {
		out = append(out, strconv.Itoa(n))
	}

	assert.Assert(len(out) == easiest-hardest+1, "a ladder carries every step between its ends", len(out))
	return out
}

// The numeric ladders, each narrowed to the steps a live screen encode reaches.
// libvpx counts its cpu-used from -16 (VP8) or -8 (VP9) and libaom from 0: the negative half belongs
// to the deadlines this pipeline does not use, both capture chains pinning the realtime one.
// SVT-AV1's own range starts at -2, which are tuning presets rather than quality steps.
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

// qsvTargetUsages is oneVPL's target-usage scale, 1 at the quality end and 7 at the speed one.
// Stated as the numbers because the numbers are oneVPL's own and the names are one engine's
// spelling: the GStreamer elements take the figure on their target-usage property,
// and the ffmpeg builder maps it onto -preset (ffmpeg/encoders.go, qsvPresets).
var qsvTargetUsages = steps(1, 7)

var qsvTargetUsageDefaults = map[string]string{
	ModeCrf:      "4",
	ModeAbr:      "4",
	ModeVbr:      "4",
	ModeCbr:      "7",
	ModeLossless: "4",
}

// amfPresets is AMD's quality preset, most effort first.
// The higher preset newer VCN generations add is left out for the reason amfUsage names in the
// builder: the older generations refuse it, and a step that fails on half the hardware in the field
// is a publish that dies at launch.
var amfPresets = []string{"quality", "balanced", "speed"}

var amfPresetDefaults = map[string]string{
	ModeCrf: "quality",
	ModeAbr: "quality",
	ModeVbr: "quality",
	ModeCbr: "speed",
}

// TuneNone is the settings value for an encode that is not tuned.
//
// It is a step of the tune ladders rather than an absence, for the reason platform.AudioSourceNone
// is a row of the source table: a control needs a value to hold, and a declared one is a value every
// consumer can name.
// Both builders answer it by passing no tune option at all.
const TuneNone = "none"
