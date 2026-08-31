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
// They live here, a speed decision written into two switch statements drifting between the engines.
//
// The VAAPI rows declare neither ladder, and each absence has its own reason.
// No VA profile carries a tuning hint, so there is nothing to aim at on either engine.
// The effort knob does exist on both, and its scale is not one number:
// the va elements take target-usage over 1..7,
// where ffmpeg's -quality runs over the range the driver reports,
// 0..32 on Mesa's radeonsi against oneVPL's 1..7 on Intel's.
// One ladder over two scales would spend a different step per engine and per card,
// which is the stream's look following the capture backend,
// so the control is greyed and names the codec.
//
// The AMF rows declare an effort ladder and no tune, though the API has a usage hint:
// this app pins it,
// a low-latency usage dropping the IDR period and leaving a late subscriber no recovery point
// (ffmpeg/encoders.go, amfUsage).
// The QSV, AMF and NVENC ladders are the ranges those APIs document,
// read off the shipped encoder's own option table rather than off silicon that is not here,
// and the ladder tests hold both builders to them.

// x264Presets is the ladder libx264 and libx265 share, most effort first.
// ffmpeg spells the knob -preset and the GStreamer elements speed-preset,
// with x264's own values on both.
var x264Presets = []string{
	"placebo", "veryslow", "slower", "slow", "medium",
	"fast", "faster", "veryfast", "superfast", "ultrafast",
}

// x264PresetDefaults is where each mode starts on that ladder.
// The two modes holding a live delay take a fast step:
// a slower preset spends its effort on lookahead and frame reordering a live encode cannot wait for.
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

// vpxTunes is the metric libvpx aims at, on both its VP8 and its VP9 encoder.
// ffmpeg spells the knob -tune and vp8enc and vp9enc spell it tuning,
// with libvpx's own two values on both.
//
// Content tuning is not on this ladder.
// A claim about the picture rather than about what to aim at,
// this app's picture is a desktop either way,
// and the two engines do not both carry it:
// the vpx elements expose no such property,
// where ffmpeg takes -tune-content and -screen-content-mode,
// so the builders pin it on where they can (ffmpeg/encoders.go),
// rather than offering a step one engine drops.
var vpxTunes = []string{TuneNone, "psnr", "ssim"}

// aomTunes is the metric libaom aims at.
// The ffmpeg engine's alone: av1enc exposes no tune property at all,
// which the row gaps step by step so the untuned one is left standing.
var aomTunes = []string{TuneNone, "psnr", "ssim"}

// svtav1Tunes are SVT-AV1's tuning modes, named as the library's own log prints them.
// svtav1TuneValues carries the numbers the two engines pass, both through a parameter string.
//
// vq weighs what the eye sees, psnr, ssim and ms-ssim each aim at their metric,
// and iq is the library's image-quality mode,
// which overrides the sharpness, quantization-matrix and screen-content settings around it,
// and says so on its own warning line.
var svtav1Tunes = []string{TuneNone, "vq", "psnr", "ssim", "iq", "ms-ssim"}

// svtav1TuneValues is the number SVT-AV1 takes for each named step.
//
// The library's numbering rather than either engine's, both handing it the same parameter string,
// and a step outside 0..4 is refused by the library rather than clamped.
// Named steps and not the bare numbers,
// so a settings file states what it asked for,
// and a shell names the metric once for every codec that aims at it.
var svtav1TuneValues = map[string]string{
	"vq": "0", "psnr": "1", "ssim": "2", "iq": "3", "ms-ssim": "4",
}

// Svtav1TuneValue is the number one named step travels as,
// and false for the untuned step, which both builders answer by passing no tune at all.
// A named step with no number would reach the library as a word it refuses,
// so the pair is held together at load.
func Svtav1TuneValue(step string) (string, bool) {
	value, named := svtav1TuneValues[step]
	return value, named
}

func init() {
	assert.Assert(len(svtav1TuneValues) == len(svtav1Tunes)-1,
		"every SVT-AV1 tune step but the untuned one carries the library's number",
		len(svtav1TuneValues), len(svtav1Tunes))
	for _, step := range svtav1Tunes {
		if step == TuneNone {
			continue
		}
		_, named := svtav1TuneValues[step]
		assert.Assert(named, "an SVT-AV1 tune step carries the number the library takes", step)
	}
}

// rav1eTunes are rav1e's two tuning modes, its own spelling on both engines:
// ffmpeg takes them in -rav1e-params and the rav1enc element on its tune property.
var rav1eTunes = []string{TuneNone, "psnr", "psychovisual"}

// qsvScenarios is oneVPL's scenario hint,
// which tells the encoder what the session is for and lets it weigh its decisions accordingly.
// displayremoting is what a screen share is, and the rest are the scenarios the same knob declares.
//
// The ffmpeg engine's alone:
// the qsv plugin's encoders expose target-usage and low-latency and no scenario,
// which the rows gap step by step.
// It reaches the H.264 and HEVC encoders, the two ffmpeg puts the option on.
var qsvScenarios = []string{
	TuneNone, "displayremoting", "videoconference", "archive", "livestreaming",
	"cameracapture", "videosurveillance", "gamestreaming", "remotegaming",
}

// metricTuneDefaults leaves the knob untuned in every mode,
// the default for a ladder whose steps aim at a measurement rather than at what a viewer sees.
// A metric target is a choice about how the encode is judged,
// and nothing about a screen share answers it, so no mode picks one on the reader's behalf.
var metricTuneDefaults = everyMode(TuneNone)

// svtav1TuneDefaults picks the library's visual mode,
// its own default and the one a picture watched by a person is coded for.
var svtav1TuneDefaults = everyMode("vq")

// rav1eTuneDefaults picks the psychovisual mode, the element's own default.
var rav1eTuneDefaults = everyMode("psychovisual")

// qsvScenarioDefaults declares the scenario this app is:
// a desktop coded for somebody watching it elsewhere.
var qsvScenarioDefaults = everyMode("displayremoting")

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

// nvencPresets is NVIDIA's preset ladder, most effort first:
// p7 is the slowest step and p1 the fastest, so the list counts down.
var nvencPresets = []string{"p7", "p6", "p5", "p4", "p3", "p2", "p1"}

// nvencPresetDefaults is where each mode starts on that ladder.
//
// CBR pins its step instead of starting on it,
// a low-latency preset being what lets NVENC hold a constant rate,
// and the form greys the control and names the step in force.
// A row claiming p7 there would make that sentence name a step the encode never spends.
var nvencPresetDefaults = map[string]string{
	ModeCrf:      "p7",
	ModeAbr:      "p7",
	ModeVbr:      "p7",
	ModeLossless: "p7",
	ModeCbr:      "p5",
}

// nvencTunes are the encoder's tuning info values.
// Lossless is a tune on NVENC and not an effort step,
// which is why the bit-exact mode is served off this ladder.
var nvencTunes = []string{"hq", "ll", "ull", "lossless"}

var nvencTuneDefaults = map[string]string{
	ModeCrf:      "hq",
	ModeAbr:      "hq",
	ModeVbr:      "hq",
	ModeCbr:      "ll",
	ModeLossless: "lossless",
}

// vulkanTunes are the two tuning modes ffmpeg's Vulkan encoders take.
// The API's lossless tuning mode is left out:
// a hint about what to optimize for and not a coding mode (vulkanGaps).
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
// libvpx counts its cpu-used from -16 (VP8) or -8 (VP9) and libaom from 0:
// the negative half belongs to the deadlines this pipeline does not use,
// both capture chains pinning the realtime one.
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

// targetUsages is the target-usage scale, 1 at the quality end and 7 at the speed one.
//
// One ladder for QSV and VAAPI, the two APIs counting the same way:
// oneVPL defines the scale,
// and the va elements take the same seven points on a property of the same name.
// Stated as the numbers, the numbers being the scale's own and the names one engine's spelling:
// the GStreamer elements take the figure on their target-usage property,
// and the ffmpeg QSV builder maps it onto -preset (ffmpeg/encoders.go, qsvPresets).
var targetUsages = steps(1, 7)

var targetUsageDefaults = map[string]string{
	ModeCrf:      "4",
	ModeAbr:      "4",
	ModeVbr:      "4",
	ModeCbr:      "7",
	ModeLossless: "4",
}

// amfPresets is AMD's quality preset, most effort first.
// The higher preset later VCN generations add is left out,
// for the reason amfUsage names in the builder:
// the older generations refuse it,
// and a step that fails on half the hardware in the field is a publish that dies at launch.
var amfPresets = []string{"quality", "balanced", "speed"}

var amfPresetDefaults = map[string]string{
	ModeCrf: "quality",
	ModeAbr: "quality",
	ModeVbr: "quality",
	ModeCbr: "speed",
}

// TuneNone is the settings value for an encode that is not tuned.
//
// A step of the tune ladders rather than an absence,
// for the reason platform.AudioSourceNone is a row of the source table:
// a control needs a value to hold, and a declared one is a value every consumer can name.
// Both builders answer it by passing no tune option at all.
const TuneNone = "none"
