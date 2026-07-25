package ffmpeg

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/settings"
)

// rates are the rate-control figures a stream's settings yield, rendered as the
// values ffmpeg takes on the command line.
type rates struct {
	bitrate, maxrate, cq, bframes string
}

func ratesFor(s settings.Stream) rates {
	return rates{
		bitrate: fmt.Sprintf("%dM", s.BitrateM),
		maxrate: fmt.Sprintf("%dM", s.MaxrateM),
		cq:      strconv.Itoa(s.Cq),
		bframes: strconv.Itoa(s.Bframes),
	}
}

// encoderMappings is the codec-specific half of the command, one entry per encoder
// family whose knobs differ. The five rate-control modes are the methods
// themselves: cbr and vbr and abr all target a bitrate and differ in the ceiling,
// crf targets a quality, lossless is bit-exact. The GStreamer publish path
// expresses the same five in gstCodecs.
//
// The NVENC family is matched by capability rather than by name (encoderArgs),
// because one mapping serves every nvenc codec.
var encoderMappings = map[string]func(s settings.Stream, r rates) []string{
	// x264 reaches bit-exact at -qp 0; x265 has no bit-exact qp, so lossless is
	// its own param.
	"libx264":    softwareArgs("libx264", []string{"-qp", "0"}),
	"libx265":    softwareArgs("libx265", []string{"-x265-params", "lossless=1"}),
	"libvpx-vp9": vp9Args,
}

// encoderArgs returns the encoder arguments for the configured codec and rate
// control mode.
func encoderArgs(s settings.Stream) ([]string, error) {
	r := ratesFor(s)
	if build, ok := encoderMappings[s.Codec]; ok {
		return build(s, r), nil
	}
	if capabilities.IsNvenc(s.Codec) {
		return nvencArgs(s, r), nil
	}
	// Generic bitrate-targeted path. capabilities.Validate rejects every codec whose
	// entry has Implemented:false, so the not-yet-wired hardware families (vaapi,
	// qsv, amf, v4l2, rkmpp, vulkan) never reach here. When one is implemented, give
	// it its own mapping: VAAPI needs a -vaapi_device and a format=nv12,hwupload
	// filter chain, QSV its own device and load path, so none of them fit this bare
	// -b:v fallback.
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

// vp9Args maps the rate-control modes to libvpx VP9 profile 1 (the 4:4:4 form).
// The realtime, row-mt, cpu-used and tune-content=screen knobs keep a live
// screen-content encode within a few cores; profile 1 carries 4:4:4 and, for
// gbrp input, VP9's identity matrix so RGB stays RGB. The five modes match the
// software H.26x path in spirit but use libvpx's own rate-control options.
func vp9Args(s settings.Stream, r rates) []string {
	base := []string{
		"-c:v", "libvpx-vp9", "-profile:v", "1",
		"-deadline", "realtime", "-row-mt", "1", "-cpu-used", "6",
		"-tune-content", "screen",
	}
	switch s.Mode {
	case "lossless":
		return append(base, "-lossless", "1")
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
