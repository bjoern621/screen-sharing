package publish

import (
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// What colours the encoder input accepts is the pixel format's answer, because an HDR
// surface has exactly one way through: the only format this app encodes that carries more
// than eight bits per component.
//
// The rows are what the child narrows to the surface's own colour (gstrun/surface.go). What
// is asserted here is the other half: that a publish states the rows a surface could be, and
// states no row an 8-bit encode could not honestly carry.

// hdrCapsSettings are settings whose pipeline this engine builds, at the given pixel format.
func hdrCapsSettings(t *testing.T, chroma string) settings.Settings {
	t.Helper()
	s := baseStream()
	s.Publish.Capture = "ximagesrc"
	s.Publish.Codec, s.Publish.Mode = "hevc_nvenc", capabilities.ModeCbr
	s.Publish.Chroma = chroma
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)
	return s
}

func TestTheEncoderInputOffersTheHdrColoursOnTheTenBitFormatAlone(t *testing.T) {
	ten, err := gstColorimetries(hdrCapsSettings(t, tenBitChroma))
	if err != nil {
		t.Fatal(err)
	}
	// Standard range leads, because it is what a capture stating no transfer at all is and
	// what a run nobody narrows has to end on.
	if len(ten) != 3 || !strings.HasSuffix(ten[0], gstBt709) {
		t.Fatalf("the 10-bit format accepts %v, want the standard-range colour and the two BT.2100 ones", ten)
	}
	if !strings.HasSuffix(ten[1], gstBt2100Pq) || !strings.HasSuffix(ten[2], gstBt2100Hlg) {
		t.Errorf("the 10-bit format accepts %v, want PQ and HLG behind the standard-range colour", ten)
	}

	eight, err := gstColorimetries(hdrCapsSettings(t, "yuv420p"))
	if err != nil {
		t.Fatal(err)
	}
	// An 8-bit encode of an HDR surface is refused on the capture's own report, so offering
	// the rows here would be offering a negotiation that ends in a stream tagged PQ over
	// eight-bit samples.
	if len(eight) != 1 || !strings.HasSuffix(eight[0], gstBt709) {
		t.Errorf("an 8-bit format accepts %v, want the standard-range colour alone", eight)
	}
}

// Every colour the encoder input accepts carries the configured range. The range is what
// decides how a viewer expands the picture, and a row that dropped it would leave one of the
// negotiable outcomes signalling something the settings never asked for.
func TestEveryAcceptedColourCarriesTheConfiguredRange(t *testing.T) {
	for _, tc := range []struct{ colorRange, want string }{
		{colorRange: "pc", want: gstRangeFull},
		{colorRange: "tv", want: gstRangeLimited},
	} {
		s := hdrCapsSettings(t, tenBitChroma)
		s.Publish.ColorRange = tc.colorRange
		colorimetries, err := gstColorimetries(s)
		if err != nil {
			t.Fatal(err)
		}
		for _, colorimetry := range colorimetries {
			if !strings.HasPrefix(colorimetry, tc.want+":") {
				t.Errorf("colour range %s accepts %s, which states another range", tc.colorRange, colorimetry)
			}
		}
	}
}

// The rows reach the capsfilter as structures rather than as one value list, which is the
// difference between a choice and a conversion: videoconvert fixates a list to its first
// entry whatever the frames carry, so a list would convert every HDR surface into the
// standard-range row and call it negotiation.
func TestTheEncoderInputStatesItsColoursAsStructures(t *testing.T) {
	caps, err := gstTestCaps(hdrCapsSettings(t, tenBitChroma))
	if err != nil {
		t.Fatal(err)
	}
	structures := strings.Split(caps, ";")
	if len(structures) != 3 {
		t.Fatalf("the encoder input caps carry %d structures, want one per accepted colour: %s", len(structures), caps)
	}
	for _, s := range structures {
		if strings.Contains(s, "{") {
			t.Errorf("a colour is stated as a value list rather than as a structure: %s", s)
		}
		if !strings.HasPrefix(s, "video/x-raw") {
			t.Errorf("a structure of the encoder input caps names no media type: %s", s)
		}
	}
}
