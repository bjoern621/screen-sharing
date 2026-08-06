package capabilities

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"
)

// The decode half of the codec facts: which decoder takes a published stream, and
// whether that decoder is hardware or the CPU.
//
// It is the same question the encode table answers in the other direction, and it is a
// publish-side concern for one reason: the chroma the publisher picks decides it. Every
// hardware decoder is 4:2:0, with 10-bit where the format has a Main-10 equivalent, and
// two of them add HEVC's full-chroma profiles. A stream in any other pixel format
// reaches every viewer's CPU, whatever GPU that viewer has, so a 4:4:4 or RGB choice
// costs the viewer a software decode at the publisher's discretion.
//
// The viewer is not this machine, so nothing here is probed: a stream is published once
// and watched on whatever hardware the watchers have. The table therefore states what
// each decoder family reaches at its best, and a verdict of "hardware" means some GPU
// decodes this rather than that a given one will. The native grid reports what its own
// decodebin picked per stream, which is the measured counterpart of this.
//
// A software decoder covers every row, so a stream is never undecodable: the question is
// only whether it costs a core.

// The decoder families a stream can be decoded by. A family is one plugin's worth of
// elements, and the axis a per-vendor fact keys off, as the encoder families are on the
// publish side. They are their own set rather than the encoder families reused: the
// vendors do not divide the same way (NVDEC and NVENC differ in what they carry, AMD has
// no decoder plugin of its own) and one of them, DXVA, is a platform's rather than a
// vendor's.
const (
	// DecodeSoftware is the CPU path: gst-libav for the H.26x formats, libvpx for VPx,
	// dav1d for AV1. Present on every install and the reason no stream is undecodable.
	DecodeSoftware = "software"
	// DecodeVa is the va plugin (vah264dec and siblings): Linux, Intel and AMD, over
	// whichever VA driver the machine has.
	DecodeVa = "va"
	// DecodeNvcodec is the nvcodec plugin (nvh264dec and siblings): NVDEC on Linux and
	// Windows.
	DecodeNvcodec = "nvcodec"
	// DecodeQsv is the qsv plugin (qsvh264dec and siblings): Intel's oneVPL runtime on
	// Linux and Windows.
	DecodeQsv = "qsv"
	// DecodeDxva is the d3d11 plugin (d3d11h264dec and siblings): Windows, vendor
	// neutral, the path a machine with no vendor plugin still has.
	DecodeDxva = "dxva"
)

// DecodeFamilies lists every decoder family a row may declare.
var DecodeFamilies = []string{DecodeSoftware, DecodeVa, DecodeNvcodec, DecodeQsv, DecodeDxva}

// Decoder is one decoder element and the pixel formats it decodes.
type Decoder struct {
	// Element is the GStreamer element name, e.g. "vah265dec". It is what the native
	// grid's stats overlay shows for a playing stream, so a verdict here and a running
	// pipeline name the same thing.
	Element string `json:"element"`
	// Family is the plugin the element comes from, one of DecodeFamilies.
	Family string `json:"family"`
	// Format is the bitstream format it decodes, as Codec.Format spells it.
	Format string `json:"format"`
	// Chromas lists the pixel formats this element decodes, named as the publish side
	// names them (Codec.Chromas), so one axis crosses the wire. A hardware element
	// advertises the subset its driver and GPU generation implement, which is at most
	// this.
	Chromas []string `json:"chromas"`
	// Reason states what bounds the chroma list: the profiles the API carries, or the
	// ones the silicon implements. It is shown where a publish choice costs a software
	// decode, so it says why rather than restating the list.
	Reason string `json:"reason"`
}

// Hardware reports whether this decoder runs on fixed-function silicon.
func (d Decoder) Hardware() bool {
	return d.Family != DecodeSoftware
}

// hardware420 is the pixel-format set of every fixed-function decoder that reaches
// 10-bit: 4:2:0 at both bit depths and nothing else. Named once because it is the answer
// for most rows below, and because a row diverging from it is the interesting case.
var hardware420 = []string{"yuv420p", "p010le"}

// Decoders is the decode capability table, one row per decoder element the viewer paths
// autoplug. Order is family order: the software row of a format first, then the hardware
// families.
//
// What is absent is as much a fact as what is here, and two absences drive most of the
// verdicts. No vendor has ever put H.264's High 4:4:4 Predictive profile in silicon, so
// nothing decodes 4:4:4 H.264 in hardware, which is also what makes lossless H.264 a
// software decode everywhere: the mode is only defined in that profile. And no AV1 or VP9
// decoder implements the full-chroma profiles (AV1 profile 1, VP9 profiles 1 and 3), so
// full chroma on the royalty-free formats is a software decode as well.
//
// HEVC is the exception on both counts. Its Range Extensions profiles are in two
// vendors' decoders, and they are what a 4:4:4 or RGB screen stream needs to reach a
// GPU: NVDEC from Turing on, and Intel's from Ice Lake on. That makes hevc the one
// format where a publisher can pick full chroma and still be decoded in hardware, and
// only by those two vendors.
//
// 4:2:2 divides the same way and one step narrower. The two software H.26x rows
// carry it because libavcodec codes every profile, and Intel's HEVC decoder is the
// one hardware row that does: NVDEC implements the 4:4:4 Range Extensions profiles
// without the 4:2:2 one, so the middle subsampling reaches fewer GPUs than full
// chroma does.
var Decoders = []Decoder{
	// H.264. Every hardware row is the same 8-bit 4:2:0 set, and the profile lists the
	// elements advertise say so: baseline through high, with no High 10 and no High
	// 4:4:4 Predictive on any of them.
	{
		Element: "avdec_h264",
		Family:  DecodeSoftware,
		Format:  "h264",
		Chromas: []string{"yuv444p", "yuv422p", "yuv420p", "p010le"},
		Reason:  "libavcodec decodes every H.264 profile, which is what covers the 4:4:4, 4:2:2 and 10-bit ones no hardware decoder carries",
	},
	{
		Element: "vah264dec",
		Family:  DecodeVa,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
		Reason:  "no VA driver exposes an H.264 decode profile above High, so 10-bit and 4:4:4 have no VA form",
	},
	{
		Element: "nvh264dec",
		Family:  DecodeNvcodec,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
		Reason:  "NVDEC's H.264 decoder is 8-bit 4:2:0 on every generation",
	},
	{
		Element: "qsvh264dec",
		Family:  DecodeQsv,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
		Reason:  "Intel's H.264 decoder is 8-bit 4:2:0 on every generation",
	},
	{
		Element: "d3d11h264dec",
		Family:  DecodeDxva,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
		Reason:  "DXVA carries one H.264 decode profile, and it is 8-bit 4:2:0",
	},

	// HEVC, the one format whose full-chroma profiles reached silicon. The two vendor
	// rows carry them and the two neutral paths do not.
	{
		Element: "avdec_h265",
		Family:  DecodeSoftware,
		Format:  "hevc",
		Chromas: []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"},
		Reason:  "libavcodec decodes the Range Extensions profiles, 4:2:2 and RGB included, so it is the CPU path for any HEVC a GPU refuses",
	},
	{
		Element: "vah265dec",
		Family:  DecodeVa,
		Format:  "hevc",
		Chromas: hardware420,
		Reason:  "Mesa's VA drivers expose HEVC Main and Main 10 for decoding and no Range Extensions profile, so 4:4:4 and RGB have no VA form on an AMD GPU",
	},
	{
		Element: "nvh265dec",
		Family:  DecodeNvcodec,
		Format:  "hevc",
		Chromas: []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		Reason:  "NVDEC decodes the HEVC 4:4:4 profiles from Turing on and no 4:2:2 one, and the element hands RGB back as RGB, so a full-chroma screen stream reaches the GPU on an NVIDIA viewer where the middle subsampling does not",
	},
	{
		Element: "qsvh265dec",
		Family:  DecodeQsv,
		Format:  "hevc",
		Chromas: []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"},
		Reason:  "Intel's HEVC decoder carries the Range Extensions profiles from Ice Lake on, 4:2:2 and 4:4:4 both, the second hardware path a full-chroma screen stream has",
	},
	{
		Element: "d3d11h265dec",
		Family:  DecodeDxva,
		Format:  "hevc",
		Chromas: hardware420,
		Reason:  "DXVA carries HEVC Main and Main 10 only, so a Windows viewer needs its vendor's own plugin for 4:4:4",
	},

	// AV1. Profile 0 covers both bit depths, and no decoder implements profile 1, so
	// full chroma is the CPU's on every vendor.
	{
		Element: "dav1ddec",
		Family:  DecodeSoftware,
		Format:  "av1",
		Chromas: []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		Reason:  "dav1d decodes every AV1 profile, which is what covers the full-chroma profile 1 no hardware decoder carries",
	},
	{
		Element: "vaav1dec",
		Family:  DecodeVa,
		Format:  "av1",
		Chromas: hardware420,
		Reason:  "the VA drivers expose AV1 profile 0, which is 4:2:0 at both bit depths",
	},
	{
		Element: "nvav1dec",
		Family:  DecodeNvcodec,
		Format:  "av1",
		Chromas: hardware420,
		Reason:  "NVDEC decodes AV1's main profile, which is 4:2:0 at both bit depths",
	},
	{
		Element: "qsvav1dec",
		Family:  DecodeQsv,
		Format:  "av1",
		Chromas: hardware420,
		Reason:  "Intel's AV1 decoder carries profile 0 from Tiger Lake on, which is 4:2:0 at both bit depths",
	},
	{
		Element: "d3d11av1dec",
		Family:  DecodeDxva,
		Format:  "av1",
		Chromas: hardware420,
		Reason:  "DXVA carries AV1 profile 0 alone",
	},

	// VP9. Profiles 0 and 2 are the 4:2:0 pair every decoder carries; profiles 1 and 3
	// are the full-chroma ones none of them does.
	{
		Element: "vp9dec",
		Family:  DecodeSoftware,
		Format:  "vp9",
		Chromas: []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
		Reason:  "libvpx decodes all four VP9 profiles, and profile 1 is what carries 4:4:4 and the identity matrix RGB travels in",
	},
	{
		Element: "vavp9dec",
		Family:  DecodeVa,
		Format:  "vp9",
		Chromas: hardware420,
		Reason:  "the VA drivers expose VP9 profiles 0 and 2, the 8-bit and 10-bit 4:2:0 pair",
	},
	{
		Element: "nvvp9dec",
		Family:  DecodeNvcodec,
		Format:  "vp9",
		Chromas: hardware420,
		Reason:  "NVDEC decodes VP9 profiles 0 and 2",
	},
	{
		Element: "qsvvp9dec",
		Family:  DecodeQsv,
		Format:  "vp9",
		Chromas: hardware420,
		Reason:  "Intel's VP9 decoder carries profiles 0 and 2",
	},
	{
		Element: "d3d11vp9dec",
		Family:  DecodeDxva,
		Format:  "vp9",
		Chromas: hardware420,
		Reason:  "DXVA carries VP9 profile 0 and the 10-bit profile 2",
	},

	// VP8: one profile, one chroma, one bit depth, so every row is the whole format.
	{
		Element: "vp8dec",
		Family:  DecodeSoftware,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
		Reason:  "VP8 has one profile, and it is 8-bit 4:2:0",
	},
	{
		Element: "vavp8dec",
		Family:  DecodeVa,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
		Reason:  "VP8 has one profile, and it is 8-bit 4:2:0",
	},
	{
		Element: "nvvp8dec",
		Family:  DecodeNvcodec,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
		Reason:  "VP8 has one profile, and it is 8-bit 4:2:0",
	},
	{
		Element: "d3d11vp8dec",
		Family:  DecodeDxva,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
		Reason:  "VP8 has one profile, and it is 8-bit 4:2:0",
	},
}

// HardwareDecoders returns the hardware decoders that take format at chroma, in table
// order, and nothing where a viewer's GPU cannot decode that pair at all.
//
// The format and chroma are the publisher's own choice, held against the codec table
// before reaching here, so an unknown pair is answered rather than asserted: it has no
// hardware decoder, which is what a caller asking about a combination this app does not
// publish should hear.
func HardwareDecoders(format, chroma string) []Decoder {
	var out []Decoder
	for _, d := range Decoders {
		if d.Format == format && d.Hardware() && contains(d.Chromas, chroma) {
			out = append(out, d)
		}
	}
	return out
}

// SoftwareDecoder returns the CPU decoder for format, and false for a format no row
// carries. Every format this app publishes has one, which is what makes a missing
// hardware decoder a cost rather than a failure.
func SoftwareDecoder(format string) (Decoder, bool) {
	for _, d := range Decoders {
		if d.Format == format && !d.Hardware() {
			return d, true
		}
	}
	return Decoder{}, false
}

// DecodesInHardware reports whether any GPU decodes format at chroma.
func DecodesInHardware(format, chroma string) bool {
	return len(HardwareDecoders(format, chroma)) > 0
}

// Decode is what a publish choice costs the viewer: the decoders that take the stream on
// a GPU, and the reasons the families that do not give.
type Decode struct {
	// Hardware names the elements that decode this stream on fixed-function silicon, in
	// table order. Empty means every viewer decodes it on the CPU.
	Hardware []Decoder `json:"hardware"`
	// Software is the CPU decoder that takes the stream where no GPU does, and the one
	// the reasons below are measured against.
	Software Decoder `json:"software"`
	// Missing is one entry per hardware family that has a decoder for the format but
	// not for this pixel format, carrying that family's reason. It is what a publisher
	// choosing a chroma with no hardware decoder is told, and it is empty when every
	// family that decodes the format decodes this pair.
	Missing []Decoder `json:"missing"`
}

// DecodeOf returns what codec at chroma costs the viewer to decode. The codec is named
// rather than the format so the caller passes the same value the settings carry, and the
// error is the codec table's: a name it does not hold has no format to decode.
//
// It answers for the format, not for the encoder: a stream is a bitstream by the time it
// reaches a viewer, so hevc_nvenc and libx265 at the same chroma decode identically.
func DecodeOf(codec, chroma string) (Decode, error) {
	c, ok := Get(codec)
	if !ok {
		return Decode{}, fmt.Errorf("unknown codec %q", codec)
	}
	software, ok := SoftwareDecoder(c.Format)
	// Every format the codec table publishes has a software decoder, so a missing one is
	// a row added to one table and not the other rather than a viewer's misfortune.
	assert.Assert(ok, "every published format has a software decoder", c.Format)

	d := Decode{Hardware: HardwareDecoders(c.Format, chroma), Software: software}
	for _, dec := range Decoders {
		if dec.Format == c.Format && dec.Hardware() && !contains(dec.Chromas, chroma) {
			d.Missing = append(d.Missing, dec)
		}
	}
	return d, nil
}
