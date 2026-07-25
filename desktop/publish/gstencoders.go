package publish

import (
	"fmt"
	"strconv"

	"bjoernblessin.de/screenshare/settings"
)

// gstRates are the rate-control figures a stream's settings yield, rendered as the
// property values the elements take: kbit per second for the x26x elements, the
// constant-quality target, and the GOP length in frames.
type gstRates struct {
	kbps, maxkbps, cq, gop string
}

func gstRatesFor(s settings.Stream, gop int) gstRates {
	return gstRates{
		kbps:    strconv.Itoa(s.BitrateM * 1000),
		maxkbps: strconv.Itoa(s.MaxrateM * 1000),
		cq:      strconv.Itoa(s.Cq),
		gop:     strconv.Itoa(gop),
	}
}

// The parsers that follow an encoder. config-interval=-1 makes the parser insert
// SPS/PPS (H.264) or VPS/SPS/PPS (H.265) ahead of every IDR frame. Without it the
// parameter sets travel once at stream start, so a viewer that joins the relay
// mid-stream never receives them and its decoder cannot start. The ffmpeg publish
// path repeats them by default, which is why only this backend needs the property.
// VP9 carries no out-of-band parameter sets, so vp9parse needs none;
// rtspclientsink payloads the parsed video/x-vp9.
var (
	h264Parser = []string{"h264parse", "config-interval=-1"}
	h265Parser = []string{"h265parse", "config-interval=-1"}
	vp9Parser  = []string{"vp9parse"}
)

// gstCodec is one codec's GStreamer half: the element that encodes it, with the
// rate-control mode mapped onto that element's own knobs, and the parser behind it.
type gstCodec struct {
	encode func(s settings.Stream, r gstRates) []string
	parser []string
}

// gstCodecs is the GStreamer engine's half of the codec facts declared once in
// capabilities.Codecs, and the counterpart to encoderArgs in the ffmpeg builder.
//
// The non-NVIDIA hardware families in capabilities.Codecs (vaapi, qsv, amf, v4l2,
// rkmpp, vulkan) are declared Implemented:false and rejected before a GStreamer
// pipeline is built, so they have no entry yet. When VAAPI is wired up, target the
// stateless "va" plugin (gst-plugins-bad): vah264enc, vah265enc, vaav1enc. Avoid the
// older "vaapi" plugin (gstreamer-vaapi: vaapih264enc, vaapih265enc). The va plugin
// is the maintained one, exposes AV1 encoding, and negotiates the DMABuf/VAMemory
// caps the portal capture path already produces; gstreamer-vaapi is effectively
// frozen and has no AV1 encoder. QSV uses the "qsv" plugin (qsvh264enc, qsvh265enc,
// qsvav1enc).
var gstCodecs = map[string]gstCodec{
	"libx264":    {encode: x264Encoder, parser: h264Parser},
	"libx265":    {encode: x265Encoder, parser: h265Parser},
	"h264_nvenc": {encode: nvencEncoder("nvh264enc"), parser: h264Parser},
	"hevc_nvenc": {encode: nvencEncoder("nvh265enc"), parser: h265Parser},
	"libvpx-vp9": {encode: vp9Encoder, parser: vp9Parser},
}

// gstEncoder returns the encoder element (with its properties) and the parser for
// the selected codec.
func gstEncoder(s settings.Stream, gop int) (encoder []string, parser []string, err error) {
	c, ok := gstCodecs[s.Codec]
	if !ok {
		return nil, nil, fmt.Errorf("codec %q has no GStreamer encoder mapping", s.Codec)
	}
	return c.encode(s, gstRatesFor(s, gop)), c.parser, nil
}

// x264Encoder maps the rate-control mode onto x264enc's pass property, the
// counterpart to the libx264 branch of encoderArgs.
//   - cbr: pass=cbr targets the bitrate and bounds the VBV to it; low delay.
//   - crf: pass=qual holds a constant quantizer (the s.Cq value), bitrate free.
//   - lossless: pass=quant at quantizer 0, x264's bit-exact coding mode.
//   - abr, vbr: pass=cbr with vbv-buf-capacity=0 disables the VBV, giving
//     one-pass ABR toward the target. x264enc cannot raise the VBV maxrate above
//     the bitrate (pass=cbr locks them equal), so the vbr ceiling binds only on
//     the ffmpeg and nvenc paths; here both run as uncapped average bitrate.
//
// cbr and lossless take tune=zerolatency to hold live delay; the bitrate-bursting
// modes keep B-frames and lookahead for efficiency. The p1-p7 preset ladder is
// NVENC-only.
func x264Encoder(s settings.Stream, r gstRates) []string {
	switch s.Mode {
	case "crf":
		return []string{"x264enc", "pass=qual", "quantizer=" + r.cq, "speed-preset=slow", "key-int-max=" + r.gop}
	case "lossless":
		return []string{"x264enc", "pass=quant", "quantizer=0", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + r.gop}
	case "abr", "vbr":
		return []string{"x264enc", "bitrate=" + r.kbps, "pass=cbr", "vbv-buf-capacity=0", "speed-preset=medium", "key-int-max=" + r.gop}
	default: // cbr
		enc := []string{"x264enc", "bitrate=" + r.kbps, "pass=cbr", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + r.gop}
		if s.VbvMs > 0 {
			enc = append(enc, "vbv-buf-capacity="+strconv.Itoa(s.VbvMs))
		}
		return enc
	}
}

// x265Encoder maps the rate-control mode onto x265enc, the HEVC counterpart to
// x264Encoder. x265enc has no pass property: rate control comes from the bitrate
// and qp properties plus an option-string of libx265 knobs.
//   - crf: qp holds a constant quantizer (s.Cq), x265's CQP mode, matching
//     x264enc's quantizer property.
//   - lossless: option-string lossless=1. Unlike x264, qp 0 is not bit-exact on
//     x265, so the dedicated flag is required; zerolatency drops B-frames.
//   - abr, vbr: bitrate alone is one-pass average bitrate. As on x264enc the vbr
//     ceiling does not bind here, only on the ffmpeg and nvenc paths.
//   - cbr: bitrate plus a vbv-maxrate=bitrate ceiling and a vbv-bufsize window,
//     x265's constrained constant bitrate; zerolatency for low delay.
func x265Encoder(s settings.Stream, r gstRates) []string {
	switch s.Mode {
	case "crf":
		return []string{"x265enc", "qp=" + r.cq, "speed-preset=slow", "key-int-max=" + r.gop}
	case "lossless":
		return []string{"x265enc", "option-string=lossless=1", "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + r.gop}
	case "abr", "vbr":
		return []string{"x265enc", "bitrate=" + r.kbps, "speed-preset=medium", "key-int-max=" + r.gop}
	default: // cbr
		// vbv-bufsize is in kbit: the bitrate held over the VBV window, one second
		// when unset, matching ffmpeg's bufsizeArg.
		bufKbit := r.kbps
		if s.VbvMs > 0 {
			bufKbit = strconv.Itoa(s.BitrateM * s.VbvMs)
		}
		opts := "vbv-maxrate=" + r.kbps + ":vbv-bufsize=" + bufKbit
		return []string{"x265enc", "bitrate=" + r.kbps, "option-string=" + opts, "tune=zerolatency", "speed-preset=veryfast", "key-int-max=" + r.gop}
	}
}

// vp9Encoder maps the rate-control mode onto vp9enc (libvpx VP9), the 4:4:4
// counterpart to the ffmpeg libvpx-vp9 branch. deadline=1 with cpu-used and
// row-mt is the realtime, multi-threaded screen encode; end-usage selects the
// rate-control family (cbr/vbr/cq) and lossless is its own property. vpxenc's
// target-bitrate is in bits/sec, unlike x264enc/x265enc's kbit.
func vp9Encoder(s settings.Stream, r gstRates) []string {
	bps := strconv.Itoa(s.BitrateM * 1_000_000)
	base := []string{
		"vp9enc", "deadline=1", "cpu-used=6", "row-mt=true", "keyframe-max-dist=" + r.gop,
	}
	switch s.Mode {
	case "lossless":
		return append(base, "lossless=true")
	case "crf":
		// Constant quality: cq end-usage at cq-level, the bitrate a burst ceiling.
		return append(base, "end-usage=cq", "cq-level="+r.cq, "target-bitrate="+bps)
	case "abr", "vbr":
		return append(base, "end-usage=vbr", "target-bitrate="+bps)
	default: // cbr
		return append(base, "end-usage=cbr", "target-bitrate="+bps)
	}
}

// nvencEncoder maps the rate-control mode onto one nvcodec element's properties,
// the counterpart to the NVENC branch of encoderArgs. The knobs differ from
// ffmpeg's: nvh264enc/nvh265enc expose rc-mode plus a constant-QP target rather
// than the SDK preset ladders and tunes.
//   - cbr: rc-mode=cbr with zero-latency reordering.
//   - vbr: rc-mode=vbr targeting the bitrate with max-bitrate as the ceiling.
//   - abr: rc-mode=vbr toward the bitrate with no ceiling.
//   - crf: rc-mode=constqp at s.Cq.
//   - lossless: the element's lossless preset, rate control dropped.
//
// B-frames apply only to the lossy bursting modes. The p1-p7 preset ladder in
// s.EncPreset has no equivalent on these elements and is not forwarded, so the
// settings form greys the preset field on this engine.
//
// The element name is bound per codec in gstCodecs, so H.264 and HEVC share one
// mapping.
func nvencEncoder(elem string) func(settings.Stream, gstRates) []string {
	return func(s settings.Stream, r gstRates) []string {
		withBframes := func(enc []string) []string {
			if s.Bframes > 0 {
				return append(enc, "bframes="+strconv.Itoa(s.Bframes))
			}
			return enc
		}
		switch s.Mode {
		case "lossless":
			// No B-frames: bit-exact coding gains nothing from them, which is why the
			// UI greys the field here.
			return []string{elem, "preset=lossless", "gop-size=" + r.gop}
		case "crf":
			return withBframes([]string{elem, "rc-mode=constqp", "qp-const=" + r.cq, "gop-size=" + r.gop})
		case "abr":
			return withBframes([]string{elem, "rc-mode=vbr", "bitrate=" + r.kbps, "gop-size=" + r.gop})
		case "vbr":
			return withBframes([]string{elem, "rc-mode=vbr", "bitrate=" + r.kbps, "max-bitrate=" + r.maxkbps, "gop-size=" + r.gop})
		default: // cbr
			return []string{elem, "rc-mode=cbr", "bitrate=" + r.kbps, "zerolatency=true", "gop-size=" + r.gop}
		}
	}
}
