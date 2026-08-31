package capabilities

import (
	"slices"
	"testing"
)

// Both tables speak one vocabulary of formats and pixel formats,
// and a decode verdict is looked up with values the codec table produced.
// A decoder row naming a format nothing publishes answers a question no caller asks,
// and one naming a chroma outside the set matches nothing it was written to cover.
func TestEveryDecoderRowSpeaksTheCodecTablesVocabulary(t *testing.T) {
	chromas := map[string]bool{}
	for _, c := range Codecs {
		for _, chroma := range c.Chromas {
			chromas[chroma] = true
		}
	}
	for _, d := range Decoders {
		if !slices.Contains(DecodeFamilies, d.Family) {
			t.Errorf("%s declares family %q, which is not one of %v", d.Element, d.Family, DecodeFamilies)
		}
		if !HasFormat(d.Format) {
			t.Errorf("%s decodes format %q, which no implemented codec publishes", d.Element, d.Format)
		}
		if len(d.Chromas) == 0 {
			t.Errorf("%s decodes no pixel format at all", d.Element)
		}
		for _, chroma := range d.Chromas {
			if !chromas[chroma] {
				t.Errorf("%s decodes chroma %q, which no codec row encodes", d.Element, chroma)
			}
		}
	}
}

// A viewer decodes what a publisher sent,
// and a pair with neither a hardware nor a software row is a stream nothing plays.
func TestEveryPublishableStreamHasADecoder(t *testing.T) {
	for _, c := range Codecs {
		if !c.Implemented {
			continue
		}
		software, ok := SoftwareDecoder(c.Format)
		if !ok {
			t.Errorf("%s publishes %s, which has no software decoder", c.Name, c.Format)
			continue
		}
		for _, chroma := range c.Chromas {
			if !contains(software.Chromas, chroma) && !DecodesInHardware(c.Format, chroma) {
				t.Errorf("%s at %s reaches no decoder at all", c.Name, chroma)
			}
		}
	}
}

// The software row is what a verdict is measured against,
// so a second one for a format would make "the CPU path" ambiguous,
// and let the first row win in silence.
func TestOneSoftwareDecoderPerFormat(t *testing.T) {
	seen := map[string]string{}
	for _, d := range Decoders {
		if d.Hardware() {
			continue
		}
		if first, ok := seen[d.Format]; ok {
			t.Errorf("%s is a second software decoder for %s, after %s", d.Element, d.Format, first)
		}
		seen[d.Format] = d.Element
	}
}

// The verdicts the model exists for:
// 4:2:0 reaches every vendor, HEVC's full-chroma profiles reach two of them,
// and H.264 4:4:4 reaches none, the one combination no vendor put in silicon,
// which is also what makes lossless H.264 a software decode everywhere.
func TestHardwareDecodeVerdicts(t *testing.T) {
	cases := []struct {
		format, chroma string
		want           []string
	}{
		{"h264", "yuv420p", []string{"vah264dec", "nvh264dec", "qsvh264dec", "d3d11h264dec", "vtdec"}},
		{"h264", "yuv444p", nil},
		{"h264", "p010le", nil},
		{"hevc", "yuv420p", []string{"vah265dec", "nvh265dec", "qsvh265dec", "d3d11h265dec", "vtdec"}},
		{"hevc", "p010le", []string{"vah265dec", "nvh265dec", "qsvh265dec", "d3d11h265dec", "vtdec"}},
		{"hevc", "yuv444p", []string{"nvh265dec", "qsvh265dec"}},
		{"hevc", "gbrp", []string{"nvh265dec", "qsvh265dec"}},
		{"av1", "yuv420p", []string{"vaav1dec", "nvav1dec", "qsvav1dec", "d3d11av1dec"}},
		{"av1", "yuv444p", nil},
		{"vp9", "p010le", []string{"vavp9dec", "nvvp9dec", "qsvvp9dec", "d3d11vp9dec"}},
		{"vp9", "gbrp", nil},
		{"vp8", "yuv420p", []string{"vavp8dec", "nvvp8dec", "d3d11vp8dec"}},
		// A format this app does not publish is answered rather than asserted.
		{"h266", "yuv420p", nil},
	}
	for _, tc := range cases {
		var got []string
		for _, d := range HardwareDecoders(tc.format, tc.chroma) {
			got = append(got, d.Element)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("HardwareDecoders(%q,%q) = %v, want %v", tc.format, tc.chroma, got, tc.want)
		}
	}
}

// The verdict follows the bitstream and not the encoder,
// so the two ways to publish one format cost a viewer alike,
// and the reasons a publisher is shown come from the families that carry the format,
// without the pixel format.
func TestDecodeOfFollowsTheFormat(t *testing.T) {
	software, err := DecodeOf("libx265", "gbrp")
	if err != nil {
		t.Fatal(err)
	}
	hardware, err := DecodeOf("hevc_nvenc", "gbrp")
	if err != nil {
		t.Fatal(err)
	}
	if len(software.Hardware) != len(hardware.Hardware) {
		t.Errorf("libx265 and hevc_nvenc at gbrp decode differently: %d against %d hardware decoders",
			len(software.Hardware), len(hardware.Hardware))
	}
	if software.Software.Element != "avdec_h265" {
		t.Errorf("HEVC falls back to %q, want avdec_h265", software.Software.Element)
	}
	// A publisher picking RGB is told about the families carrying HEVC without Range Extensions,
	// each with its own reason.
	var missing []string
	for _, d := range software.Missing {
		missing = append(missing, d.Element)
	}
	if !slices.Equal(missing, []string{"vah265dec", "d3d11h265dec", "vtdec"}) {
		t.Errorf("HEVC at gbrp misses %v, want the VA, DXVA and VideoToolbox rows", missing)
	}

	// A chroma every family decodes leaves nothing missing.
	full, err := DecodeOf("hevc_vaapi", "yuv420p")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Missing) != 0 {
		t.Errorf("HEVC at 4:2:0 is decoded by every family, got %d missing", len(full.Missing))
	}

	if _, err := DecodeOf("nope", "yuv420p"); err == nil {
		t.Error("DecodeOf must reject a codec the table does not carry")
	}
}
