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

// The elements that link an encoder to the sink. config-interval=-1 makes the
// parser insert SPS/PPS (H.264) or VPS/SPS/PPS (H.265) ahead of every IDR frame.
// Without it the parameter sets travel once at stream start, so a viewer that joins
// the relay mid-stream never receives them and its decoder cannot start. The ffmpeg
// publish engine repeats them by default, which is why only this engine needs the
// property. VP9 and AV1 carry no out-of-band parameter sets, so their parsers need
// none; rtspclientsink payloads the parsed video/x-vp9 and video/x-av1.
//
// VP8 is the one format with no parser element. vp8enc leaves the profile field on
// its output caps a list and then rejects its own unfixed caps with "Invalid vpx
// profile", so a capsfilter pins profile 0, which is the only profile an 8-bit
// 4:2:0 VP8 bitstream can carry.
var (
	h264Parser = []string{"h264parse", "config-interval=-1"}
	h265Parser = []string{"h265parse", "config-interval=-1"}
	vp9Parser  = []string{"vp9parse"}
	av1Parser  = []string{"av1parse"}
	vp8Caps    = []string{"video/x-vp8,profile=(string)0"}
)

// gstCodec is one codec's GStreamer half: the element that encodes it, with the
// rate-control mode mapped onto that element's own knobs, and the elements linking
// it to the sink.
type gstCodec struct {
	encode func(s settings.Stream, r gstRates) []string
	link   []string
}

// gstCodecs is the GStreamer engine's half of the codec facts declared once in
// capabilities.Codecs, and the counterpart to encoderArgs in the ffmpeg builder. A
// rate-control mode or a pixel format an element has no form of is declared as a Gap
// on that codec's row, carrying Engine "gstreamer" where the ffmpeg encoder reaches
// it, and rejected before a pipeline is built, so no branch below approximates one.
//
// The VAAPI rows target the stateless "va" plugin (gst-plugins-bad), not the older
// "vaapi" one (gstreamer-vaapi: vaapih264enc, vaapih265enc). The va plugin is the
// maintained one, it is the only one with an AV1 encoder, and it negotiates the
// DMABuf/VAMemory caps the portal capture backend already produces. Its elements
// register per detected device, so an element name below exists only where the
// driver exposes that encode entrypoint, which is the same condition the codec's
// probe tests (encoders.Detect).
//
// The families still declared Implemented:false in capabilities.Codecs (qsv, amf,
// v4l2, rkmpp, vulkan) are rejected before a pipeline is built, so they have no
// entry. QSV uses the "qsv" plugin (qsvh264enc, qsvh265enc, qsvav1enc).
var gstCodecs = map[string]gstCodec{
	"libx264":    {encode: x264Encoder, link: h264Parser},
	"libx265":    {encode: x265Encoder, link: h265Parser},
	"h264_nvenc": {encode: nvencEncoder("nvh264enc"), link: h264Parser},
	"hevc_nvenc": {encode: nvencEncoder("nvh265enc"), link: h265Parser},
	"av1_nvenc":  {encode: nvencEncoder("nvav1enc"), link: av1Parser},
	"libvpx-vp9": {encode: vpxEncoder("vp9enc", "cpu-used=6", "row-mt=true"), link: vp9Parser},
	"libvpx":     {encode: vpxEncoder("vp8enc", "cpu-used=8"), link: vp8Caps},
	"libaom-av1": {encode: aomEncoder, link: av1Parser},
	"libsvtav1":  {encode: svtav1Encoder, link: av1Parser},
	"librav1e":   {encode: rav1eEncoder, link: av1Parser},
	"h264_vaapi": {encode: vaEncoder("vah264enc", "qpi", "qpp"), link: h264Parser},
	"hevc_vaapi": {encode: vaEncoder("vah265enc", "qpi", "qpp"), link: h265Parser},
	"av1_vaapi":  {encode: vaEncoder("vaav1enc", "qp"), link: av1Parser},
	"vp9_vaapi":  {encode: vaEncoder("vavp9enc", "qp"), link: vp9Parser},
	// No link element: VP8 has no parser, and unlike vp8enc the va element leaves
	// nothing unfixed for a capsfilter to pin, so rtspclientsink payloads its
	// video/x-vp8 directly.
	"vp8_vaapi": {encode: vaEncoder("vavp8enc", "qp")},
}

// GstEncoderElement returns the GStreamer element that encodes codec, and false when
// this engine has no mapping for it. The name is read off a built encoder rather than
// stored beside the mapping, so it cannot drift from the element a pipeline runs.
//
// The element is what the encoder probe asks the plugin registry about: unlike the
// ffmpeg encoders, each of these lives in a plugin an install may not carry, and the
// hardware ones register per detected device.
func GstEncoderElement(codec string) (string, bool) {
	c, ok := gstCodecs[codec]
	if !ok {
		return "", false
	}
	return c.encode(settings.Defaults(), gstRates{})[0], true
}

// gstEncoder returns the encoder element (with its properties) and the elements
// that link it to the sink for the selected codec.
func gstEncoder(s settings.Stream, gop int) (encoder []string, link []string, err error) {
	c, ok := gstCodecs[s.Codec]
	if !ok {
		return nil, nil, fmt.Errorf("codec %q has no GStreamer encoder mapping", s.Codec)
	}
	return c.encode(s, gstRatesFor(s, gop)), c.link, nil
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

// vpxEncoder maps the rate-control mode onto vp8enc or vp9enc, the libvpx elements
// and the counterpart to the ffmpeg libvpx branches. One mapping serves both: the
// two expose the same libvpx properties, and what differs between them (VP9's
// row-mt, the cpu-used point each is worth running at) is bound per codec in
// gstCodecs.
//
// deadline=1 is libvpx's realtime deadline, and static-threshold is the motion
// threshold the elements' own documentation recommends at 100 for screen and window
// sharing. end-usage selects the rate-control family. Constant quality is cq at
// cq-level, where the bitrate is a burst cap and not a target: libvpx has no
// unbounded constant-quality mode here, unlike the -b:v 0 the ffmpeg path uses.
// Neither element implements lossless, which capabilities.Codecs declares as a gap
// on this engine. vpxenc counts target-bitrate in bits/sec and buffer-size in
// milliseconds, unlike x264enc/x265enc's kbit.
func vpxEncoder(elem string, extra ...string) func(settings.Stream, gstRates) []string {
	return func(s settings.Stream, r gstRates) []string {
		bps := strconv.Itoa(s.BitrateM * 1_000_000)
		base := append([]string{elem, "deadline=1", "static-threshold=100",
			"keyframe-max-dist=" + r.gop}, extra...)
		switch s.Mode {
		case "crf":
			return append(base, "end-usage=cq", "cq-level="+r.cq, "target-bitrate="+bps)
		case "abr", "vbr":
			return append(base, "end-usage=vbr", "target-bitrate="+bps)
		default: // cbr
			enc := append(base, "end-usage=cbr", "target-bitrate="+bps)
			if s.VbvMs > 0 {
				enc = append(enc, "buffer-size="+strconv.Itoa(s.VbvMs))
			}
			return enc
		}
	}
}

// aomEncoder maps the rate-control mode onto av1enc (libaom AV1), the counterpart
// to the ffmpeg libaom-av1 branch. usage-profile=realtime switches libaom off its
// two-pass defaults, and cpu-used, row-mt and lag-in-frames=0 are the same realtime
// trade as there.
//
// end-usage=q is libaom's unbounded constant-quality mode, the one place this
// element goes further than vpxenc, which has only the bitrate-capped cq. av1enc
// exposes no cq-level, so the quantizer target is pinned by setting both quantizer
// bounds to it. target-bitrate is in kbit/s and buf-sz in milliseconds.
func aomEncoder(s settings.Stream, r gstRates) []string {
	base := []string{
		"av1enc", "usage-profile=realtime", "cpu-used=8", "row-mt=true",
		"lag-in-frames=0", "keyframe-max-dist=" + r.gop,
	}
	switch s.Mode {
	case "crf":
		return append(base, "end-usage=q", "min-quantizer="+r.cq, "max-quantizer="+r.cq)
	case "abr", "vbr":
		return append(base, "end-usage=vbr", "target-bitrate="+r.kbps)
	default: // cbr
		enc := append(base, "end-usage=cbr", "target-bitrate="+r.kbps)
		if s.VbvMs > 0 {
			enc = append(enc, "buf-sz="+strconv.Itoa(s.VbvMs))
		}
		return enc
	}
}

// svtav1Preset is the point on SVT-AV1's 0-13 quality/speed ladder the modes encode
// at, the same value the ffmpeg builder passes: the two engines drive one library,
// so a stream's look must not depend on which capture backend produced it.
const svtav1Preset = "9"

// svtav1Encoder maps the rate-control mode onto svtav1enc, the counterpart to the
// ffmpeg libsvtav1 branch. Two of the library's constraints leave it with two
// branches where the other elements have four: target-bitrate alone selects VBR and
// max-bitrate is refused outside constant-quality mode, so cbr, vbr and abr all come
// out as one bitrate target, and capabilities.Codecs declares cbr unreachable on this
// engine because the prediction structure it needs stalls the element.
func svtav1Encoder(s settings.Stream, r gstRates) []string {
	base := []string{"svtav1enc", "preset=" + svtav1Preset, "intra-period-length=" + r.gop}
	if s.Mode == "crf" {
		return append(base, "crf="+r.cq)
	}
	return append(base, "target-bitrate="+r.kbps)
}

// rav1eEncoder maps the rate-control mode onto rav1enc, the counterpart to the
// ffmpeg librav1e branch. rav1e's rate control is one bitrate target with no
// ceiling and no rate buffer, so cbr, vbr and abr differ only in whether frame
// reordering is dropped for delay. bitrate is in bits/sec and the quantizer counts
// to 255. The element exposes no keyframe interval at all, so the configured GOP
// does not reach it and rav1e's own default stands.
func rav1eEncoder(s settings.Stream, r gstRates) []string {
	base := []string{"rav1enc", "speed-preset=10"}
	switch s.Mode {
	case "crf":
		return append(base, "quantizer="+r.cq)
	case "abr", "vbr":
		return append(base, "bitrate="+strconv.Itoa(s.BitrateM*1_000_000))
	default: // cbr
		return append(base, "bitrate="+strconv.Itoa(s.BitrateM*1_000_000), "low-latency=true")
	}
}

// vaEncoder maps the rate-control mode onto one va plugin element, the counterpart
// to vaapiArgs in the ffmpeg builder. quantizers are the properties the
// constant-quality target is written to, bound per codec in gstCodecs because the
// va elements split them by codec: the H.26x ones take a quantizer per frame type
// (qpi for I, qpp for P), the AV1, VP9 and VP8 ones one qp for every frame.
//   - crf: rate-control=cqp with those quantizer properties set.
//   - cbr: rate-control=cbr at the target bitrate, cpb-size its rate buffer.
//   - vbr: rate-control=vbr, where the element reads bitrate as the ceiling and
//     target-percentage places the target under it.
//   - abr: the same VBR with the ceiling at twice the target, which is what ffmpeg
//     derives for a VAAPI VBR encode given no ceiling, so the stream does not change
//     with the capture backend. VAAPI codes against a maximum either way, so this is
//     as unbounded as an average gets here.
//
// bitrate and cpb-size are in kbit; a zero cpb-size leaves the element its own
// calculation, so the VBV window only appears when the settings carry one. No preset
// or B-frame count, matching the ffmpeg path: the p1-p7 ladder is NVENC's and VAAPI
// B-frame support varies per driver. lossless has no VAAPI form (vaapiGaps).
func vaEncoder(elem string, quantizers ...string) func(settings.Stream, gstRates) []string {
	return func(s settings.Stream, r gstRates) []string {
		base := []string{elem, "key-int-max=" + r.gop}
		switch s.Mode {
		case "crf":
			enc := append(base, "rate-control=cqp")
			for _, q := range quantizers {
				enc = append(enc, q+"="+r.cq)
			}
			return enc
		case "abr":
			return append(base, "rate-control=vbr",
				"bitrate="+vaBitrate(s.BitrateM*2), "target-percentage=50")
		case "vbr":
			enc := append(base, "rate-control=vbr",
				"bitrate="+vaBitrate(s.MaxrateM), "target-percentage="+vaTargetPercentage(s))
			if s.VbvMs > 0 {
				enc = append(enc, "cpb-size="+strconv.Itoa(s.MaxrateM*s.VbvMs))
			}
			return enc
		default: // cbr
			enc := append(base, "rate-control=cbr", "bitrate="+vaBitrate(s.BitrateM))
			if s.VbvMs > 0 {
				enc = append(enc, "cpb-size="+strconv.Itoa(s.BitrateM*s.VbvMs))
			}
			return enc
		}
	}
}

// vaMaxBitrateKbps is the highest value the va elements' bitrate property accepts.
const vaMaxBitrateKbps = 2_048_000

// vaBitrate renders a Mbit/s rate as the kbit figure the bitrate property takes,
// bounded by the range it accepts. Only a rate far past what a fixed-function encoder
// sustains reaches that bound, and it is the abr mapping's doubled ceiling that gets
// there first.
func vaBitrate(rateM int) string {
	return strconv.Itoa(min(rateM*1000, vaMaxBitrateKbps))
}

// vaTargetPercentage renders the va elements' way of expressing a VBR target under a
// ceiling: bitrate is the maximum and this percentage places the target below it. The
// property's range is 50-100, so a ceiling above twice the target clamps at the floor
// and one at or below the target reads as 100.
func vaTargetPercentage(s settings.Stream) string {
	pct := 100
	if s.MaxrateM > 0 {
		pct = s.BitrateM * 100 / s.MaxrateM
	}
	return strconv.Itoa(min(max(pct, 50), 100))
}

// nvencEncoder maps the rate-control mode onto one nvcodec element's properties,
// the counterpart to the NVENC branch of encoderArgs. The knobs differ from
// ffmpeg's: the elements expose rc-mode plus a constant-QP target rather than the
// SDK preset ladders and tunes.
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
// The element name is bound per codec in gstCodecs, so H.264, HEVC and AV1 share one
// mapping: the nvcodec encoders derive from one base class and expose the same
// properties, the codec deciding only which of them the hardware honours.
func nvencEncoder(elem string) func(settings.Stream, gstRates) []string {
	return func(s settings.Stream, r gstRates) []string {
		withBframes := func(enc []string) []string {
			if s.Bframes > 0 {
				return append(enc, "b-frames="+strconv.Itoa(s.Bframes))
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
