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

// gstRates carries the rate-control figures as the element property values they are spelled in:
// kbit/s for the x26x elements, the constant-quality target, and the GOP in frames.
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

// The elements linking an encoder to the sink.
//
// config-interval=-1 inserts SPS/PPS (H.264) or VPS/SPS/PPS (H.265) ahead of every IDR.
// Sent once at stream start they never reach a viewer joining the relay mid-stream,
// whose decoder then cannot start.
// The property is this engine's alone: the ffmpeg engine repeats the parameter sets by default.
// VP9 and AV1 carry no out-of-band parameter sets, so their parsers need none and rtspclientsink
// payloads the parsed video/x-vp9 and video/x-av1.
//
// VP8 has no parser element.
// vp8enc leaves profile a list on its output caps and then rejects its own unfixed caps with
// "Invalid vpx profile", so a capsfilter pins profile 0,
// the only profile an 8-bit 4:2:0 VP8 bitstream carries.
var (
	h264Parser = []string{"h264parse", "config-interval=-1"}
	h265Parser = []string{"h265parse", "config-interval=-1"}
	vp9Parser  = []string{"vp9parse"}
	av1Parser  = []string{"av1parse"}
	vp8Caps    = []string{"video/x-vp8,profile=(string)0"}
)

// gstCodec is one codec's GStreamer half: the encoding element with the rate-control mode mapped
// onto its own knobs, and the elements linking it to the sink.
type gstCodec struct {
	// encode builds the element and its properties.
	// l carries the two ladder steps the encode spends, resolved once against the codec's own row,
	// so a mapping states which property carries a step and never which step that is.
	encode func(s settings.Settings, r gstRates, l capabilities.Steps) []string
	link   []string
	// limits refuses a settings combination the element cannot express, beyond what the capability
	// table and gstFamilyLimits already cover.
	// Per codec where the bound is one element's property range: two elements of one plugin can
	// declare the same property differently.
	limits func(settings.Settings) error
}

// gstCodecs is this engine's half of the codec facts capabilities.Codecs declares once,
// and the counterpart to encoderArgs in the ffmpeg builder.
// A rate-control mode or pixel format an element has no form of is a Gap on that codec's row,
// carrying Engine "gstreamer" where the ffmpeg encoder reaches it,
// and is rejected before a pipeline is built, so no branch below approximates one.
//
// The VAAPI rows target the stateless "va" plugin from gst-plugins-bad,
// not gstreamer-vaapi's older vaapih264enc and vaapih265enc.
// The va plugin is the maintained one, it holds the only AV1 encoder,
// and it negotiates the VAMemory caps the portal backend's GPU path runs on:
// vapostproc imports the compositor's dmabuf and converts into surfaces these elements read
// (gpupath.Paths).
// Its elements register per detected device, so a name below exists only where the driver exposes
// that encode entrypoint, which is the condition the codec's probe tests (encoders.Detect).
//
// The QSV rows target the "qsv" plugin, which drives Intel's oneVPL runtime over VA on Linux and
// D3D11 on Windows, and registers per detected device as the va plugin does.
//
// A family with no entry is rejected before a pipeline is built, either as Implemented:false in
// capabilities.Codecs (v4l2, rkmpp) or as a row gapping it off this engine (amf, vulkan).
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
	// No link element: VP8 has no parser, and vavp8enc fixes its output caps where vp8enc leaves the
	// profile a list, so rtspclientsink payloads its video/x-vp8 directly.
	"vp8_vaapi": {encode: vaEncoder("vavp8enc", "qp")},
	"h264_qsv":  {encode: qsvEncoder("qsvh264enc"), link: h264Parser},
	"hevc_qsv":  {encode: qsvEncoder("qsvh265enc"), link: h265Parser},
	// bitrate is an unsigned 16-bit property on the AV1 and VP9 elements where the H.26x ones take any
	// rate (qsvShortBitrateKbps).
	"av1_qsv": {encode: qsvEncoder("qsvav1enc"), link: av1Parser, limits: qsvShortRateLimits},
	"vp9_qsv": {encode: qsvEncoder("qsvvp9enc"), link: vp9Parser, limits: qsvShortRateLimits},
}

// GstEncoderElement returns the element that encodes codec from frames in system memory,
// and false where this engine has no mapping for it.
//
// The element is what the encoder probe asks the plugin registry about: unlike the ffmpeg encoders,
// each lives in a plugin an install may not carry, and the hardware ones register per detected
// device.
//
// System memory is the path every pair has, so a caller asking whether this machine encodes a codec
// at all needs no memory to ask with.
// A caller holding a resolved memory takes GstEncoderElementOn: where a plugin ships an element per
// memory kind the two names differ, and a registry query for the wrong one reports the family
// present while the element a run launches is missing.
func GstEncoderElement(codec string) (string, bool) {
	return GstEncoderElementOn(codec, gpupath.MemorySystem)
}

// GstEncoderElementOn returns the element that encodes codec from frames in the resolved memory,
// and false where this engine has no mapping for the codec.
//
// The system-memory name is read back off a built encoder rather than stored beside the mapping,
// so it cannot drift from the element a pipeline runs.
// The device name comes from the family's device row, the lookup gstEncoder builds with.
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
	// The codec's own steps for that mode: the defaults carry another codec's,
	// and a step off this ladder spells a property value no element takes.
	// Harmless for the element name read back below, and a trap for a caller that reads further.
	l, err := c.ResolveSteps(mode, "", "")
	assert.Assert(err == nil, "a codec's own row resolves against its own ladders", codec, err)
	return gst.encode(s, gstRates{}, l)[0], true
}

// firstGstMode names a rate-control mode this codec's element implements on this engine,
// and false where the table gaps every one of them off.
//
// Reading an element name means building an encoder, and a mapping dispatches on the mode to decide
// which properties to set, so the mode handed to it has to be one the element implements:
// the settings' own default is lossless, which the hardware families have no form of,
// and asking for it would reach the assert.Never every mapping ends in.
func firstGstMode(codec string) (string, bool) {
	for _, mode := range capabilities.Modes {
		if capabilities.Reaches(codec, capabilities.EngineGst, capabilities.OptionMode, mode) {
			return mode, true
		}
	}
	return "", false
}

// gstFamilyLimits are the settings bounds a family's elements impose beyond capabilities.Codecs,
// keyed as that table names the family.
// A family absent here takes whatever the capability table already approved.
//
// Per family because the bound comes from the properties a plugin's elements share,
// and here rather than in the capability table because it binds two settings against each other
// (a ceiling against its target) instead of taking one value out of a set.
var gstFamilyLimits = map[string]func(settings.Settings) error{
	capabilities.FamilyVaapi: vaRateLimits,
}

// gstEncoder returns the encoder element with its properties, and the elements linking it to the
// sink, for the selected codec reading frames in the resolved memory.
// A rate outside an element's property range is refused rather than moved into it.
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
	// Both bounds apply where both exist: the family's comes from what its elements share,
	// the row's from one element's own property range, so neither stands in for the other.
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
	// The element name is the first word every mapping writes, which is how GstEncoderElement reads it
	// back out.
	// A plugin shipping one element per memory kind changes that word and nothing after it:
	// both elements derive from one base class and take the same rate-control properties,
	// the memory deciding only which of them links to the conversion ahead of it.
	if device, named := gstDeviceEncoderElement(c.Family, s.Publish.Codec, memory); named {
		encoder[0] = device
	}
	// The name goes on every encoder whatever the element turned out to be: a write to a playing
	// pipeline addresses "enc" over the control socket, and the codec decides which element wears it
	// (gstlive.go).
	encoder = slices.Insert(encoder, 1, "name="+gstEncoderName)
	return encoder, gst.link, nil
}

// x264Encoder maps the rate-control mode onto x264enc's pass property, the counterpart to the
// libx264 branch of encoderArgs.
//   - crf: pass=qual at a constant quantizer, bitrate free.
//   - lossless: pass=quant at quantizer 0, x264's bit-exact coding mode.
//   - abr: pass=cbr with vbv-buf-capacity=0, which disables the VBV and leaves one-pass ABR toward
//     the target.
//   - cbr: pass=cbr, which targets the bitrate and bounds the VBV to it.
//
// vbr has no branch: pass=cbr locks the VBV maxrate to the bitrate and x264enc cannot raise it
// above, so the mode is a gap on this engine (gstNoRateCeiling) rather than run as this one.
// speed-preset carries x264's named effort ladder, and the tune travels in whichever of the
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

// x264PsyTunes are the steps x264enc takes on its psy-tune property rather than on its tune one.
//
// The element splits x264's tune list across two properties: tune is a flagset of the tunes that
// change what the encoder codes, psy-tune an enum of the ones weighing what the eye sees.
// ffmpeg spells the whole list through one -tune, so which property carries a step is this engine's
// while the step stays the encoder's own identifier.
var x264PsyTunes = []string{"film", "animation", "grain", "psnr", "ssim"}

// x264TuneProperty is the property one tune step travels in on x264enc, and nothing where the
// ladder leaves the knob alone.
// Both properties default to no tuning, so an untuned encode is what neither of them set yields.
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

// x265Encoder maps the rate-control mode onto x265enc, the HEVC counterpart to x264Encoder.
// The element has no pass property: rate control comes from bitrate and qp plus an option-string of
// libx265 knobs.
//   - crf: qp holds a constant quantizer, x265's CQP mode, where x264enc spells the same on
//     quantizer.
//   - lossless: option-string lossless=1, qp 0 not being bit-exact on x265 as it is on x264.
//   - abr: bitrate alone, one-pass average bitrate.
//   - cbr: bitrate with a vbv-maxrate ceiling at the same figure and a vbv-bufsize window,
//     x265's constrained constant bitrate.
//
// vbr has no branch, as on x264enc: the element takes no ceiling above the target
// (gstNoRateCeiling).
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
		// vbv-bufsize is in kbit: the bitrate held over the VBV window, one second where the settings
		// name none, matching ffmpeg's bufsizeArg.
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

// x265TuneProperty is the tune step as x265enc spells it, and nothing where the ladder leaves the
// knob to the encoder.
//
// The untuned step is the one that has to be spelled: this element's tune enum starts at ssim,
// so leaving the property off is a tuning choice here where it is none on ffmpeg.
// The enum's zero entry is named with a space no gst-launch line carries, so it travels as tune=0.
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

// vpxEncoder maps the rate-control mode onto vp8enc or vp9enc, the counterpart to the ffmpeg libvpx
// branches.
// One mapping serves both elements: they expose the same libvpx properties,
// and what differs (VP9's row-mt, the cpu-used point each is worth running at) is bound per codec
// in gstCodecs.
//
// deadline=1 is libvpx's realtime deadline, and static-threshold=100 is the motion threshold the
// elements' own documentation names for screen and window sharing.
// end-usage picks the rate-control family: cq with cq-level is constant quality,
// where the bitrate is a burst cap rather than a target,
// so libvpx has no unbounded constant-quality mode here as the ffmpeg path's -b:v 0 gives.
// end-usage=vbr is the average-bitrate family and takes no ceiling, so vbr has no branch and is a
// gap on this engine (gstNoRateCeiling), as lossless is for want of a property.
// target-bitrate counts in bits/sec and buffer-size in ms, where x264enc and x265enc count kbit.
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

// aomEncoder maps the rate-control mode onto av1enc, the libaom encoder and the counterpart to the
// ffmpeg libaom-av1 branch.
// usage-profile=realtime takes libaom off its two-pass defaults, and cpu-used, row-mt and
// lag-in-frames=0 are the same realtime trade as there.
//
// end-usage=q is libaom's unbounded constant-quality mode, which vpxenc's bitrate-capped cq is not.
// The element exposes no cq-level, so the quantizer target is pinned by setting both quantizer
// bounds to it.
// end-usage=vbr takes no ceiling, so vbr is a gap on this engine (gstNoRateCeiling).
// target-bitrate is in kbit/s and buf-sz in milliseconds.
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

// svtav1Encoder maps the rate-control mode onto svtav1enc, the counterpart to the ffmpeg libsvtav1
// branch.
// Two of the library's constraints are gaps in capabilities.Codecs rather than letting a
// mode run as another: max-bitrate is refused outside constant-quality mode, so vbr has no form on
// either engine, and cbr's low-delay prediction structure stalls this element,
// so cbr has none on this one.
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

// rav1eEncoder maps the rate-control mode onto rav1enc, the counterpart to the ffmpeg librav1e
// branch.
// rav1e's rate control is one bitrate target with no ceiling and no rate buffer,
// so vbr has no form on either engine and cbr differs from abr in dropping frame reordering for
// delay.
// bitrate is in bits/sec and the quantizer counts to 255.
// The element exposes no keyframe interval, so the configured GOP never reaches it and rav1e's own
// default stands.
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

// vaEncoder maps the rate-control mode onto one va plugin element, the counterpart to vaapiArgs in
// the ffmpeg builder.
// quantizers are the properties the constant-quality target is written to, bound per codec in
// gstCodecs: the H.26x elements take one per frame type (qpi for I, qpp for P) and the AV1, VP9 and
// VP8 ones a single qp for every frame.
//   - crf: rate-control=cqp with those quantizer properties set.
//   - cbr: rate-control=cbr at the target bitrate, cpb-size its rate buffer.
//   - vbr: rate-control=vbr, where bitrate is the ceiling and target-percentage places the target
//     under it.
//   - abr: the same VBR with the ceiling at twice the target, which is what ffmpeg derives for a
//     VAAPI VBR encode given no ceiling, so the stream does not change with the capture backend.
//
// VAAPI codes against a maximum either way, so abr is as unbounded as an average gets here.
// bitrate and target-percentage each have a range the settings can fall outside of,
// and vaRateLimits refuses such a combination ahead of this mapping.
//
// bitrate and cpb-size are in kbit, and a zero cpb-size leaves the element its own calculation,
// so the VBV window appears only where the settings carry one.
// No effort step and no B-frame count, as on the ffmpeg path: the family's row declares no ladder,
// and VAAPI B-frame support varies per driver.
// lossless has no VAAPI form (vaapiGaps).
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

// The bounds the va elements' rate-control properties impose: the highest kbit/s bitrate accepts,
// and the floor of target-percentage, which is how far under its ceiling a VBR target may sit.
const (
	vaMaxBitrateKbps      = 2_048_000
	vaMinTargetPercentage = 50
)

// vaAbrPeak is the factor abr places its ceiling at above the target,
// so abr reaches the bitrate bound at half the target the other modes need.
const vaAbrPeak = 2

// vaRateLimits returns why the va elements cannot express the settings' rates, and nil where they
// can.
// A rate outside a property's range is refused rather than moved into it: the ffmpeg engine drives
// the same hardware at the rate the settings name, so a substitution would make the bitrate a
// function of the capture backend, with no field on the form stating what the encode runs at.
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
		// crf drives no rate, and lossless has no VAAPI form.
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

// vaBitrate spells a Mbit/s rate as the kbit figure the bitrate property takes.
func vaBitrate(rateM int) string {
	return strconv.Itoa(rateM * 1000)
}

// vaTargetPercentage places a VBR target under the ceiling bitrate carries, as a percentage of it.
// A ceiling more than twice the target falls under the property's floor and never reaches here
// (vaRateLimits); one at or below the target reads as 100.
func vaTargetPercentage(s settings.Settings) string {
	pct := 100
	if s.Publish.MaxrateM > 0 {
		pct = s.Publish.BitrateM * 100 / s.Publish.MaxrateM
	}
	return strconv.Itoa(min(pct, 100))
}

// qsvAbrPeak is the factor abr places its ceiling at above the target, the derivation the ffmpeg
// builder's QSV mapping makes too, so an unbounded average means one thing on both publish engines.
const qsvAbrPeak = 2

// qsvShortBitrateKbps is the highest rate the AV1 and VP9 qsv elements take: their bitrate and
// max-bitrate properties are unsigned 16-bit where the H.264 and H.265 elements' are unbounded.
const qsvShortBitrateKbps = 65535

// qsvShortRateLimits returns why an element with the 16-bit bitrate property cannot express the
// settings' rates, and nil where it can.
// Refused rather than moved into range, as on the va elements: the ffmpeg engine drives the same
// hardware at the rate the settings name, so a substitution would make the bitrate a function of
// the capture backend.
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
		// crf drives no rate, and lossless has no QSV form.
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

// qsvEncoder maps the rate-control mode onto one qsv element's properties, the counterpart to
// qsvArgs in the ffmpeg builder.
// The mode is named on rate-control rather than derived from which options carry a value,
// which is the one place this engine's QSV path is the plainer of the two.
//   - crf: rate-control=cqp with the quantizer on qp-i and qp-p.
//   - cbr: rate-control=cbr at the target bitrate, plus low-latency, which drops the encoder from
//     four frames in flight to one.
//   - vbr: rate-control=vbr at the target with max-bitrate as the ceiling.
//   - abr: the same VBR with the ceiling at twice the target, since oneVPL codes against a maximum
//     either way and picks one the settings never stated where it is left unset.
//
// One mapping serves every qsv codec: the elements derive from one base class and spell the
// quantizer properties alike.
// The H.264 and H.265 elements count their quantizer to 51 and the AV1 and VP9 ones to 255,
// which is each capability row's CqMax.
//
// bitrate and max-bitrate are in kbit/s.
// The VBV window reaches no qsv element: they expose no rate-buffer property,
// so the settings' figure binds on the ffmpeg engine alone.
// The effort step travels as target-usage, the property oneVPL's scale is spelled on here.
// No B-frame count: the elements code no B-pictures unless asked, as the ffmpeg mapping pins them
// off.
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

// nvencTuneProperties spells each NVENC tune step as this engine's elements take it,
// keyed as the capability row declares the step.
//
// The row uses the SDK's abbreviations, as ffmpeg does; the elements spell them in full words,
// the property's own GstNvEncoderTune enum.
// A step with no entry is one the elements do not implement, and the builder then passes no tune
// rather than a word the element would refuse.
//
// The tune replaces the deprecated half of the elements' preset enum, whose entries name presets
// (default, hp, low-latency-hq) where the p1-p7 half names ladder steps;
// the element's own help says to use the ladder with a tune instead.
var nvencTuneProperties = map[string]string{
	"hq":       "high-quality",
	"ll":       "low-latency",
	"ull":      "ultra-low-latency",
	"lossless": "lossless",
}

// nvencEncoder maps the rate-control mode onto one nvcodec element's properties, the counterpart to
// the NVENC branch of encoderArgs.
// These elements express rate control as rc-mode plus a constant-QP target, where ffmpeg does not.
//   - crf: rc-mode=constqp at the quantizer target.
//   - abr: rc-mode=vbr toward the bitrate with no ceiling.
//   - vbr: rc-mode=vbr toward the bitrate with max-bitrate as the ceiling.
//   - cbr: rc-mode=cbr with zerolatency reordering.
//   - lossless: the lossless tune, rate control dropped.
//
// preset takes the row's p1-p7 step and tune the row's step in this engine's spelling,
// so an NVENC stream encodes at the step the form shows whichever engine built it.
// B-frames reach the lossy bursting modes alone.
//
// The element name is bound per codec in gstCodecs and H.264, HEVC and AV1 share this mapping:
// the nvcodec encoders derive from one base class and expose the same properties,
// the codec deciding only which of them the hardware honours.
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
				// bframes, not b-frames: the elements spell it without the separator,
				// and a property name no element carries fails the launch rather than being ignored.
				return append(enc, "bframes="+strconv.Itoa(s.Publish.Bframes))
			}
			return enc
		}
		switch s.Publish.Mode {
		case "lossless":
			// No B-frames: bit-exact coding gains nothing from them, which is why the form greys the field
			// here.
			//
			// The tune carries the mode rather than the preset: the enum's lossless preset fails session
			// init with NV_ENC_ERR_INVALID_PARAM on the H.264 encoder at 4:2:0,
			// where the tune reaches every element and chroma the mode is offered on.
			// The row declares that tune for this mode, so it arrives through base.
			return append(base, "gop-size="+r.gop)
		case "crf":
			// The quantizer goes on the three frame types rather than through qp-const: nvav1enc exposes
			// qp-const-i, -p and -b and no combined property, where the H.26x elements expose both.
			// The combined property sets one quantizer on all three anyway,
			// so this is the spelling every element in the family reads.
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
