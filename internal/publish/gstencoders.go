package publish

import (
	"fmt"
	"slices"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/gpupath"
	"bjoernblessin.de/screenshare/internal/settings"
)

// gstRates are the rate-control figures a stream's settings yield, rendered as the
// property values the elements take: kbit per second for the x26x elements, the
// constant-quality target, and the GOP length in frames.
type gstRates struct {
	kbps, maxkbps, cq, gop string
}

func gstRatesFor(s settings.Settings, gop int) gstRates {
	return gstRates{
		kbps:    strconv.Itoa(s.Publish.BitrateM * 1000),
		maxkbps: strconv.Itoa(s.Publish.MaxrateM * 1000),
		cq:      strconv.Itoa(s.Publish.Cq),
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
	// encode builds the element and its properties. l carries the two ladder steps the
	// encode spends, resolved once against the codec's own row, so a mapping states which
	// property carries a step and never which step that is.
	encode func(s settings.Settings, r gstRates, l capabilities.Steps) []string
	link   []string
	// limits refuses a settings combination this element cannot express, beyond what
	// the capability table and the family's own limits already cover. It is per codec
	// rather than per family where the bound is the element's property range, since
	// two elements of one plugin can declare the same property differently.
	limits func(settings.Settings) error
}

// gstCodecs is the GStreamer engine's half of the codec facts declared once in
// capabilities.Codecs, and the counterpart to encoderArgs in the ffmpeg builder. A
// rate-control mode or a pixel format an element has no form of is declared as a Gap
// on that codec's row, carrying Engine "gstreamer" where the ffmpeg encoder reaches
// it, and rejected before a pipeline is built, so no branch below approximates one.
//
// The VAAPI rows target the stateless "va" plugin (gst-plugins-bad), not the older
// "vaapi" one (gstreamer-vaapi: vaapih264enc, vaapih265enc). The va plugin is the
// maintained one, it is the only one with an AV1 encoder, and it negotiates the VAMemory
// caps that make the portal backend's GPU path possible: vapostproc imports the
// compositor's dmabuf and converts into surfaces these elements read (gpupath.Paths).
// Its elements register per detected device, so an element name below exists only where
// the driver exposes that encode entrypoint, which is the same condition the codec's
// probe tests (encoders.Detect).
//
// The QSV rows target the "qsv" plugin, which drives Intel's oneVPL runtime over VA on
// Linux and D3D11 on Windows. Its elements register per detected device as the va ones do,
// so an element name below exists only where an Intel GPU with that encode capability is
// present, which is the same condition the codec's probe tests.
//
// Two kinds of family have no entry, both rejected before a pipeline is built: the
// ones still declared Implemented:false in capabilities.Codecs (v4l2, rkmpp), and
// the ones that row gaps off this engine (amf, vulkan).
var gstCodecs = map[string]gstCodec{
	"libx264":    {encode: x264Encoder, link: h264Parser},
	"libx265":    {encode: x265Encoder, link: h265Parser},
	"h264_nvenc": {encode: nvencEncoder("nvh264enc"), link: h264Parser},
	"hevc_nvenc": {encode: nvencEncoder("nvh265enc"), link: h265Parser},
	"av1_nvenc":  {encode: nvencEncoder("nvav1enc"), link: av1Parser},
	"libvpx-vp9": {encode: vpxEncoder("vp9enc", "row-mt=true"), link: vp9Parser},
	"libvpx":     {encode: vpxEncoder("vp8enc"), link: vp8Caps},
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
	"h264_qsv":  {encode: qsvEncoder("qsvh264enc"), link: h264Parser},
	"hevc_qsv":  {encode: qsvEncoder("qsvh265enc"), link: h265Parser},
	// The AV1 and VP9 elements state their bitrate in an unsigned 16-bit property
	// where the H.26x ones take any rate (qsvShortBitrate).
	"av1_qsv": {encode: qsvEncoder("qsvav1enc"), link: av1Parser, limits: qsvShortRateLimits},
	"vp9_qsv": {encode: qsvEncoder("qsvvp9enc"), link: vp9Parser, limits: qsvShortRateLimits},
}

// GstEncoderElement returns the GStreamer element that encodes codec from frames in
// system memory, and false when this engine has no mapping for it.
//
// The element is what the encoder probe asks the plugin registry about: unlike the
// ffmpeg encoders, each of these lives in a plugin an install may not carry, and the
// hardware ones register per detected device.
//
// System memory is the path every pair has, which is why it is this form's answer and why
// a caller asking "can this machine encode that codec at all" needs no memory to ask it
// with. A caller holding a resolved memory asks GstEncoderElementOn instead: on a family
// whose plugin ships one element per memory kind the two differ, and a registry query for
// the wrong one reports the family present while the element a run would launch is
// missing.
func GstEncoderElement(codec string) (string, bool) {
	return GstEncoderElementOn(codec, gpupath.MemorySystem)
}

// GstEncoderElementOn returns the element that encodes codec from frames reaching it in
// the resolved memory, and false when this engine has no mapping for the codec.
//
// The system-memory name is read off a built encoder rather than stored beside the
// mapping, so it cannot drift from the element a pipeline runs; the device name comes from
// the family's device row, which is the same lookup gstEncoder builds with.
func GstEncoderElementOn(codec, memory string) (string, bool) {
	gst, ok := gstCodecs[codec]
	if !ok {
		return "", false
	}
	c, ok := capabilities.Get(codec)
	if !ok {
		return "", false
	}
	if device, named := gstDeviceEncoderElement(c.Family, codec, memory); named {
		return device, true
	}
	mode, ok := firstGstMode(codec)
	if !ok {
		return "", false
	}
	s := settings.Defaults()
	s.Publish.Mode = mode
	// The steps this codec's row declares for that mode, which is what a draft naming this
	// codec would hold. The defaults carry another codec's, and a mapping handed a step off
	// its own ladder would spell a property value no element takes - harmless for the
	// element name read back below, and a trap for the next caller that reads further.
	l, err := c.ResolveSteps(mode, "", "")
	assert.Assert(err == nil, "a codec's own row resolves against its own ladders", codec, err)
	return gst.encode(s, gstRates{}, l)[0], true
}

// firstGstMode names a rate-control mode this codec's element has on this engine, and
// false when the table gaps every one of them off.
//
// Building an encoder is what reads the element name off the mapping, and a mapping
// takes the mode apart to decide which properties to set, so the mode it is handed has
// to be one the element implements: the settings' own default is lossless, which the
// hardware families have no form of, and asking for it would reach the exhaustive
// dispatch every mapping ends in.
func firstGstMode(codec string) (string, bool) {
	for _, mode := range capabilities.Modes {
		if capabilities.Reaches(codec, capabilities.EngineGst, capabilities.OptionMode, mode) {
			return mode, true
		}
	}
	return "", false
}

// gstFamilyLimits are the settings bounds an encoder family's elements impose beyond
// what capabilities.Codecs declares, keyed as capabilities.Codecs names the family. A
// family absent here takes whatever the capability table already approved.
//
// The bound is per family rather than per codec because it comes from the properties
// a plugin's elements share, and it lives here rather than in the capability table
// because it binds two settings against each other (a ceiling against its target)
// instead of taking one value out of a set.
var gstFamilyLimits = map[string]func(settings.Settings) error{
	capabilities.FamilyVaapi: vaRateLimits,
}

// gstEncoder returns the encoder element (with its properties) and the elements that
// link it to the sink for the selected codec, reading frames in the resolved memory. A
// rate outside the element's property range is refused rather than moved into it.
func gstEncoder(s settings.Settings, gop int, memory string) (encoder []string, link []string, err error) {
	gst, ok := gstCodecs[s.Publish.Codec]
	if !ok {
		return nil, nil, fmt.Errorf("codec %q has no GStreamer encoder mapping", s.Publish.Codec)
	}
	c, ok := capabilities.Get(s.Publish.Codec)
	if !ok {
		return nil, nil, fmt.Errorf("unknown codec %q", s.Publish.Codec)
	}
	if limits, ok := gstFamilyLimits[c.Family]; ok {
		if err := limits(s); err != nil {
			return nil, nil, err
		}
	}
	// Both bounds apply where both exist: a family's limit comes from what its
	// elements share and a row's from one element's own property range, so neither
	// stands in for the other.
	if gst.limits != nil {
		if err := gst.limits(s); err != nil {
			return nil, nil, err
		}
	}
	l, err := c.ResolveSteps(s.Publish.Mode, s.Publish.Effort, s.Publish.Tune)
	if err != nil {
		return nil, nil, err
	}
	encoder = gst.encode(s, gstRatesFor(s, gop), l)
	assert.Assert(len(encoder) > 0, "a codec mapping names its element ahead of its properties", s.Publish.Codec)
	// The element name is the first word every mapping writes, which is also how
	// GstEncoderElement reads it back out. A family whose plugin ships one element per
	// memory kind changes that word and nothing after it: the two elements derive from
	// one base class and take the same rate-control properties, the memory deciding only
	// which of them can link to the conversion ahead of it.
	if device, named := gstDeviceEncoderElement(c.Family, s.Publish.Codec, memory); named {
		encoder[0] = device
	}
	// The name goes on every encoder, whichever element it turned out to be, because that
	// is how a value reaches this element while the pipeline plays: the control socket
	// addresses "enc" and the codec decides what wears it (gstlive.go).
	encoder = slices.Insert(encoder, 1, "name="+gstEncoderName)
	return encoder, gst.link, nil
}

// x264Encoder maps the rate-control mode onto x264enc's pass property, the
// counterpart to the libx264 branch of encoderArgs.
//   - cbr: pass=cbr targets the bitrate and bounds the VBV to it; low delay.
//   - crf: pass=qual holds a constant quantizer (the s.Cq value), bitrate free.
//   - lossless: pass=quant at quantizer 0, x264's bit-exact coding mode.
//   - abr: pass=cbr with vbv-buf-capacity=0 disables the VBV, giving one-pass ABR
//     toward the target. vbr has no branch: x264enc cannot raise the VBV maxrate
//     above the bitrate, pass=cbr locking the two equal, so the mode is declared
//     as a gap on this engine (gstNoRateCeiling) rather than run as this one.
//
// Both ladder steps come off the codec's row, spelled as this element's properties:
// speed-preset carries x264's named ladder, and the tune travels in whichever of the
// element's two tune properties declares it (x264TuneProperty).
func x264Encoder(s settings.Settings, r gstRates, l capabilities.Steps) []string {
	base := slices.Concat([]string{"x264enc", "speed-preset=" + l.Effort}, x264TuneProperty(l.Tune),
		[]string{"key-int-max=" + r.gop})
	switch s.Publish.Mode {
	case "crf":
		return append(base, "pass=qual", "quantizer="+r.cq)
	case "lossless":
		return append(base, "pass=quant", "quantizer=0")
	case "abr":
		return append(base, "bitrate="+r.kbps, "pass=cbr", "vbv-buf-capacity=0")
	case "cbr":
		enc := append(base, "bitrate="+r.kbps, "pass=cbr")
		if s.Publish.VbvMs > 0 {
			enc = append(enc, "vbv-buf-capacity="+strconv.Itoa(s.Publish.VbvMs))
		}
		return enc
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// x264PsyTunes are the steps x264enc takes on its psy-tune property rather than on its
// tune one.
//
// The element splits x264's own tune list across two properties: tune is a flagset of the
// three tunes that change what the encoder codes, and psy-tune an enum of the five that
// weigh what the eye sees. ffmpeg spells all eight through one -tune, so a step's property
// is this engine's alone while the step itself stays the encoder's own identifier.
var x264PsyTunes = []string{"film", "animation", "grain", "psnr", "ssim"}

// x264TuneProperty is the property one tune step travels in on x264enc, and nothing where
// the ladder leaves the knob alone. Both properties default to no tuning, so an untuned
// encode is what the element does with neither of them set.
func x264TuneProperty(step string) []string {
	switch {
	case step == "" || step == capabilities.TuneNone:
		return nil
	case slices.Contains(x264PsyTunes, step):
		return []string{"psy-tune=" + step}
	default:
		return []string{"tune=" + step}
	}
}

// x265Encoder maps the rate-control mode onto x265enc, the HEVC counterpart to
// x264Encoder. x265enc has no pass property: rate control comes from the bitrate
// and qp properties plus an option-string of libx265 knobs.
//   - crf: qp holds a constant quantizer (s.Cq), x265's CQP mode, matching
//     x264enc's quantizer property.
//   - lossless: option-string lossless=1. Unlike x264, qp 0 is not bit-exact on
//     x265, so the dedicated flag is required; zerolatency drops B-frames.
//   - abr: bitrate alone is one-pass average bitrate. As on x264enc there is no vbr
//     branch, the element taking no ceiling above the target (gstNoRateCeiling).
//   - cbr: bitrate plus a vbv-maxrate=bitrate ceiling and a vbv-bufsize window,
//     x265's constrained constant bitrate; zerolatency for low delay.
func x265Encoder(s settings.Settings, r gstRates, l capabilities.Steps) []string {
	base := slices.Concat([]string{"x265enc", "speed-preset=" + l.Effort}, x265TuneProperty(l.Tune),
		[]string{"key-int-max=" + r.gop})
	switch s.Publish.Mode {
	case "crf":
		return append(base, "qp="+r.cq)
	case "lossless":
		return append(base, "option-string=lossless=1")
	case "abr":
		return append(base, "bitrate="+r.kbps)
	case "cbr":
		// vbv-bufsize is in kbit: the bitrate held over the VBV window, one second
		// when unset, matching ffmpeg's bufsizeArg.
		bufKbit := r.kbps
		if s.Publish.VbvMs > 0 {
			bufKbit = strconv.Itoa(s.Publish.BitrateM * s.Publish.VbvMs)
		}
		opts := "vbv-maxrate=" + r.kbps + ":vbv-bufsize=" + bufKbit
		return append(base, "bitrate="+r.kbps, "option-string="+opts)
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// x265TuneProperty is the tune step as x265enc's own property, and nothing where the
// ladder leaves the knob to the encoder.
//
// The untuned step is the one that has to be spelled. This element's tune enum starts at
// ssim rather than at no tuning, so leaving the property off is a tuning choice here where
// it is none on ffmpeg, and the enum's zero entry is named with a space that a gst-launch
// line cannot carry. It travels as its number instead.
func x265TuneProperty(step string) []string {
	switch step {
	case "":
		return nil
	case capabilities.TuneNone:
		return []string{"tune=0"}
	default:
		return []string{"tune=" + step}
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
// end-usage=vbr is libvpx's average-bitrate family and takes no ceiling, so vbr has
// no branch and is a gap on this engine (gstNoRateCeiling), as lossless is for want
// of a property. vpxenc counts target-bitrate in bits/sec and buffer-size in
// milliseconds, unlike x264enc/x265enc's kbit.
func vpxEncoder(elem string, extra ...string) func(settings.Settings, gstRates, capabilities.Steps) []string {
	return func(s settings.Settings, r gstRates, l capabilities.Steps) []string {
		bps := strconv.Itoa(s.Publish.BitrateM * 1_000_000)
		base := append([]string{elem, "deadline=1", "static-threshold=100",
			"cpu-used=" + l.Effort, "keyframe-max-dist=" + r.gop}, extra...)
		switch s.Publish.Mode {
		case "crf":
			return append(base, "end-usage=cq", "cq-level="+r.cq, "target-bitrate="+bps)
		case "abr":
			return append(base, "end-usage=vbr", "target-bitrate="+bps)
		case "cbr":
			enc := append(base, "end-usage=cbr", "target-bitrate="+bps)
			if s.Publish.VbvMs > 0 {
				enc = append(enc, "buffer-size="+strconv.Itoa(s.Publish.VbvMs))
			}
			return enc
		default:
			assert.Never("unexpected rate-control mode", s.Publish.Mode)
			return nil
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
// bounds to it. Its end-usage=vbr takes no ceiling either, so vbr is a gap on this
// engine (gstNoRateCeiling). target-bitrate is in kbit/s and buf-sz in milliseconds.
func aomEncoder(s settings.Settings, r gstRates, l capabilities.Steps) []string {
	base := []string{
		"av1enc", "usage-profile=realtime", "cpu-used=" + l.Effort, "row-mt=true",
		"lag-in-frames=0", "keyframe-max-dist=" + r.gop,
	}
	switch s.Publish.Mode {
	case "crf":
		return append(base, "end-usage=q", "min-quantizer="+r.cq, "max-quantizer="+r.cq)
	case "abr":
		return append(base, "end-usage=vbr", "target-bitrate="+r.kbps)
	case "cbr":
		enc := append(base, "end-usage=cbr", "target-bitrate="+r.kbps)
		if s.Publish.VbvMs > 0 {
			enc = append(enc, "buf-sz="+strconv.Itoa(s.Publish.VbvMs))
		}
		return enc
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// svtav1Encoder maps the rate-control mode onto svtav1enc, the counterpart to the
// ffmpeg libsvtav1 branch. Two of the library's constraints leave it with two
// branches where the other elements have four, and capabilities.Codecs declares both
// as gaps rather than letting a mode run as another: max-bitrate is refused outside
// constant-quality mode, so vbr has no form on either engine, and cbr's low-delay
// prediction structure stalls this element, so it has none on this one.
func svtav1Encoder(s settings.Settings, r gstRates, l capabilities.Steps) []string {
	base := []string{"svtav1enc", "preset=" + l.Effort, "intra-period-length=" + r.gop}
	switch s.Publish.Mode {
	case "crf":
		return append(base, "crf="+r.cq)
	case "abr":
		return append(base, "target-bitrate="+r.kbps)
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
}

// rav1eEncoder maps the rate-control mode onto rav1enc, the counterpart to the
// ffmpeg librav1e branch. rav1e's rate control is one bitrate target with no
// ceiling and no rate buffer, so vbr has no form on either engine and cbr and abr
// differ only in whether frame reordering is dropped for delay. bitrate is in
// bits/sec and the quantizer counts
// to 255. The element exposes no keyframe interval at all, so the configured GOP
// does not reach it and rav1e's own default stands.
func rav1eEncoder(s settings.Settings, r gstRates, l capabilities.Steps) []string {
	base := []string{"rav1enc", "speed-preset=" + l.Effort}
	switch s.Publish.Mode {
	case "crf":
		return append(base, "quantizer="+r.cq)
	case "abr":
		return append(base, "bitrate="+strconv.Itoa(s.Publish.BitrateM*1_000_000))
	case "cbr":
		return append(base, "bitrate="+strconv.Itoa(s.Publish.BitrateM*1_000_000), "low-latency=true")
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
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
// bitrate and target-percentage both have a range the settings can fall outside of.
// vaRateLimits refuses such a combination ahead of this mapping.
//
// bitrate and cpb-size are in kbit; a zero cpb-size leaves the element its own
// calculation, so the VBV window only appears when the settings carry one. No effort step
// or B-frame count, matching the ffmpeg path: this family's row declares no ladder and
// VAAPI B-frame support varies per driver. lossless has no VAAPI form (vaapiGaps).
func vaEncoder(elem string, quantizers ...string) func(settings.Settings, gstRates, capabilities.Steps) []string {
	return func(s settings.Settings, r gstRates, _ capabilities.Steps) []string {
		base := []string{elem, "key-int-max=" + r.gop}
		switch s.Publish.Mode {
		case "crf":
			enc := append(base, "rate-control=cqp")
			for _, q := range quantizers {
				enc = append(enc, q+"="+r.cq)
			}
			return enc
		case "abr":
			return append(base, "rate-control=vbr",
				"bitrate="+vaBitrate(s.Publish.BitrateM*vaAbrPeak),
				"target-percentage="+strconv.Itoa(100/vaAbrPeak))
		case "vbr":
			enc := append(base, "rate-control=vbr",
				"bitrate="+vaBitrate(s.Publish.MaxrateM), "target-percentage="+vaTargetPercentage(s))
			if s.Publish.VbvMs > 0 {
				enc = append(enc, "cpb-size="+strconv.Itoa(s.Publish.MaxrateM*s.Publish.VbvMs))
			}
			return enc
		case "cbr":
			enc := append(base, "rate-control=cbr", "bitrate="+vaBitrate(s.Publish.BitrateM))
			if s.Publish.VbvMs > 0 {
				enc = append(enc, "cpb-size="+strconv.Itoa(s.Publish.BitrateM*s.Publish.VbvMs))
			}
			return enc
		default:
			assert.Never("unexpected rate-control mode", s.Publish.Mode)
			return nil
		}
	}
}

// The bounds the va elements' rate-control properties impose: the highest value
// bitrate accepts, and the floor of target-percentage, which bounds how far under its
// ceiling a VBR target can sit.
const (
	vaMaxBitrateKbps      = 2_048_000
	vaMinTargetPercentage = 50
)

// vaAbrPeak is the factor the abr mapping places its ceiling at above the target, so
// abr reaches the bitrate bound at half the target the other modes need.
const vaAbrPeak = 2

// vaRateLimits returns the reason the va elements cannot express the settings' rates,
// and nil where they can. A rate outside a property's range is refused rather than
// moved into it: the ffmpeg engine drives the same hardware at the rate the settings
// name, so a substitution here would make the bitrate a function of the capture
// backend, with no field on the form stating what the encode runs at.
func vaRateLimits(s settings.Settings) error {
	var rateM int
	switch s.Publish.Mode {
	case "cbr":
		rateM = s.Publish.BitrateM
	case "abr":
		rateM = s.Publish.BitrateM * vaAbrPeak
	case "vbr":
		if s.Publish.MaxrateM > 0 && s.Publish.BitrateM*100/s.Publish.MaxrateM < vaMinTargetPercentage {
			return fmt.Errorf("the va encoder elements state a VBR target as a percentage of the ceiling and take %d%% at the lowest, so a %d Mbit/s target under a %d Mbit/s ceiling has no form here: the ceiling can be at most twice the target",
				vaMinTargetPercentage, s.Publish.BitrateM, s.Publish.MaxrateM)
		}
		rateM = s.Publish.MaxrateM
	case "crf", "lossless":
		// crf sets no rate, and lossless has no VAAPI form at all.
		return nil
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
	if rateM*1000 > vaMaxBitrateKbps {
		return fmt.Errorf("the va encoder elements' bitrate property stops at %d kbit/s, and %s mode drives it at %d Mbit/s from these settings",
			vaMaxBitrateKbps, s.Publish.Mode, rateM)
	}
	return nil
}

// vaBitrate renders a Mbit/s rate as the kbit figure the bitrate property takes.
func vaBitrate(rateM int) string {
	return strconv.Itoa(rateM * 1000)
}

// vaTargetPercentage renders the va elements' way of expressing a VBR target under a
// ceiling: bitrate is the maximum and this percentage places the target below it. A
// ceiling more than twice the target falls under the property's floor and never reaches
// here (vaRateLimits); one at or below the target reads as 100.
func vaTargetPercentage(s settings.Settings) string {
	pct := 100
	if s.Publish.MaxrateM > 0 {
		pct = s.Publish.BitrateM * 100 / s.Publish.MaxrateM
	}
	return strconv.Itoa(min(pct, 100))
}

// qsvAbrPeak is the factor the abr mapping places its ceiling at above the target, the
// same derivation the ffmpeg builder's QSV mapping uses, so an unbounded average means
// the same thing on both publish engines.
const qsvAbrPeak = 2

// qsvShortBitrateKbps is the highest rate the AV1 and VP9 qsv elements accept: their
// bitrate and max-bitrate properties are unsigned 16-bit where the H.264 and H.265
// elements' are unbounded.
const qsvShortBitrateKbps = 65535

// qsvShortRateLimits returns the reason an element with the 16-bit bitrate property
// cannot express the settings' rates, and nil where it can. As on the va elements the
// rate is refused rather than moved into range: the ffmpeg engine drives the same
// hardware at the rate the settings name, so a substitution here would make the bitrate
// a function of the capture backend.
func qsvShortRateLimits(s settings.Settings) error {
	var rateM int
	switch s.Publish.Mode {
	case "cbr":
		rateM = s.Publish.BitrateM
	case "abr":
		rateM = s.Publish.BitrateM * qsvAbrPeak
	case "vbr":
		rateM = s.Publish.MaxrateM
	case "crf", "lossless":
		// crf sets no rate, and lossless has no QSV form at all.
		return nil
	default:
		assert.Never("unexpected rate-control mode", s.Publish.Mode)
		return nil
	}
	if rateM*1000 > qsvShortBitrateKbps {
		return fmt.Errorf("the AV1 and VP9 qsv elements' bitrate property stops at %d kbit/s, and %s mode drives it at %d Mbit/s from these settings",
			qsvShortBitrateKbps, s.Publish.Mode, rateM)
	}
	return nil
}

// qsvEncoder maps the rate-control mode onto one qsv element's properties, the
// counterpart to qsvArgs in the ffmpeg builder. The rate control is named here rather
// than derived from which options carry a value, which is the one place this engine's
// QSV path is the plainer of the two.
//   - crf: rate-control=cqp with the quantizer on qp-i and qp-p.
//   - cbr: rate-control=cbr at the target bitrate, plus low-latency, which drops the
//     encoder's pipeline from four frames in flight to one.
//   - vbr: rate-control=vbr at the target with max-bitrate as the ceiling.
//   - abr: the same VBR with the ceiling at twice the target, since oneVPL codes against
//     a maximum either way and left unset it picks one the settings never stated.
//
// One mapping serves all four codecs: the qsv elements derive from one base class, and
// the quantizer properties are spelled the same on each. The H.264 and H.265 elements
// count their quantizer to 51 and the AV1 and VP9 ones to 255, which is the CqMax of
// each capability row.
//
// bitrate and max-bitrate are in kbit/s. The VBV window does not reach these elements:
// they expose no rate-buffer property, so the settings' figure binds on the ffmpeg engine
// alone. The effort step is the settings' own and reaches the element as its target-usage,
// which is the property oneVPL's scale is spelled on here. No B-frame count: the elements code no
// B-pictures unless asked, which matches the ffmpeg mapping pinning them off.
func qsvEncoder(elem string) func(settings.Settings, gstRates, capabilities.Steps) []string {
	return func(s settings.Settings, r gstRates, l capabilities.Steps) []string {
		base := []string{elem, "gop-size=" + r.gop}
		if l.Effort != "" {
			base = append(base, "target-usage="+l.Effort)
		}
		switch s.Publish.Mode {
		case "crf":
			return append(base, "rate-control=cqp",
				"qp-i="+r.cq, "qp-p="+r.cq)
		case "abr":
			return append(base, "rate-control=vbr",
				"bitrate="+r.kbps, "max-bitrate="+strconv.Itoa(s.Publish.BitrateM*1000*qsvAbrPeak))
		case "vbr":
			return append(base, "rate-control=vbr",
				"bitrate="+r.kbps, "max-bitrate="+r.maxkbps)
		case "cbr":
			return append(base, "rate-control=cbr",
				"bitrate="+r.kbps, "low-latency=true")
		default:
			assert.Never("unexpected rate-control mode", s.Publish.Mode)
			return nil
		}
	}
}

// nvencTuneProperties is how each step of the NVENC tune ladder is spelled on this
// engine, keyed as the capability row declares the step.
//
// The row spells the steps as the SDK and ffmpeg do; the elements spell the same four in
// full words, which is the property's own GstNvEncoderTune enum. A step this map has no
// entry for is one the elements do not implement, and the builder passes no tune rather
// than a word the element would refuse.
//
// The deprecated half of the elements' preset enum is what this replaces. Those entries
// name presets (default, hp, low-latency-hq) where the p1-p7 half names ladder steps, and
// the element's own help says to use the ladder with a tune instead.
var nvencTuneProperties = map[string]string{
	"hq":       "high-quality",
	"ll":       "low-latency",
	"ull":      "ultra-low-latency",
	"lossless": "lossless",
}

// nvencEncoder maps the rate-control mode onto one nvcodec element's properties,
// the counterpart to the NVENC branch of encoderArgs. The rate control differs from
// ffmpeg's: the elements express it as rc-mode plus a constant-QP target.
//   - cbr: rc-mode=cbr with zero-latency reordering.
//   - vbr: rc-mode=vbr targeting the bitrate with max-bitrate as the ceiling.
//   - abr: rc-mode=vbr toward the bitrate with no ceiling.
//   - crf: rc-mode=constqp at s.Cq.
//   - lossless: the lossless tune, rate control dropped.
//
// Both ladder steps reach these elements: preset takes the row's p1-p7 step and tune the
// row's step in this engine's spelling, so an NVENC stream encodes at the step the form
// shows whichever engine builds it. B-frames apply only to the lossy bursting modes.
//
// The element name is bound per codec in gstCodecs, so H.264, HEVC and AV1 share one
// mapping: the nvcodec encoders derive from one base class and expose the same
// properties, the codec deciding only which of them the hardware honours.
func nvencEncoder(elem string) func(settings.Settings, gstRates, capabilities.Steps) []string {
	return func(s settings.Settings, r gstRates, l capabilities.Steps) []string {
		base := []string{elem}
		if l.Effort != "" {
			base = append(base, "preset="+l.Effort)
		}
		if tune, spelled := nvencTuneProperties[l.Tune]; spelled {
			base = append(base, "tune="+tune)
		}
		withBframes := func(enc []string) []string {
			if s.Publish.Bframes > 0 {
				// bframes, not b-frames: the nvcodec elements spell it without the
				// separator, and a property name no element carries fails the launch
				// rather than being ignored.
				return append(enc, "bframes="+strconv.Itoa(s.Publish.Bframes))
			}
			return enc
		}
		switch s.Publish.Mode {
		case "lossless":
			// No B-frames: bit-exact coding gains nothing from them, which is why the
			// UI greys the field here.
			//
			// The tune carries the mode rather than the preset: the enum's lossless preset
			// no longer reaches the H.264 encoder at 4:2:0 at all, failing session init
			// with NV_ENC_ERR_INVALID_PARAM, where the tune reaches every element and
			// chroma the mode is offered on. The row declares that tune for this mode, so
			// it arrives through base above.
			return append(base, "gop-size="+r.gop)
		case "crf":
			// The quantizer goes on the three frame types rather than through qp-const,
			// which the AV1 element does not carry: nvav1enc exposes qp-const-i, -p and
			// -b and no combined one, where the H.26x elements expose both. One quantizer
			// on all three frame types is what the combined property sets anyway, so this
			// is the spelling every element in the family reads.
			return withBframes(append(base, "rc-mode=constqp",
				"qp-const-i="+r.cq, "qp-const-p="+r.cq, "qp-const-b="+r.cq,
				"gop-size="+r.gop))
		case "abr":
			return withBframes(append(base, "rc-mode=vbr", "bitrate="+r.kbps, "gop-size="+r.gop))
		case "vbr":
			return withBframes(append(base, "rc-mode=vbr", "bitrate="+r.kbps, "max-bitrate="+r.maxkbps, "gop-size="+r.gop))
		case "cbr":
			return append(base, "rc-mode=cbr", "bitrate="+r.kbps, "zerolatency=true", "gop-size="+r.gop)
		default:
			assert.Never("unexpected rate-control mode", s.Publish.Mode)
			return nil
		}
	}
}
