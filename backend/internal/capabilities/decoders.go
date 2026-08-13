package capabilities

import (
	"fmt"

	"bjoernblessin.de/go-utils/util/assert"
)

// The decode half of the codec facts: which decoder takes a published stream, and whether it runs on
// silicon or on a core.
// The publisher's chroma decides it, which is what makes a viewer's cost a publish-side question.
//
// Nothing here is probed, because the viewer is not this machine: a stream is published once and
// watched on whatever hardware the watchers have.
// A row states what its decoder family reaches at its best, so a hardware verdict means some GPU
// decodes the stream rather than that a given one will.
// The measured counterpart is the native grid, which reports the element its own decodebin picked
// per stream.
//
// Every format carries a software row, so no stream is undecodable and a verdict is only ever about
// what it costs.

// The decoder families a stream can be decoded by, and the axis every per-vendor fact keys off,
// as the encoder families are on the publish side.
// They are their own set rather than the encoder families reused: the vendors do not divide the same
// way (NVDEC and NVENC carry different formats, AMD ships no decoder plugin of its own),
// and DXVA belongs to a platform rather than to a vendor.
const (
	// DecodeSoftware is the CPU path: gst-libav for the H.26x formats, libvpx for VPx, dav1d for AV1.
	// It is on every install, which is what keeps every stream playable.
	DecodeSoftware = "software"
	// DecodeVa is the va plugin (vah264dec and siblings): Linux, Intel and AMD, over whichever VA driver
	// the machine has.
	DecodeVa = "va"
	// DecodeNvcodec is the nvcodec plugin (nvh264dec and siblings): NVDEC on Linux and Windows.
	DecodeNvcodec = "nvcodec"
	// DecodeQsv is the qsv plugin (qsvh264dec and siblings): Intel's oneVPL runtime on Linux and
	// Windows.
	DecodeQsv = "qsv"
	// DecodeDxva is the d3d11 plugin (d3d11h264dec and siblings): Windows and vendor neutral,
	// the path a machine with no vendor plugin still has.
	DecodeDxva = "dxva"
)

// DecodeFamilies is the closed set a row's Family comes out of.
var DecodeFamilies = []string{DecodeSoftware, DecodeVa, DecodeNvcodec, DecodeQsv, DecodeDxva}

// Decoder is one decoder element and the pixel formats it decodes.
type Decoder struct {
	// Element is the GStreamer element name, "vah265dec".
	// The native grid's stats overlay shows the same string for a playing stream, so a verdict and a
	// running pipeline name one thing.
	Element string `json:"element"`
	// Family is the plugin the element comes from, one of DecodeFamilies.
	Family string `json:"family"`
	// Format is the bitstream format, spelled as Codec.Format spells it.
	Format string `json:"format"`
	// Chromas are the pixel formats this element decodes, named as the publish side names them
	// (Codec.Chromas), so one vocabulary crosses the wire.
	// A hardware element advertises the subset its driver and GPU generation implement, at most this.
	Chromas []string `json:"chromas"`
}

func (d Decoder) Hardware() bool {
	return d.Family != DecodeSoftware
}

// hardware420 is the format set of every fixed-function decoder that reaches 10-bit: 4:2:0 at both
// bit depths.
// Named once so a row diverging from it reads as the interesting case.
var hardware420 = []string{"yuv420p", "p010le"}

// Decoders is the decode capability table, one row per element the viewer paths autoplug,
// in family order.
//
// The absences drive most of the verdicts.
// No vendor put H.264's High 4:4:4 Predictive profile in silicon, so 4:4:4 H.264 is a software
// decode everywhere, and lossless H.264 with it: the mode is defined in that profile alone.
// No AV1 or VP9 decoder implements the full-chroma profiles (AV1 profile 1, VP9 profiles 1 and 3),
// so full chroma on the royalty-free formats costs a core as well.
//
// HEVC is the exception on both counts, and the reason a 4:4:4 or RGB screen stream reaches a GPU at
// all: its Range Extensions profiles are in NVDEC from Turing on and in Intel's decoder from Ice
// Lake on.
// 4:2:2 divides one step narrower, since NVDEC implements those profiles without the 4:2:2 one,
// which leaves Intel's HEVC decoder as the only hardware row carrying it.
// The two software H.26x rows carry it because libavcodec implements every profile.
var Decoders = []Decoder{
	{
		Element: "avdec_h264",
		Family:  DecodeSoftware,
		Format:  "h264",
		Chromas: []string{"yuv444p", "yuv422p", "yuv420p", "p010le"},
	},
	{
		Element: "vah264dec",
		Family:  DecodeVa,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
	},
	{
		Element: "nvh264dec",
		Family:  DecodeNvcodec,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
	},
	{
		Element: "qsvh264dec",
		Family:  DecodeQsv,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
	},
	{
		Element: "d3d11h264dec",
		Family:  DecodeDxva,
		Format:  "h264",
		Chromas: []string{"yuv420p"},
	},

	{
		Element: "avdec_h265",
		Family:  DecodeSoftware,
		Format:  "hevc",
		Chromas: []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"},
	},
	{
		Element: "vah265dec",
		Family:  DecodeVa,
		Format:  "hevc",
		Chromas: hardware420,
	},
	{
		Element: "nvh265dec",
		Family:  DecodeNvcodec,
		Format:  "hevc",
		Chromas: []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
	},
	{
		Element: "qsvh265dec",
		Family:  DecodeQsv,
		Format:  "hevc",
		Chromas: []string{"gbrp", "yuv444p", "yuv422p", "yuv420p", "p010le"},
	},
	{
		Element: "d3d11h265dec",
		Family:  DecodeDxva,
		Format:  "hevc",
		Chromas: hardware420,
	},

	{
		Element: "dav1ddec",
		Family:  DecodeSoftware,
		Format:  "av1",
		Chromas: []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
	},
	{
		Element: "vaav1dec",
		Family:  DecodeVa,
		Format:  "av1",
		Chromas: hardware420,
	},
	{
		Element: "nvav1dec",
		Family:  DecodeNvcodec,
		Format:  "av1",
		Chromas: hardware420,
	},
	{
		Element: "qsvav1dec",
		Family:  DecodeQsv,
		Format:  "av1",
		Chromas: hardware420,
	},
	{
		Element: "d3d11av1dec",
		Family:  DecodeDxva,
		Format:  "av1",
		Chromas: hardware420,
	},

	{
		Element: "vp9dec",
		Family:  DecodeSoftware,
		Format:  "vp9",
		Chromas: []string{"gbrp", "yuv444p", "yuv420p", "p010le"},
	},
	{
		Element: "vavp9dec",
		Family:  DecodeVa,
		Format:  "vp9",
		Chromas: hardware420,
	},
	{
		Element: "nvvp9dec",
		Family:  DecodeNvcodec,
		Format:  "vp9",
		Chromas: hardware420,
	},
	{
		Element: "qsvvp9dec",
		Family:  DecodeQsv,
		Format:  "vp9",
		Chromas: hardware420,
	},
	{
		Element: "d3d11vp9dec",
		Family:  DecodeDxva,
		Format:  "vp9",
		Chromas: hardware420,
	},

	{
		Element: "vp8dec",
		Family:  DecodeSoftware,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
	},
	{
		Element: "vavp8dec",
		Family:  DecodeVa,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
	},
	{
		Element: "nvvp8dec",
		Family:  DecodeNvcodec,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
	},
	{
		Element: "d3d11vp8dec",
		Family:  DecodeDxva,
		Format:  "vp8",
		Chromas: []string{"yuv420p"},
	},
}

// HardwareDecoders returns the hardware decoders that take format at chroma, in table order.
//
// A pair no row carries is answered with nothing rather than asserted: the format and the chroma are
// the publisher's own choice, held against the codec table before reaching here,
// and a combination this app does not publish has no hardware decoder either.
func HardwareDecoders(format, chroma string) []Decoder {
	var out []Decoder
	for _, d := range Decoders {
		if d.Format == format && d.Hardware() && contains(d.Chromas, chroma) {
			out = append(out, d)
		}
	}
	return out
}

// SoftwareDecoder returns the CPU decoder for format, and false for a format no row carries.
// Every format this app publishes has one, which is what makes a missing hardware decoder a cost
// rather than a failure.
func SoftwareDecoder(format string) (Decoder, bool) {
	for _, d := range Decoders {
		if d.Format == format && !d.Hardware() {
			return d, true
		}
	}
	return Decoder{}, false
}

func DecodesInHardware(format, chroma string) bool {
	return len(HardwareDecoders(format, chroma)) > 0
}

// Decode is what a publish choice costs the viewer: the decoders that take the stream on a GPU,
// and the reason from each family that does not.
type Decode struct {
	// Hardware are the elements that decode this stream on fixed-function silicon, in table order.
	// Empty means every viewer spends a core on it.
	Hardware []Decoder `json:"hardware"`
	// Software is the CPU decoder the stream falls to, and what the reasons below are measured
	// against.
	Software Decoder `json:"software"`
	// Missing carries one entry per hardware family that decodes the format but not this pixel format,
	// which is what a publisher picking such a chroma is told.
	Missing []Decoder `json:"missing"`
}

// DecodeOf returns what codec at chroma costs a viewer to decode.
// The codec is named rather than the format so a caller passes the value its settings carry,
// and the error is the codec table's: a name it does not hold has no format to look up.
//
// It answers for the format and not for the encoder, so hevc_nvenc and libx265 at one chroma decode
// alike.
func DecodeOf(codec, chroma string) (Decode, error) {
	c, ok := Get(codec)
	if !ok {
		return Decode{}, fmt.Errorf("unknown codec %q", codec)
	}
	software, ok := SoftwareDecoder(c.Format)
	// A published format with no software decoder is a row added to one table and not the other,
	// rather than a viewer's misfortune.
	assert.Assert(ok, "every published format has a software decoder", c.Format)

	d := Decode{Hardware: HardwareDecoders(c.Format, chroma), Software: software}
	for _, dec := range Decoders {
		if dec.Format == c.Format && dec.Hardware() && !contains(dec.Chromas, chroma) {
			d.Missing = append(d.Missing, dec)
		}
	}

	assert.Assert(d.Software.Element != "", "a decode verdict names the CPU decoder behind it", c.Format)
	return d, nil
}
